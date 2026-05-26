package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

func (s *ExecutionService) authorizeExecution(ctx context.Context, input ExecuteToolInput, state events.SessionState, resolved resolvedExecutionRequest) (ToolExecutionResult, bool, error) {
	mode := statePermissionMode(state, s.config.PermissionMode)
	if permissionModeGrantsFullAccess(mode) {
		return ToolExecutionResult{}, false, nil
	}
	if exactExecutionApprovalAllowed(state, input, resolved) {
		return ToolExecutionResult{}, false, nil
	}
	temporaryGrants := append([]workspace.Grant(nil), input.TemporaryGrants...)
	for _, request := range resolved.Request.PathRequests {
		authorized, err := s.sessions.AuthorizePath(ctx, PathAuthorizationInput{
			SessionID:       input.SessionID,
			TurnID:          input.TurnID,
			ToolCallID:      input.ToolCallID,
			Path:            request.Path,
			Access:          request.Access,
			ToolName:        input.ToolName,
			Command:         resolved.Request.Preview,
			Reason:          request.Reason,
			TemporaryGrants: append([]workspace.Grant(nil), temporaryGrants...),
		})
		if err != nil {
			return ToolExecutionResult{}, true, err
		}
		if len(authorized.Grants) > 0 {
			temporaryGrants = appendUniqueWorkspaceGrants(temporaryGrants, authorized.Grants...)
		}
		if authorized.Status == AuthorizationStatusPending {
			return ToolExecutionResult{
				Status:           ToolExecutionStatusPending,
				PendingRequestID: authorized.RequestID,
			}, true, nil
		}
	}
	scope, err := scopeFromState(state, temporaryGrants...)
	if err != nil {
		return ToolExecutionResult{}, true, err
	}

	scopeApprovalNeeded := false
	for _, request := range contractPathRequests(resolved.Contract) {
		decision, err := scope.Check(request.Access, request.Path)
		if err != nil {
			return ToolExecutionResult{}, false, err
		}
		if !decision.RequiresApproval() {
			continue
		}
		if action, ok := s.sessions.matchExternalDirectoryPolicy(state, decision.ResolvedPath); ok {
			switch action {
			case permissionpolicy.ActionAllow:
				continue
			case permissionpolicy.ActionDeny:
				return ToolExecutionResult{}, true, PermissionPolicyDeniedError{
					Subject: "permissions.external_directory",
					Value:   decision.ResolvedPath,
				}
			}
		}
		scopeApprovalNeeded = true
		break
	}
	sessionGrantPaths := executionSessionGrantPaths(sessionWorkspaceRoots(state), resolved.Contract)
	networkApprovalNeeded, err := s.executionNetworkApprovalNeeded(state, input, resolved.Request, mode)
	if err != nil {
		return ToolExecutionResult{}, true, err
	}
	opaquePathApprovalNeeded := strings.TrimSpace(resolved.Request.OpaquePathReason) != ""
	if !scopeApprovalNeeded && !networkApprovalNeeded && executionGrantAllowed(state, executionGrantRule(resolved.Request), sessionGrantPaths, resolved.Request.NetworkTargets) {
		return ToolExecutionResult{}, false, nil
	}
	commandPolicyApprovalNeeded := false
	commandPolicyAllows := false
	if resolved.Request.Kind == tool.BashToolName {
		if action, ok := s.sessions.matchBashPolicy(state, resolved.Request.Preview); ok {
			switch action {
			case permissionpolicy.ActionAllow:
				commandPolicyAllows = true
			case permissionpolicy.ActionAsk:
				commandPolicyApprovalNeeded = true
			case permissionpolicy.ActionDeny:
				return ToolExecutionResult{}, true, PermissionPolicyDeniedError{
					Subject: "permissions.bash",
					Value:   resolved.Request.Preview,
				}
			}
		}
	}
	readOnlyApprovalNeeded := permissionModeRequiresExecutionApproval(mode, state, input, resolved.Request, sessionGrantPaths) && !commandPolicyAllows
	intentApprovalNeeded := executionIntentApprovalRequired(mode, resolved.Request) && !commandPolicyAllows
	if !scopeApprovalNeeded && !readOnlyApprovalNeeded && !networkApprovalNeeded && !opaquePathApprovalNeeded && !intentApprovalNeeded && !commandPolicyApprovalNeeded {
		return ToolExecutionResult{}, false, nil
	}

	authorized, err := s.sessions.AuthorizeExecution(ctx, ExecutionAuthorizationInput{
		SessionID:             input.SessionID,
		TurnID:                input.TurnID,
		ExecutionID:           executionID(input.ToolCallID),
		ToolCallID:            input.ToolCallID,
		ToolName:              input.ToolName,
		Command:               resolved.Request.Preview,
		WorkingDir:            resolved.Contract.WorkingDirectory,
		PrefixRule:            append([]string(nil), resolved.Request.PrefixRule...),
		Reason:                executionAuthorizationReason(resolved.Request, scopeApprovalNeeded, readOnlyApprovalNeeded, networkApprovalNeeded, opaquePathApprovalNeeded, intentApprovalNeeded, commandPolicyApprovalNeeded),
		SessionGrantPaths:     append([]string(nil), sessionGrantPaths...),
		NetworkTargets:        append([]string(nil), resolved.Request.NetworkTargets...),
		AvailableDecisions:    executionApprovalOptions(resolved.Request),
		ProposedNetworkPolicy: executionApprovalProposedNetworkPolicy(networkApprovalNeeded),
	})
	if err != nil {
		return ToolExecutionResult{}, false, err
	}
	if authorized.Status == AuthorizationStatusPending {
		return ToolExecutionResult{
			Status:           ToolExecutionStatusPending,
			PendingRequestID: authorized.RequestID,
		}, true, nil
	}
	return ToolExecutionResult{}, false, nil
}

