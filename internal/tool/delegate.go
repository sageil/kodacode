package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const DelegateToolName = "delegate"
const delegateContextSummaryMaxChars = 2000

var (
	ErrDelegateAgentRequired   = errors.New("agent_id is required")
	ErrDelegateTaskRequired    = errors.New("task is required")
	ErrDelegateContextRequired = errors.New("context_summary is required")
	ErrDelegateContextTooLong  = errors.New("context_summary exceeds maximum length")
)

type DelegateTool struct{}

type delegateInput struct {
	ChildAgentID     string
	Task             string
	ContextSummary   string
	SourceHandoffIDs []string
}

func NewDelegateTool() DelegateTool {
	return DelegateTool{}
}

func (DelegateTool) Definition() Definition {
	return Definition{
		Name:                DelegateToolName,
		Description:         "Delegate a well-scoped child task to another agent and return a saved handoff summary. Use this when a separate child session is the cleanest way to isolate planning, review, or another bounded subtask. Common built-in child agents include `planner` and `reviewer`; projects may also define additional subagents. Provide the child agent id, the exact task for that child, and a short context summary the child needs before it starts. Runtime resolves source handoffs from child agent contracts when possible; source_handoff_ids is an advanced override.",
		ProviderDescription: "Delegate a well-scoped child task to an agent id such as planner or reviewer and return the handoff result.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"agent_id":{"type":"string","description":"Child agent id to run, such as \"planner\", \"reviewer\", or a project-defined subagent."},"task":{"type":"string","description":"Exact delegated task for the child agent."},"context_summary":{"type":"string","maxLength":2000,"description":"Short summary of the parent context the child needs before starting. Maximum 2000 characters."},"source_handoff_ids":{"type":["array","null"],"items":{"type":"string"},"description":"Advanced override: completed handoff ids whose results should be passed as source context to the child. Usually omit this and let runtime resolve sources from the child agent handoff contract."}},"required":["agent_id","task","context_summary"],"additionalProperties":false}`),
		ArgumentExamples: []string{
			`{"agent_id":"planner","task":"Inspect the caching layer and produce an implementation plan.","context_summary":"We need a narrow plan for cache invalidation in the API handlers."}`,
		},
	}
}

func (DelegateTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	manager, err := ectx.Delegates()
	if err != nil {
		return Result{}, err
	}
	input, err := parseDelegateInput(args)
	if err != nil {
		return Result{}, err
	}
	record, err := manager.Delegate(DelegateRequest{
		ChildAgentID:     input.ChildAgentID,
		Task:             input.Task,
		ContextSummary:   input.ContextSummary,
		SourceHandoffIDs: append([]string(nil), input.SourceHandoffIDs...),
	})
	if err != nil {
		return Result{}, err
	}
	output, err := json.Marshal(record)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: string(output)}, nil
}

func parseDelegateInput(args json.RawMessage) (_ delegateInput, err error) {
	defer func() {
		err = normalizeToolInputError(DelegateToolName, err)
	}()
	var raw struct {
		AgentID          *string         `json:"agent_id"`
		Task             *string         `json:"task"`
		ContextSummary   *string         `json:"context_summary"`
		SourceHandoffIDs json.RawMessage `json:"source_handoff_ids"`
	}
	if err := DecodeArgs(DelegateToolName, args, &raw); err != nil {
		return delegateInput{}, err
	}
	if raw.AgentID == nil || strings.TrimSpace(*raw.AgentID) == "" {
		return delegateInput{}, ErrDelegateAgentRequired
	}
	if raw.Task == nil || strings.TrimSpace(*raw.Task) == "" {
		return delegateInput{}, ErrDelegateTaskRequired
	}
	if raw.ContextSummary == nil || strings.TrimSpace(*raw.ContextSummary) == "" {
		return delegateInput{}, ErrDelegateContextRequired
	}
	contextSummary := strings.TrimSpace(*raw.ContextSummary)
	if utf8.RuneCountInString(contextSummary) > delegateContextSummaryMaxChars {
		return delegateInput{}, fmt.Errorf("%w: maximum %d characters", ErrDelegateContextTooLong, delegateContextSummaryMaxChars)
	}
	sourceHandoffIDs, _, err := decodeOptionalStringArrayArg(DelegateToolName, raw.SourceHandoffIDs, "source_handoff_ids")
	if err != nil {
		return delegateInput{}, err
	}
	return delegateInput{
		ChildAgentID:     strings.TrimSpace(*raw.AgentID),
		Task:             strings.TrimSpace(*raw.Task),
		ContextSummary:   contextSummary,
		SourceHandoffIDs: compactStringList(sourceHandoffIDs),
	}, nil
}

func compactStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
