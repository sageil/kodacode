package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/tool"
)

const defaultSubagentTimeout = 5 * time.Minute

const (
	subagentAccountingTimeout = 1 * time.Second
	subagentDrainTimeout      = 5 * time.Second
)

// ephemeralKey wraps tool.EphemeralKey for backward compatibility within service.
type ephemeralKey = tool.EphemeralKey

type sessionSubagentService struct {
	limiter chan struct{}
}

type structuredPlannerKey struct{}

func newSessionSubagentService(cfg *config.Config) *sessionSubagentService {
	return &sessionSubagentService{limiter: newSubagentLimiter(cfg)}
}

func (s *sessionSubagentService) Acquire() (func(), error) {
	if s == nil || s.limiter == nil {
		return func() {}, nil
	}
	select {
	case s.limiter <- struct{}{}:
		return func() { <-s.limiter }, nil
	default:
		return nil, fmt.Errorf("subagent limit reached (%d/%d) — wait for running subagents to complete", len(s.limiter), cap(s.limiter))
	}
}

// InitSubagentLimit is kept for compatibility with older callers.
// Concurrency is now scoped per SessionService instance, so this is a no-op.
func InitSubagentLimit(max int) {
	_ = max
}

func newSubagentLimiter(cfg *config.Config) chan struct{} {
	max := 10
	if cfg != nil && cfg.Session.MaxSubagents > 0 {
		max = cfg.Session.MaxSubagents
	}
	if max <= 0 {
		max = 10
	}
	return make(chan struct{}, max)
}

// ProgressFunc receives formatted progress lines from a subagent's tool execution.
// Each line describes a tool start or completion (e.g. "⟩ Read file.go\n").
type ProgressFunc func(line string)

func (s *SessionService) SpawnSubagent(ctx context.Context, parentSessionID, agentID, task string, onProgress ProgressFunc) (string, error) {
	release, err := s.subagents.Acquire()
	if err != nil {
		return "", err
	}
	defer release()

	timeout := defaultSubagentTimeout
	if s.runtime != nil && s.runtime.cfg != nil && s.runtime.cfg.Session.SubagentTimeout > 0 {
		timeout = time.Duration(s.runtime.cfg.Session.SubagentTimeout) * time.Minute
	}

	parent, err := s.store.sessions.Get(ctx, parentSessionID)
	if err != nil {
		return "", fmt.Errorf("get parent session: %w", err)
	}
	if modeLookup, ok := s.runtime.agents.(interface{ Mode(string) (string, error) }); ok {
		mode, err := modeLookup.Mode(agentID)
		if err != nil {
			return "", fmt.Errorf("unknown agent %q — available agents are listed in the agent_id parameter description", agentID)
		}
		if mode == "" || mode == agent.ModePrimary {
			return "", fmt.Errorf("agent %q is not available as a subagent", agentID)
		}
	}
	agentCfg := s.resolveAgentConfig(agentID)
	if agentCfg.SystemPrompt == "" && agentCfg.Tools == nil {
		return "", fmt.Errorf("unknown agent %q — available agents are listed in the agent_id parameter description", agentID)
	}
	agentCfg = subagentAgentConfig(ctx, agentID, agentCfg)
	log.Printf("workflow: spawning subagent=%s from parent=%s task=%q", agentID, parentSessionID, truncateLog(task, 200))
	modelID := parent.ModelID
	if agentID == "planner" {
		log.Printf("subagent %s: forcing parent model %s", agentID, modelID)
	} else {
		switch agentCfg.Model {
		case "utility":
			if s.runtime != nil && s.runtime.cfg != nil && s.runtime.cfg.UtilityModel != "" {
				modelID = s.runtime.cfg.UtilityModel
			} else {
				log.Printf("subagent %s: agent requests model=utility but no utility_model configured — falling back to parent model %s", agentID, modelID)
			}
		case "reviewer":
			if s.runtime != nil && s.runtime.cfg != nil && s.runtime.cfg.ReviewerModel != "" {
				modelID = s.runtime.cfg.ReviewerModel
			} else {
				log.Printf("subagent %s: no reviewer_model configured — falling back to parent model %s", agentID, modelID)
			}
		case "":
		default:
			modelID = agentCfg.Model
		}
	}

	result, timedOut := s.runSubagent(ctx, parentSessionID, agentID, modelID, task, timeout, onProgress)

	// Retry once on timeout. Give the subagent another chance with the
	// partial results as context so it can pick up where it left off.
	if timedOut && result != "" {
		log.Printf("subagent: %s timed out with %d chars — retrying with partial context", agentID, len(result))
		retryTask := fmt.Sprintf("Continue the following task. Here is what was completed before timeout:\n\n%s\n\n---\nOriginal task: %s\n\nPick up where the previous attempt left off. Do NOT repeat work already done above.", result, task)
		retryResult, retryTimedOut := s.runSubagent(ctx, parentSessionID, agentID, modelID, retryTask, timeout, onProgress)
		if retryTimedOut {
			log.Printf("subagent: %s retry also timed out — returning combined partial output", agentID)
			return result + "\n\n" + retryResult + "\n\n[subagent timed out after retry — partial results above]", nil
		}
		if retryResult != "" {
			return retryResult, nil
		}
		// If the retry produced nothing, return the original partial output.
		return result + "\n\n[subagent timed out — partial results above]", nil
	}

	if timedOut {
		return "", fmt.Errorf("subagent %s timed out with no output", agentID)
	}

	if result == "" {
		log.Printf("workflow: subagent=%s produced no response", agentID)
		return "Subagent produced no response.", nil
	}

	log.Printf("workflow: subagent=%s completed, response=%d chars", agentID, len(result))
	return result, nil
}