func executionApprovalOptions(request tool.ExecutionRequest) []events.ExecutionApprovalDecision {
	decisions := []events.ExecutionApprovalDecision{events.ExecutionApprovalDecisionAccept, events.ExecutionApprovalDecisionAcceptForSession}
	decisions = append(decisions, events.ExecutionApprovalDecisionDecline, events.ExecutionApprovalDecisionCancel)
	return decisions
}

func executionSessionGrantPaths(workspaceRoots []string, contract executionContract) []string {
	if strings.TrimSpace(contract.WorkingDirectory) == "" || withinSessionWorkspaceRoots(workspaceRoots, contract.WorkingDirectory) {
		return nil
	}
	return []string{contract.WorkingDirectory}
}

func executionGrantRule(request tool.ExecutionRequest) []string {
	if len(request.PrefixRule) > 0 {
		return append([]string(nil), request.PrefixRule...)
	}
	if preview := strings.TrimSpace(request.Preview); preview != "" {
		return []string{preview}
	}
	return nil
}

func contractPathRequests(contract executionContract) []contractPathRequest {
	if strings.TrimSpace(contract.WorkingDirectory) == "" {
		return nil
	}
	return []contractPathRequest{{
		Access: workspace.AccessWorkdir,
		Path:   contract.WorkingDirectory,
		Reason: "requires external working directory access for command execution",
	}}
}

func executionApprovalProposedNetworkPolicy(required bool) *events.ExecutionNetworkPolicyAmendment {
	if !required {
		return nil
	}
	return &events.ExecutionNetworkPolicyAmendment{Enabled: true}
}

func executionNetworkAuthorized(config ExecutionConfig, state events.SessionState, request tool.ExecutionRequest, networkPolicy *events.ExecutionNetworkPolicyAmendment, mode PermissionMode) bool {
	if permissionModeGrantsFullAccess(mode) || config.Network == ExecutionNetworkEnabled {
		return true
	}
	if networkPolicy != nil && networkPolicy.Enabled {
		return true
	}
	if len(request.NetworkTargets) == 0 {
		return executionCommandScopedNetworkGranted(state, request)
	}
	for _, target := range request.NetworkTargets {
		if !networkTargetGranted(state, target) {
			return false
		}
	}
	return true
}

