package app

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func permissionDeniedMessage(request events.PermissionRequestedPayload) string {
	switch request.Kind {
	case events.PermissionRequestKindExecution:
		return fmt.Sprintf("permission denied for execution command %s", request.Command)
	case events.PermissionRequestKindNetwork:
		return fmt.Sprintf("permission denied for network target %s", request.Path)
	default:
		return fmt.Sprintf("permission denied for %s path %s", request.Access, request.Path)
	}
}

func executionApprovalDeniedMessage(request events.ExecutionApprovalRequestedPayload, decision events.ExecutionApprovalDecision) string {
	subject := "command"
	switch decision {
	case events.ExecutionApprovalDecisionCancel:
		return fmt.Sprintf("execution approval cancelled for %s %s", subject, request.Command)
	default:
		return fmt.Sprintf("execution approval denied for %s %s", subject, request.Command)
	}
}

func resumeToolResultOutput(output string, request events.PermissionRequestedPayload, decision events.PermissionDecision, scope events.PermissionScope, executionRequest *events.ExecutionApprovalRequestedPayload, executionDecision events.ExecutionApprovalDecision) string {
	note := strings.TrimSpace(resumePermissionNote(request, decision, scope, executionRequest, executionDecision))
	if note == "" || strings.TrimSpace(output) == "" {
		return output
	}
	return note + "\n\n" + output
}

func resumeToolResultError(errorText string, request events.PermissionRequestedPayload, decision events.PermissionDecision, scope events.PermissionScope, executionRequest *events.ExecutionApprovalRequestedPayload, executionDecision events.ExecutionApprovalDecision) string {
	note := strings.TrimSpace(resumePermissionNote(request, decision, scope, executionRequest, executionDecision))
	if note == "" || strings.TrimSpace(errorText) == "" {
		return errorText
	}
	return note + "\n\n" + errorText
}

func resumePermissionNote(request events.PermissionRequestedPayload, decision events.PermissionDecision, scope events.PermissionScope, executionRequest *events.ExecutionApprovalRequestedPayload, executionDecision events.ExecutionApprovalDecision) string {
	if executionRequest != nil {
		switch executionDecision {
		case events.ExecutionApprovalDecisionAccept:
			return fmt.Sprintf(
				"Runtime note: Command execution %q from %s was explicitly approved for this turn. The following tool result is authorized. Use it directly.",
				executionRequest.Command,
				executionRequest.WorkingDirectory,
			)
		case events.ExecutionApprovalDecisionAcceptForSession:
			return fmt.Sprintf(
				"Runtime note: Command execution %q from %s was explicitly approved for this session. The following tool result is authorized. Use it directly.",
				executionRequest.Command,
				executionRequest.WorkingDirectory,
			)
		case events.ExecutionApprovalDecisionAcceptWithExecPolicy:
			return fmt.Sprintf(
				"Runtime note: Command execution %q from %s was approved with a runtime execution policy amendment. The following tool result is authorized. Use it directly.",
				executionRequest.Command,
				executionRequest.WorkingDirectory,
			)
		case events.ExecutionApprovalDecisionApplyNetworkPolicy:
			return fmt.Sprintf(
				"Runtime note: Command execution %q from %s was approved with a runtime network policy amendment. The following tool result is authorized. Use it directly.",
				executionRequest.Command,
				executionRequest.WorkingDirectory,
			)
		case events.ExecutionApprovalDecisionDecline:
			return fmt.Sprintf(
				"Runtime note: Command execution %q from %s was explicitly denied. The following tool result reflects that denial.",
				executionRequest.Command,
				executionRequest.WorkingDirectory,
			)
		case events.ExecutionApprovalDecisionCancel:
			return fmt.Sprintf(
				"Runtime note: Command execution %q from %s was cancelled before running. The following tool result reflects that cancellation.",
				executionRequest.Command,
				executionRequest.WorkingDirectory,
			)
		}
	}
	switch decision {
	case events.PermissionDecisionApproved:
		switch request.Kind {
		case events.PermissionRequestKindExecution:
			return fmt.Sprintf(
				"Runtime note: Command execution %q from %s was explicitly approved %s. The following tool result is authorized. Use it directly instead of refusing based on default workspace restrictions.",
				request.Command,
				request.WorkingDirectory,
				resumePermissionScopeLabel(scope),
			)
		case events.PermissionRequestKindNetwork:
			return fmt.Sprintf(
				"Runtime note: External network target %s was explicitly approved %s. The following tool result is authorized. Use it directly instead of refusing based on default workspace restrictions.",
				request.Path,
				resumePermissionScopeLabel(scope),
			)
		case events.PermissionRequestKindPath:
			if strings.TrimSpace(request.Path) == "" || strings.TrimSpace(request.Access) == "" {
				return ""
			}
			return fmt.Sprintf(
				"Runtime note: External %s access to %s was explicitly approved %s. The following tool result is authorized. Use it directly instead of refusing based on default workspace restrictions.",
				request.Access,
				request.Path,
				resumePermissionScopeLabel(scope),
			)
		default:
			return ""
		}
	case events.PermissionDecisionDenied:
		switch request.Kind {
		case events.PermissionRequestKindExecution:
			return fmt.Sprintf(
				"Runtime note: Command execution %q from %s was explicitly denied %s. The following tool result reflects that denial.",
				request.Command,
				request.WorkingDirectory,
				resumePermissionScopeLabel(scope),
			)
		case events.PermissionRequestKindNetwork:
			return fmt.Sprintf(
				"Runtime note: External network target %s was explicitly denied %s. The following tool result reflects that denial.",
				request.Path,
				resumePermissionScopeLabel(scope),
			)
		case events.PermissionRequestKindPath:
			if strings.TrimSpace(request.Path) == "" || strings.TrimSpace(request.Access) == "" {
				return ""
			}
			return fmt.Sprintf(
				"Runtime note: External %s access to %s was explicitly denied %s. The following tool result reflects that denial.",
				request.Access,
				request.Path,
				resumePermissionScopeLabel(scope),
			)
		default:
			return ""
		}
	default:
		return ""
	}
}

func executionDecisionApproved(decision events.ExecutionApprovalDecision) bool {
	switch decision {
	case events.ExecutionApprovalDecisionAccept, events.ExecutionApprovalDecisionAcceptForSession, events.ExecutionApprovalDecisionAcceptWithExecPolicy, events.ExecutionApprovalDecisionApplyNetworkPolicy:
		return true
	default:
		return false
	}
}

func resumePermissionScopeLabel(scope events.PermissionScope) string {
	switch scope {
	case events.PermissionScopeSession:
		return "for this session"
	case events.PermissionScopeOnce:
		return "for this turn"
	default:
		return "for this turn"
	}
}