// runSubagent executes a single subagent attempt. Returns the collected output
// and whether the attempt timed out.
func (s *SessionService) runSubagent(ctx context.Context, parentSessionID, agentID, modelID, task string, timeout time.Duration, onProgress ProgressFunc) (string, bool) {
	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	sess, err := s.Create(ctx, agentID, modelID, WithEphemeral())
	if err != nil {
		log.Printf("subagent: failed to create session: %v", err)
		return "", false
	}
	startTime := time.Now()

	defer func() {
		elapsed := time.Since(startTime)
		if snap, ok := s.GetSessionCost(sess.ID); ok {
			costCtx, costCancel := context.WithTimeout(context.WithoutCancel(ctx), subagentAccountingTimeout)
			parentCost := s.GetOrCreateCost(costCtx, parentSessionID)
			costCancel()
			parentCost.AddSubagentCost(snap)
			log.Printf("subagent-cost: agent=%s model=%s in=%d out=%d cost=$%.4f elapsed=%s",
				agentID, modelID, snap.InputTokens, snap.OutputTokens, snap.TotalCost, elapsed.Truncate(time.Millisecond))
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		if delErr := s.Delete(cleanupCtx, sess.ID); delErr != nil {
			log.Printf("subagent: failed to delete session %s: %v", sess.ID, delErr)
			s.cleanupSessionState(sess.ID)
		}
	}()

	var response strings.Builder
	var mu sync.Mutex
	done := make(chan struct{})

	sub, cancel := s.Subscribe(sess.ID)
	defer cancel()

	go func() {
		defer close(done)
		for ev := range sub {
			switch ev.Type {
			case "reasoning_delta":
				if d, ok := ev.Data.(SSEReasoningDeltaData); ok && d.Content != "" {
					if onProgress != nil {
						onProgress(d.Content)
					}
				}
			case "reasoning_done":
			case "delta":
				if d, ok := ev.Data.(SSEDeltaData); ok {
					mu.Lock()
					response.WriteString(d.Content)
					mu.Unlock()
					if onProgress != nil {
						onProgress(d.Content)
					}
				}
			case "tool_start":
				if d, ok := ev.Data.(SSEToolStartData); ok && d.Input != "" {
					s.publish(parentSessionID, SSEEvent{
						Type: "subagent_activity",
						Data: SSESubagentActivityData{Tool: d.Tool, Input: d.Input},
					})
				}
			case "tool_output":
			case "tool_end":
				if d, ok := ev.Data.(SSEToolEndData); ok {
					s.publish(parentSessionID, SSEEvent{
						Type: "subagent_activity",
						Data: SSESubagentActivityData{Tool: d.Tool, Output: d.Output, Done: true, Error: d.Error != nil},
					})
				}
			case "question", "user_question":
				s.publish(parentSessionID, ev)
				// Clear the response buffer. Text before the question was
				// the question prompt, which is already displayed via the
				// inline question panel. Keep only post-answer content.
				mu.Lock()
				response.Reset()
				mu.Unlock()
			case "done", "error":
				return
			}
		}
	}()

	sendCtx := context.WithValue(ctx, ephemeralKey{}, true)
	if shouldForwardTaskSession(ctx, agentID) {
		sendCtx = context.WithValue(sendCtx, tool.TaskSessionKey{}, parentSessionID)
	}
	sendErr := s.Send(sendCtx, sess.ID, task, nil, 0)

	s.publish(sess.ID, SSEEvent{Type: "done", Data: SSEDoneData{}})
	drainCtx, drainCancel := context.WithTimeout(context.WithoutCancel(ctx), subagentDrainTimeout)
	defer drainCancel()
	select {
	case <-done:
	case <-drainCtx.Done():
	}

	mu.Lock()
	result := response.String()
	mu.Unlock()

	if sendErr != nil {
		if ctx.Err() != nil {
			// Timeout. Return partial output.
			return result, true
		}
		log.Printf("subagent: send error: %v", sendErr)
		if result != "" {
			return result + "\n\n[subagent error: " + sendErr.Error() + "]", false
		}
		return "", false
	}

	return result, false
}

func withStructuredPlanner(ctx context.Context) context.Context {
	return context.WithValue(ctx, structuredPlannerKey{}, true)
}

func usesStructuredPlanner(ctx context.Context, agentID string) bool {
	return agentID == "planner" && ctx != nil && ctx.Value(structuredPlannerKey{}) == true
}

func shouldForwardTaskSession(ctx context.Context, agentID string) bool {
	return !usesStructuredPlanner(ctx, agentID)
}

func subagentAgentConfig(ctx context.Context, agentID string, cfg config.AgentConfig) config.AgentConfig {
	if !usesStructuredPlanner(ctx, agentID) {
		return cfg
	}
	out := cfg
	if len(out.Tools) > 0 {
		tools := make([]string, 0, len(out.Tools))
		for _, name := range out.Tools {
			if name != "task" {
				tools = append(tools, name)
			}
		}
		out.Tools = tools
	}
	denied := make([]string, 0, len(out.DenyTools)+1)
	denied = append(denied, out.DenyTools...)
	for _, name := range denied {
		if name == "task" {
			out.DenyTools = denied
			return out
		}
	}
	out.DenyTools = append(denied, "task")
	return out
}

func truncateLog(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
