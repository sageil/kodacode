package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func findPendingNetworkPermission(state events.SessionState, target, toolName string) *events.PermissionRequestState {
	target = strings.TrimSpace(target)
	for _, requestID := range state.PendingPermissionOrder {
		request := state.PendingPermissions[requestID]
		if request == nil {
			continue
		}
		if request.Kind == events.PermissionRequestKindNetwork && request.Path == target && request.ToolName == toolName {
			return request
		}
	}
	return nil
}

func findPendingPathPermission(state events.SessionState, path, access, toolName string) *events.PermissionRequestState {
	path = strings.TrimSpace(path)
	access = strings.TrimSpace(access)
	for _, requestID := range state.PendingPermissionOrder {
		request := state.PendingPermissions[requestID]
		if request == nil {
			continue
		}
		if request.Kind != events.PermissionRequestKindPath {
			continue
		}
		if request.Path == path && request.Access == access && request.ToolName == toolName {
			return request
		}
	}
	return nil
}

func findPendingExecutionPermission(state events.SessionState, executionID, toolName string) *events.ExecutionApprovalState {
	executionID = strings.TrimSpace(executionID)
	for _, requestID := range state.PendingExecutionOrder {
		request := state.PendingExecutions[requestID]
		if request == nil {
			continue
		}
		if request.ExecutionID == executionID && request.ToolName == toolName {
			return request
		}
	}
	return nil
}

func pendingPermissionRequestState(state events.SessionState, requestID string) *events.PermissionRequestState {
	return state.PendingPermissions[requestID]
}

func pendingExecutionApprovalState(state events.SessionState, requestID string) *events.ExecutionApprovalState {
	return state.PendingExecutions[requestID]
}

func pendingInteractionTurnID(state events.SessionState, requestID string) string {
	if request := pendingExecutionApprovalState(state, requestID); request != nil {
		return strings.TrimSpace(request.TurnID)
	}
	if request := pendingPermissionRequestState(state, requestID); request != nil {
		return strings.TrimSpace(request.TurnID)
	}
	if request := pendingQuestionRequestState(state, requestID); request != nil {
		return strings.TrimSpace(request.TurnID)
	}
	return ""
}

func networkTargetGranted(state events.SessionState, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, grant := range state.NetworkGrants {
		if strings.TrimSpace(grant.Target) == target {
			return true
		}
	}
	return false
}

func executionGrantAllowed(state events.SessionState, prefixRule, sessionPaths, networkTargets []string) bool {
	if len(prefixRule) == 0 {
		return false
	}
	for _, grant := range state.ExecutionGrants {
		if slicesEqualFold(grant.PrefixRule, prefixRule) &&
			stringSliceSubsetFold(sessionPaths, grant.SessionPaths) &&
			stringSliceSubsetFold(networkTargets, grant.NetworkTargets) {
			return true
		}
	}
	return false
}

func cloneExecutionNetworkPolicy(input *events.ExecutionNetworkPolicyAmendment) *events.ExecutionNetworkPolicyAmendment {
	if input == nil {
		return nil
	}
	copyInput := *input
	return &copyInput
}

func slicesEqualFold(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if !strings.EqualFold(strings.TrimSpace(left[idx]), strings.TrimSpace(right[idx])) {
			return false
		}
	}
	return true
}

func stringSliceSubsetFold(required, available []string) bool {
	if len(required) == 0 {
		return true
	}
	if len(available) == 0 {
		return false
	}
	for _, candidate := range required {
		found := false
		for _, existing := range available {
			if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(existing)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
