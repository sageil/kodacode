package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/skills"
	"github.com/sageil/kodacode/v1/internal/tool"
)

type toolExecution struct {
	call    provider.ToolCall
	result  *tool.Result // full structured result; nil on Go-level error
	output  string
	errStr  *string
	elapsed time.Duration
}

// dispatchToolCalls executes tool calls in parallel and collects results in
// call order. Identical read-only tool calls within the same batch are
// deduplicated: the second caller waits for the first and reuses its result.
// Side-effecting tools are always executed independently.
//
// Concurrency safety notes (Go memory model):
//   - dedupMap and callDedup are populated in the sequential loop (lines below)
//     BEFORE any goroutines are launched. Goroutines only read these maps.
//   - Writes to entry.output and entry.errStr happen before close(entry.done).
//     Per Go's memory model, close(ch) happens-before a receive on ch returns,
//     so readers after <-entry.done see the final values.
//   - Each goroutine writes to a unique executions[idx] entry, so there are no races.
func (tl *turnLoop) dispatchToolCalls(calls []provider.ToolCall) []toolExecution {
	toolCtx := tl.ctx
	if tl.req.Usage != nil {
		cs := tl.req.Model.EffectiveContextSize()
		if cs <= 0 {
			cs = 128000
		}
		actualTokens := tl.req.Usage.InputTokens + tl.req.Usage.CacheReadTokens + tl.req.Usage.CacheWriteTokens
		toolCtx = tool.WithContextUsage(tl.ctx, float64(actualTokens)/float64(cs))
	}
	if tl.globalCfg != nil {
		toolCtx = tool.WithSkillPolicy(toolCtx, skills.ResolveAccess(tl.globalCfg, tl.req.ProviderID, tl.req.Model.ID, tl.req.Agent))
	}

	type dedupEntry struct {
		firstIdx int
		output   string
		errStr   *string
		done     chan struct{}
	}
	dedupMap := make(map[string]*dedupEntry)
	callDedup := make([]*dedupEntry, len(calls))
	for i, tc := range calls {
		readOnly := tl.sndbx != nil && tl.sndbx.IsReadOnly(tc.Name)
		key := tc.Name + "|" + canonicalizeArgs(tc.Arguments)
		if readOnly {
			if entry, exists := dedupMap[key]; exists {
				callDedup[i] = entry
				log.Printf("llm: dedup tool call %s (call %d reuses call %d)", tc.Name, i, entry.firstIdx)
				continue
			}
		}
		entry := &dedupEntry{firstIdx: i, done: make(chan struct{})}
		dedupMap[key] = entry
		callDedup[i] = entry
	}

	executions := make([]toolExecution, len(calls))
	var wg sync.WaitGroup
	for i, tc := range calls {
		wg.Add(1)
		go func(idx int, tc provider.ToolCall) {
			defer wg.Done()
			execStart := time.Now()

			entry := callDedup[idx]

			if entry.firstIdx == idx && tl.toolCache != nil && tl.sndbx != nil && tl.sndbx.IsReadOnly(tc.Name) && tc.Name != "bash" {
				if cached, cachedErr, hit := tl.toolCache.lookup(tc.Name, tc.Arguments); hit {
					entry.output = cached
					entry.errStr = cachedErr
					close(entry.done)
					executions[idx] = toolExecution{call: tc, output: cached, errStr: cachedErr, elapsed: time.Since(execStart)}
					return
				}
			}

			if entry.firstIdx == idx && !isToolAllowed(tc.Name, tl.req.Tools) {
				errMsg := tl.blockedToolMessage(tc.Name)
				errText := errMsg
				log.Printf("tool_dispatch: rejected %s — not in agent's tool list", tc.Name)
				entry.output = errMsg
				entry.errStr = &errText
				close(entry.done)
				tl.publish(tl.req.SessionID, SSEEvent{
					Type: "tool_end",
					Data: SSEToolEndData{Tool: tc.Name, Output: errMsg, Error: &errText, CallID: tc.ID},
				})
				executions[idx] = toolExecution{call: tc, output: errMsg, errStr: &errText, elapsed: time.Since(execStart)}
				return
			}

			if entry.firstIdx == idx && tc.Name == "subagent" && plannerAgentIDFromArgs(tc.Arguments) == "planner" {
				if reason := plannerBlockedReason(ensureWorkflowState(tl.req), tl.req.AgentID, tl.req.Ephemeral, calls); reason != "" {
					log.Printf("workflow: blocked planner subagent call at step=%d: %s", tl.req.Step, reason)
					output := fmt.Sprintf("tool error [subagent]: %s", reason)
					errText := reason
					entry.output = output
					entry.errStr = &errText
					close(entry.done)
					tl.publish(tl.req.SessionID, SSEEvent{
						Type: "tool_end",
						Data: SSEToolEndData{Tool: tc.Name, Output: output, Error: &errText, CallID: tc.ID},
					})
					executions[idx] = toolExecution{call: tc, output: output, errStr: &errText, elapsed: time.Since(execStart)}
					return
				}
			}

			if tl.sndbx == nil || tl.sndbx.KnownTool(tc.Name) || tc.Name == "subagent" {
				tl.publish(tl.req.SessionID, SSEEvent{
					Type: "tool_start",
					Data: SSEToolStartData{Tool: tc.Name, Input: tc.Arguments, CallID: tc.ID},
				})
			}

			if entry.firstIdx != idx {
				<-entry.done
				tl.publish(tl.req.SessionID, SSEEvent{
					Type: "tool_end",
					Data: SSEToolEndData{Tool: tc.Name, Output: entry.output, Error: entry.errStr, CallID: tc.ID},
				})
				executions[idx] = toolExecution{call: tc, output: entry.output, errStr: entry.errStr, elapsed: time.Since(execStart)}
				return
			}

			var output string
			var fullResult *tool.Result
			var execErr error
			if tl.sndbx != nil {
				var askFn func(string, string) error
				if tl.askPerm != nil {
					askFn = tl.askPerm(tl.ctx, tl.req.SessionID, tc.Name, tc.Arguments)
				}
				var askUserFn func(string, []string, bool, string) (string, error)
				if tl.askUser != nil {
					askUserFn = tl.askUser(tl.ctx, tl.req.SessionID)
				}
				var spawnFn func(context.Context, string, string) (string, error)
				if tl.spawnSubagent != nil {
					sid := tl.req.SessionID
					callID := tc.ID
					spawnFn = func(ctx context.Context, agentID, task string) (string, error) {
						if agentID == "reviewer" && tl.reviewFindings != "" {
							task += "\n\n--- PREVIOUS REVIEW FINDINGS (re-review only these) ---\n" +
								tl.reviewFindings +
								"\n--- END PREVIOUS FINDINGS ---\n" +
								"This is a re-review. ONLY verify whether the findings listed above have been addressed. " +
								"Do NOT re-check criteria that already passed."
							log.Printf("review: injected %d chars of previous findings into reviewer task", len(tl.reviewFindings))
						}
						progress := ProgressFunc(func(line string) {
							tl.publish(sid, SSEEvent{
								Type: "tool_output",
								Data: map[string]string{"tool": "subagent", "chunk": line, "call_id": callID},
							})
						})
						return tl.spawnSubagent(ctx, sid, agentID, task, progress)
					}
				}
				result, err := tl.sndbx.Execute(toolCtx, tl.req.SessionID, tc,
					askFn, askUserFn, spawnFn,
					func(evType string, data any) {
						if m, ok := data.(map[string]string); ok {
							m["call_id"] = tc.ID
						}
						tl.publish(tl.req.SessionID, SSEEvent{Type: evType, Data: data})
					},
				)
				if err != nil {
					execErr = err
				} else if result != nil {
					fullResult = result
					output = result.Output
					if result.ErrorCode != "" {
						output = fmt.Sprintf("tool error (%s): %s", result.ErrorCode, result.Output)
					}
					if result.Retryable && result.ErrorCode != "" {
						output += "\n(This error is transient — retrying may succeed.)"
					}
				}
			}
			var errStr *string
			if execErr != nil {
				msg := execErr.Error()
				errStr = &msg
				output = fmt.Sprintf("tool error [%s]: %s. Check your arguments and try a different approach if this persists.", tc.Name, execErr)
			}

			entry.output = output
			entry.errStr = errStr
			close(entry.done)

			if tl.toolCache != nil && tl.sndbx != nil {
				if tl.sndbx.IsReadOnly(tc.Name) && errStr == nil && tc.Name != "bash" {
					tl.toolCache.store(tc.Name, tc.Arguments, output, errStr)
				} else if !tl.sndbx.IsReadOnly(tc.Name) {
					switch tc.Name {
					case "write", "edit", "patch":
						if path := extractFilePath(tc.Arguments); path != "" {
							tl.toolCache.invalidateByPath(path)
						} else {
							tl.toolCache.invalidate()
						}
					case "bash":
						tl.toolCache.invalidate()
					case "test", "task":
					case "git":
						if gitArgsMutate(tc.Arguments) {
							tl.toolCache.invalidate()
						}
					case "subagent":
						if name := extractSubagentName(tc.Arguments); name == "explorer" || name == "advisor" || name == "reviewer" {
						} else {
							tl.toolCache.invalidate()
						}
					default:
						tl.toolCache.invalidate()
					}
				}
			}

			if tl.sndbx == nil || tl.sndbx.KnownTool(tc.Name) || tc.Name == "subagent" {
				tl.publish(tl.req.SessionID, SSEEvent{
					Type: "tool_end",
					Data: SSEToolEndData{Tool: tc.Name, Output: output, Error: errStr, CallID: tc.ID},
				})
			}
			executions[idx] = toolExecution{call: tc, result: fullResult, output: output, errStr: errStr, elapsed: time.Since(execStart)}
		}(i, tc)
	}
	wg.Wait()
	return executions
}

