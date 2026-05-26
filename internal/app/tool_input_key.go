package app

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func retryableLogicalToolRetryOfCall(state events.SessionState, input ExecuteToolInput, tools map[string]tool.Tool) string {
	if !retryableLogicalToolName(input.ToolName) {
		return ""
	}
	currentKey, ok := logicalToolKey(tools, input.ToolName, input.Arguments)
	if !ok {
		return ""
	}
	turn := state.Turns[input.TurnID]
	if turn == nil {
		return ""
	}
	for idx := len(turn.ToolCallOrder) - 1; idx >= 0; idx-- {
		candidateID := turn.ToolCallOrder[idx]
		if candidateID == input.ToolCallID {
			continue
		}
		call := turn.ToolCalls[candidateID]
		if call == nil || !call.Completed {
			continue
		}
		if strings.TrimSpace(call.ToolName) != strings.TrimSpace(input.ToolName) {
			continue
		}
		candidateKey, ok := logicalToolKey(tools, call.ToolName, json.RawMessage(call.Input))
		if !ok || candidateKey != currentKey {
			continue
		}
		if call.Succeeded || strings.TrimSpace(call.Error) == "" {
			return ""
		}
		return strings.TrimSpace(call.CallID)
	}
	return ""
}

func retryableLogicalToolName(name string) bool {
	switch strings.TrimSpace(name) {
	case tool.QuestionToolName, tool.TaskWorkflowToolName, tool.TaskReviewToolName:
		return true
	default:
		return false
	}
}

func logicalToolKey(tools map[string]tool.Tool, toolName string, args json.RawMessage) (string, bool) {
	key, ok := normalizedToolInputKey(tools, toolName, args)
	if ok {
		return key, true
	}
	trimmed := strings.TrimSpace(string(args))
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func normalizedToolInputKey(tools map[string]tool.Tool, toolName string, args json.RawMessage) (string, bool) {
	tl, ok := tools[strings.TrimSpace(toolName)]
	if !ok {
		return "", false
	}
	if provider, ok := tl.(tool.NormalizedInputKeyProvider); ok {
		key, err := provider.NormalizedInputKey(args)
		if err != nil {
			return "", false
		}
		return key, true
	}
	trimmed := strings.TrimSpace(string(args))
	if trimmed == "" {
		return "", false
	}
	var decoded any
	if err := json.Unmarshal(args, &decoded); err == nil {
		canonical, err := json.Marshal(decoded)
		if err == nil && len(canonical) > 0 {
			return string(canonical), true
		}
	}
	return trimmed, true
}

func cloneObservedToolResources(resources []events.ObservedResource) []events.ObservedResource {
	if len(resources) == 0 {
		return nil
	}
	out := make([]events.ObservedResource, len(resources))
	copy(out, resources)
	return out
}
