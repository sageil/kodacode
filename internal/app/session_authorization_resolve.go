package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/workspace"
)

func (s *SessionService) ResolvePermission(ctx context.Context, input ResolvePermissionInput) (events.Event, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return events.Event{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return events.Event{}, ErrTurnIDRequired
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return events.Event{}, ErrPermissionRequestMissing
	}

	state, err := s.Snapshot(ctx, input.SessionID)
	if err != nil {
		return events.Event{}, err
	}
	scope, err := scopeFromState(state)
	if err != nil {
		return events.Event{}, err
	}

	if executionRequest := pendingExecutionApprovalState(state, input.RequestID); executionRequest != nil {
		turnID := strings.TrimSpace(executionRequest.TurnID)
		if turnID == "" {
			turnID = input.TurnID
		}
		executionDecision := input.ExecutionDecision
		if executionDecision == "" {
			executionDecision = executionDecisionFromPermissionInput(input.Decision, input.Scope)
		}
		payload := events.ExecutionApprovalResolvedPayload{
			RequestID:            input.RequestID,
			Decision:             executionDecision,
			AppliedExecPolicy:    input.ExecutionExecPolicy,
			AppliedNetworkPolicy: input.ExecutionNetworkPolicy,
		}
		if executionRequest.ProposedNetworkPolicy != nil &&
			(payload.Decision == events.ExecutionApprovalDecisionAccept || payload.Decision == events.ExecutionApprovalDecisionAcceptForSession) &&
			payload.AppliedNetworkPolicy == nil {
			payload.AppliedNetworkPolicy = cloneExecutionNetworkPolicy(executionRequest.ProposedNetworkPolicy)
		}
		if payload.Decision == events.ExecutionApprovalDecisionAcceptForSession {
			if len(executionRequest.PrefixRule) > 0 {
				payload.GrantPrefixRule = append([]string(nil), executionRequest.PrefixRule...)
			} else if strings.TrimSpace(executionRequest.Command) != "" {
				payload.GrantPrefixRule = []string{strings.TrimSpace(executionRequest.Command)}
			}
			grantPaths, err := resolveExecutionApprovalGrantPaths(scope, executionRequest)
			if err != nil {
				return events.Event{}, err
			}
			payload.GrantPaths = append([]string(nil), grantPaths...)
			if len(executionRequest.NetworkTargets) > 0 {
				payload.GrantNetworkTargets = append([]string(nil), executionRequest.NetworkTargets...)
			}
		}
		return s.append(ctx, events.Draft{
			SessionID: input.SessionID,
			TurnID:    turnID,
			Type:      events.TypeExecutionApprovalResolved,
			Payload:   payload,
		})
	}

	request := pendingPermissionRequestState(state, input.RequestID)
	if request == nil {
		return events.Event{}, ErrPermissionRequestMissing
	}
	turnID := strings.TrimSpace(request.TurnID)
	if turnID == "" {
		turnID = input.TurnID
	}

	payload := events.PermissionResolvedPayload{
		RequestID: input.RequestID,
		Decision:  input.Decision,
		Scope:     input.Scope,
		Recursive: input.Recursive,
	}
	if input.Decision == events.PermissionDecisionApproved && input.Scope == events.PermissionScopeSession && request.Kind != events.PermissionRequestKindNetwork {
		grantPaths, recursive, err := resolvePermissionGrantPaths(state, scope, request, input)
		if err != nil {
			return events.Event{}, err
		}
		if len(grantPaths) == 1 {
			payload.GrantPath = grantPaths[0]
		}
		if len(grantPaths) > 1 {
			payload.GrantPaths = append([]string(nil), grantPaths...)
		}
		payload.Recursive = recursive
	}

	return s.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    turnID,
		Type:      events.TypePermissionResolved,
		Payload:   payload,
	})
}

func executionDecisionFromPermissionInput(decision events.PermissionDecision, scope events.PermissionScope) events.ExecutionApprovalDecision {
	switch decision {
	case events.PermissionDecisionApproved:
		if scope == events.PermissionScopeSession {
			return events.ExecutionApprovalDecisionAcceptForSession
		}
		return events.ExecutionApprovalDecisionAccept
	case events.PermissionDecisionDenied:
		return events.ExecutionApprovalDecisionDecline
	default:
		return ""
	}
}

func resolvePermissionGrantPaths(state events.SessionState, scope *workspace.Scope, request *events.PermissionRequestState, input ResolvePermissionInput) ([]string, bool, error) {
	grantPath := input.GrantPath
	if strings.TrimSpace(grantPath) == "" && request != nil {
		grantPath = request.Path
	}
	grant, err := scope.Grant(grantPath, input.Recursive)
	if err != nil {
		return nil, false, err
	}
	return []string{grant.Path}, input.Recursive, nil
}

func resolveExecutionApprovalGrantPaths(scope *workspace.Scope, request *events.ExecutionApprovalState) ([]string, error) {
	if request == nil {
		return nil, nil
	}
	paths := uniqueStrings(request.SessionGrantPaths)
	if len(paths) == 0 {
		return nil, nil
	}
	grants := make([]string, 0, len(paths))
	for _, path := range paths {
		grant, err := scope.Grant(path, false)
		if err != nil {
			return nil, err
		}
		grants = append(grants, grant.Path)
	}
	return grants, nil
}
