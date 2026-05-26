package app

import (
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/workspace"
)

func executionDeclarationMatches(state events.SessionState, turnID, callID string, resolved resolvedExecutionRequest) bool {
	turn := state.Turns[turnID]
	if turn == nil {
		return false
	}
	call := turn.ToolCalls[callID]
	if call == nil || call.Execution == nil {
		return false
	}
	execution := call.Execution
	if execution.ExecutionID != executionID(callID) ||
		execution.Kind != resolved.Request.Kind ||
		execution.Intent != string(resolved.Request.Intent) ||
		execution.ToolName != "" && execution.ToolName != call.ToolName ||
		execution.WorkingDirectory != resolved.Contract.WorkingDirectory ||
		execution.TimeoutMS != resolved.Contract.Timeout.Milliseconds() ||
		execution.OutputLimit != resolved.Contract.OutputLimit {
		return false
	}
	return slicesEqual(execution.Command, resolved.Contract.Command)
}

func executionID(callID string) string {
	return "exec-" + strings.TrimSpace(callID)
}

func isPermissionRequired(err error) bool {
	return errors.Is(err, workspace.ErrPermissionRequired)
}

func exactExecutionApprovalAllowed(state events.SessionState, input ExecuteToolInput, resolved resolvedExecutionRequest) bool {
	approved := state.ApprovedExecutions[executionID(input.ToolCallID)]
	if approved == nil {
		return false
	}
	if approved.TurnID != input.TurnID || approved.ToolCallID != input.ToolCallID || approved.ToolName != input.ToolName {
		return false
	}
	if strings.TrimSpace(approved.Command) != strings.TrimSpace(resolved.Request.Preview) {
		return false
	}
	if strings.TrimSpace(approved.WorkingDirectory) != strings.TrimSpace(resolved.Contract.WorkingDirectory) {
		return false
	}
	if !executionPolicyAmendmentsEqual(approved.AppliedExecPolicy, input.ExecutionExecPolicy) {
		return false
	}
	if !executionNetworkPolicyAmendmentsEqual(approved.AppliedNetworkPolicy, input.ExecutionNetworkPolicy) {
		return false
	}
	return true
}

func executionPolicyAmendmentsEqual(left, right *events.ExecutionPolicyAmendment) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return boolPointersEqual(left.AllowLoginShell, right.AllowLoginShell)
	}
}

func executionNetworkPolicyAmendmentsEqual(left, right *events.ExecutionNetworkPolicyAmendment) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return left.Enabled == right.Enabled
	}
}

func boolPointersEqual(left, right *bool) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