func permissionModeRequiresExecutionApproval(mode PermissionMode, state events.SessionState, input ExecuteToolInput, request tool.ExecutionRequest, sessionGrantPaths []string) bool {
	if normalizePermissionMode(mode) != PermissionModeReadOnly {
		return false
	}
	if input.ExecutionExecPolicy != nil || input.ExecutionNetworkPolicy != nil || len(input.TemporaryGrants) > 0 {
		return false
	}
	return !executionGrantAllowed(state, executionGrantRule(request), sessionGrantPaths, request.NetworkTargets)
}

func executionIntentApprovalRequired(mode PermissionMode, request tool.ExecutionRequest) bool {
	if normalizePermissionMode(mode) != PermissionModeAuto {
		return false
	}
	switch request.Intent {
	case tool.ExecutionIntentServer, tool.ExecutionIntentWatcher:
		return true
	default:
		return false
	}
}

func (s *ExecutionService) executionNetworkApprovalNeeded(state events.SessionState, input ExecuteToolInput, request tool.ExecutionRequest, mode PermissionMode) (bool, error) {
	if len(request.NetworkTargets) == 0 {
		return false, nil
	}
	unresolvedTargets := make([]string, 0, len(request.NetworkTargets))
	for _, target := range request.NetworkTargets {
		target = strings.TrimSpace(target)
		if target == "" || networkTargetGranted(state, target) {
			continue
		}
		if action, ok := s.sessions.matchNetworkTargetPolicy(state, target); ok {
			switch action {
			case permissionpolicy.ActionAllow:
				continue
			case permissionpolicy.ActionAsk:
				return true, nil
			case permissionpolicy.ActionDeny:
				return false, PermissionPolicyDeniedError{
					Subject: "permissions.network_target",
					Value:   target,
				}
			}
		}
		unresolvedTargets = append(unresolvedTargets, target)
	}
	if len(unresolvedTargets) == 0 {
		return false, nil
	}
	unresolvedRequest := request
	unresolvedRequest.NetworkTargets = unresolvedTargets
	return !executionNetworkAuthorized(s.config, state, unresolvedRequest, input.ExecutionNetworkPolicy, mode), nil
}

func executionAuthorizationReason(request tool.ExecutionRequest, scopeApprovalNeeded, readOnlyApprovalNeeded, networkApprovalNeeded, opaquePathApprovalNeeded, intentApprovalNeeded, commandPolicyApprovalNeeded bool) string {
	if commandPolicyApprovalNeeded && !scopeApprovalNeeded && !networkApprovalNeeded && !opaquePathApprovalNeeded && !intentApprovalNeeded {
		return "requires approval by permissions.bash policy"
	}
	if intentApprovalNeeded {
		switch request.Intent {
		case tool.ExecutionIntentServer:
			return "requires approval to start a persistent local server"
		case tool.ExecutionIntentWatcher:
			return "requires approval to start a persistent watch process"
		}
	}
	if opaquePathApprovalNeeded {
		return request.OpaquePathReason
	}
	possibleNetwork := executionUsesCommandScopedNetworkTarget(request.NetworkTargets)
	switch {
	case networkApprovalNeeded && possibleNetwork && (scopeApprovalNeeded || readOnlyApprovalNeeded):
		return "requires approval for command execution and possible network access"
	case networkApprovalNeeded && (scopeApprovalNeeded || readOnlyApprovalNeeded):
		return "requires approval for command execution and network access"
	case networkApprovalNeeded && possibleNetwork:
		return "requires approval for possible network access"
	case networkApprovalNeeded:
		return "requires approval for network access"
	case readOnlyApprovalNeeded:
		return "requires approval in read-only mode"
	default:
		return "requires approval for command execution"
	}
}

func executionUsesCommandScopedNetworkTarget(targets []string) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if !strings.HasPrefix(strings.TrimSpace(target), "command: ") {
			return false
		}
	}
	return true
}

func executionCommandScopedNetworkGranted(state events.SessionState, request tool.ExecutionRequest) bool {
	target := tool.CommandScopedNetworkTarget(request.Preview, request.PrefixRule)
	if strings.TrimSpace(target) == "" {
		return false
	}
	return networkTargetGranted(state, target)
}