func gitArgsMutate(argsJSON string) bool {
	var m map[string]any
	if json.Unmarshal([]byte(argsJSON), &m) != nil {
		return false
	}
	action, _ := m["action"].(string)
	args, _ := m["args"].(string)
	return tool.GitActionMutates(action, args)
}

// canonicalizeArgs re-serializes JSON arguments with sorted keys so that
// semantically identical tool calls (differing only in key order or whitespace)
// produce the same dedup/cache key.
func isToolAllowed(name string, tools []provider.Tool) bool {
	if isAlwaysAllowedTool(name) {
		return true
	}
	if tools == nil {
		return false
	}
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func (tl *turnLoop) blockedToolMessage(name string) string {
	if tl.req != nil && tl.req.PhaseFilterActive && isToolAllowed(name, tl.req.FullTools) {
		return phaseBlockedToolMessage(name, tl.req.AgentID, tl.req.Workflow)
	}
	return fmt.Sprintf("Tool %q is not available. Use one of the tools provided to you.", name)
}

func phaseBlockedToolMessage(name, agentID string, workflow *pipeline.WorkflowState) string {
	phase := pipeline.WorkflowPhaseUnknown
	if workflow != nil {
		phase = workflow.Phase
	}
	switch phase {
	case pipeline.WorkflowPhasePrebuild:
		return fmt.Sprintf("Tool %q is blocked in the current prebuild phase. Do not edit yet. Run the test tool first.", name)
	case pipeline.WorkflowPhasePreplan:
		if isEngineerWorkflowAgent(agentID) {
			return fmt.Sprintf("Tool %q is blocked in the current preplan phase. Do not edit yet. Planning is still in progress. Stay read-only until planning is complete, unless the user's request is purely explanatory or diagnostic.", name)
		}
		return fmt.Sprintf("Tool %q is blocked in the current preplan phase. Do not edit yet.", name)
	case pipeline.WorkflowPhasePostplanPending:
		return fmt.Sprintf("Tool %q is blocked while plan approval is pending. Wait for the approval result before editing.", name)
	case pipeline.WorkflowPhasePostplanRejected:
		return fmt.Sprintf("Tool %q is blocked because the plan was declined. Do not implement anything until the user gives a new direction.", name)
	default:
		return fmt.Sprintf("Tool %q is not available in the current workflow phase. Use one of the tools currently provided to you.", name)
	}
}

func canonicalizeArgs(raw string) string {
	var obj any
	if json.Unmarshal([]byte(raw), &obj) != nil {
		return raw
	}
	canon, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return string(canon)
}
