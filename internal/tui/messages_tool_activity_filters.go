package tui

import (
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func showMutationToolInTranscript(call *events.ToolCallState) bool {
	if !transcriptOwnsToolCallRow(call) {
		return false
	}

	switch call.ToolName {
	case "write", "edit":
		return strings.TrimSpace(call.Error) == ""
	case "apply_patch":
		return strings.TrimSpace(call.Error) == "" && !isApplyPatchNoop(call)
	case "bash":
		return outcomeCategoryForTool(call) == toolOutcomeMutation && strings.TrimSpace(call.Error) == ""
	case "rename_symbol", "code_action":
		return true
	case "mkdir":
		return strings.TrimSpace(call.Error) == ""
	default:
		return false
	}
}

func transcriptOwnsToolCallRow(call *events.ToolCallState) bool {
	return call != nil && call.Completed
}

func shouldRenderToolCallInTranscript(turn *events.TurnState, callID string, call *events.ToolCallState) bool {
	if !transcriptOwnsToolCallRow(call) {
		return false
	}
	if shouldHideDelegateToolCallInTranscript(turn, call) {
		return false
	}
	if shouldHideFailedWriteEditInTranscript(call) {
		return false
	}
	if strings.TrimSpace(call.ToolName) == "skill" {
		return false
	}
	if isApplyPatchNoop(call) {
		return false
	}
	if shouldHideSupersededMutationFailure(turn, callID, call) {
		return false
	}
	if shouldHideSupersededRetriedLogicalToolCall(turn, callID, call) {
		return false
	}
	if shouldHideSupersededDelegateAttempt(turn, callID, call) {
		return false
	}
	return true
}

func shouldHideDelegateToolCallInTranscript(turn *events.TurnState, call *events.ToolCallState) bool {
	if !isDelegateToolCall(call) {
		return false
	}
	return delegateHandoffForCall(turn, call) != nil
}

func shouldHideFailedWriteEditInTranscript(call *events.ToolCallState) bool {
	if call == nil || !call.Completed {
		return false
	}
	switch strings.TrimSpace(call.ToolName) {
	case "write", "edit", "apply_patch":
		return strings.TrimSpace(call.Error) != ""
	default:
		return false
	}
}

func shouldHideSupersededMutationFailure(turn *events.TurnState, callID string, call *events.ToolCallState) bool {
	if turn == nil || !isFailedMutationToolCall(call) {
		return false
	}
	paths := mutationCallPaths(call)
	if len(paths) == 0 {
		return false
	}
	toolName := strings.TrimSpace(call.ToolName)
	seenCurrent := false
	for _, nextID := range orderedToolCallIDs(turn) {
		if nextID == callID {
			seenCurrent = true
			continue
		}
		if !seenCurrent {
			continue
		}
		next := turn.ToolCalls[nextID]
		if next == nil || strings.TrimSpace(next.ToolName) != toolName || !next.Completed || strings.TrimSpace(next.Error) != "" {
			continue
		}
		if sameMutationPathSet(paths, mutationCallPaths(next)) {
			return true
		}
	}
	return false
}

func shouldHideSupersededRetriedLogicalToolCall(turn *events.TurnState, callID string, call *events.ToolCallState) bool {
	if turn == nil || call == nil || !call.Completed || !isRetryCollapsibleLogicalToolCall(call) {
		return false
	}
	seenCurrent := false
	for _, nextID := range orderedToolCallIDs(turn) {
		if nextID == callID {
			seenCurrent = true
			continue
		}
		if !seenCurrent {
			continue
		}
		next := turn.ToolCalls[nextID]
		if !toolCallRetryChainContains(turn, next, callID) {
			continue
		}
		if strings.TrimSpace(next.ToolName) != strings.TrimSpace(call.ToolName) {
			continue
		}
		return true
	}
	return false
}

func shouldHideSupersededDelegateAttempt(turn *events.TurnState, callID string, call *events.ToolCallState) bool {
	if turn == nil || call == nil || !call.Completed || strings.TrimSpace(call.ToolName) != "delegate" {
		return false
	}
	request, ok := parseDelegateLogicalRequest(call.Input)
	if !ok {
		return false
	}
	seenCurrent := false
	for _, nextID := range orderedToolCallIDs(turn) {
		if nextID == callID {
			seenCurrent = true
			continue
		}
		if !seenCurrent {
			continue
		}
		next := turn.ToolCalls[nextID]
		if next == nil || !next.Completed || strings.TrimSpace(next.ToolName) != "delegate" {
			continue
		}
		nextRequest, ok := parseDelegateLogicalRequest(next.Input)
		if !ok {
			continue
		}
		if request != nextRequest {
			continue
		}
		return true
	}
	return false
}

func isRetryCollapsibleLogicalToolCall(call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	switch strings.TrimSpace(call.ToolName) {
	case "question", "task", "task_workflow", "task_review", "edit":
		return true
	default:
		return false
	}
}

func toolCallRetryChainContains(turn *events.TurnState, call *events.ToolCallState, targetCallID string) bool {
	targetCallID = strings.TrimSpace(targetCallID)
	if turn == nil || call == nil || targetCallID == "" {
		return false
	}
	current := strings.TrimSpace(call.RetryOfCallID)
	if current == "" {
		return false
	}
	seen := make(map[string]struct{})
	for current != "" {
		if current == targetCallID {
			return true
		}
		if _, duplicated := seen[current]; duplicated {
			return false
		}
		seen[current] = struct{}{}
		next := turn.ToolCalls[current]
		if next == nil {
			return false
		}
		current = strings.TrimSpace(next.RetryOfCallID)
	}
	return false
}

func isMutationToolCall(call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	switch call.ToolName {
	case "write", "edit", "apply_patch":
		return true
	case "bash":
		return outcomeCategoryForTool(call) == toolOutcomeMutation
	default:
		return false
	}
}

func isFailedMutationToolCall(call *events.ToolCallState) bool {
	return isMutationToolCall(call) && call.Completed && strings.TrimSpace(call.Error) != ""
}

func mutationCallPaths(call *events.ToolCallState) []string {
	if call == nil {
		return nil
	}
	if strings.TrimSpace(call.ToolName) == "mkdir" {
		return nil
	}
	if presenter, ok := toolPresenterForCall(call); ok && presenter.MutationPaths != nil {
		return presenter.MutationPaths(call)
	}
	return nil
}

func normalizeMutationPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func sameMutationPathSet(left, right []string) bool {
	left = uniqueComparableMutationPaths(left)
	right = uniqueComparableMutationPaths(right)
	if len(left) == 0 || len(right) == 0 || len(left) != len(right) {
		return false
	}
	used := make([]bool, len(right))
	for _, leftPath := range left {
		matched := false
		for idx, rightPath := range right {
			if used[idx] || !equivalentMutationPath(leftPath, rightPath) {
				continue
			}
			used[idx] = true
			matched = true
			break
		}
		if !matched {
			return false
		}
	}
	return true
}

func uniqueComparableMutationPaths(paths []string) []string {
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		path = normalizeMutationPath(path)
		if path == "" {
			continue
		}
		seen := false
		for _, existing := range unique {
			if equivalentMutationPath(existing, path) {
				seen = true
				break
			}
		}
		if !seen {
			unique = append(unique, path)
		}
	}
	return unique
}

func equivalentMutationPath(left, right string) bool {
	left = normalizeMutationPath(left)
	right = normalizeMutationPath(right)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	return strings.HasSuffix(left, "/"+right) || strings.HasSuffix(right, "/"+left)
}
