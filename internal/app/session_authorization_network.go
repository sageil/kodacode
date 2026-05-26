package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/permissionpolicy"
)

func (s *SessionService) AuthorizeNetwork(ctx context.Context, input NetworkAuthorizationInput) (AuthorizationResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return AuthorizationResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return AuthorizationResult{}, ErrTurnIDRequired
	}
	target := strings.TrimSpace(input.Target)
	if target == "" {
		return AuthorizationResult{}, errors.New("network target is required")
	}

	state, err := s.Snapshot(ctx, input.SessionID)
	if err != nil {
		return AuthorizationResult{}, err
	}
	if permissionModeGrantsFullAccess(statePermissionMode(state, PermissionModeAuto)) {
		return AuthorizationResult{Status: AuthorizationStatusAuthorized}, nil
	}
	if stringSliceSubsetFold([]string{target}, input.TemporaryNetworkTargets) {
		return AuthorizationResult{Status: AuthorizationStatusAuthorized}, nil
	}
	if networkTargetGranted(state, target) {
		return AuthorizationResult{Status: AuthorizationStatusAuthorized}, nil
	}
	if decision := s.matchNetworkPolicy(state, input); decision.Matched {
		switch decision.Action {
		case permissionpolicy.ActionAllow:
			return AuthorizationResult{Status: AuthorizationStatusAuthorized}, nil
		case permissionpolicy.ActionDeny:
			return AuthorizationResult{}, PermissionPolicyDeniedError{Subject: decision.Subject, Value: decision.Value}
		}
	}
	if existing := findPendingNetworkPermission(state, target, input.ToolName); existing != nil {
		return AuthorizationResult{
			Status:    AuthorizationStatusPending,
			RequestID: existing.RequestID,
		}, nil
	}

	requestID := fmt.Sprintf("perm-%d", time.Now().UTC().UnixNano())
	if _, err := s.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypePermissionRequested,
		Payload: events.PermissionRequestedPayload{
			Kind:       events.PermissionRequestKindNetwork,
			RequestID:  requestID,
			ToolCallID: input.ToolCallID,
			Path:       target,
			ToolName:   input.ToolName,
			Command:    input.Command,
			Reason:     input.Reason,
		},
	}); err != nil {
		return AuthorizationResult{}, err
	}
	return AuthorizationResult{
		Status:    AuthorizationStatusPending,
		RequestID: requestID,
	}, nil
}
