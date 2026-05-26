package app

import "github.com/sageil/kodacode/internal/events"

func permissionChoiceFromInput(line string, pendingKind events.PermissionRequestKind, pendingPath string) (events.PermissionDecision, events.PermissionScope, string, bool) {
	switch line {
	case "1":
		return events.PermissionDecisionApproved, events.PermissionScopeOnce, "", false
	case "2":
		return events.PermissionDecisionApproved, events.PermissionScopeSession, sessionGrantTarget(pendingKind, pendingPath), false
	default:
		return events.PermissionDecisionDenied, "", "", false
	}
}

func sessionGrantTarget(kind events.PermissionRequestKind, pendingPath string) string {
	if kind == events.PermissionRequestKindExecution || kind == events.PermissionRequestKindNetwork {
		return ""
	}
	return pendingPath
}
