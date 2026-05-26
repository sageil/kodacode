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

func (s *SessionService) AuthorizePath(ctx context.Context, input PathAuthorizationInput) (AuthorizationResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return AuthorizationResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return AuthorizationResult{}, ErrTurnIDRequired
	}
	if strings.TrimSpace(input.ToolCallID) == "" {
		return AuthorizationResult{}, errors.New("tool_call_id is required")
	}
	if strings.TrimSpace(input.ToolName) == "" {
		return AuthorizationResult{}, errors.New("tool_name is required")
	}
	if input.Access == "" {
		return AuthorizationResult{}, errors.New("access is required")
	}
	if strings.TrimSpace(input.Path) == "" {
		return AuthorizationResult{}, errors.New("path is required")
	}

	state, err := s.Snapshot(ctx, input.SessionID)
	if err != nil {
		return AuthorizationResult{}, err
	}
	scope, err := scopeFromState(state, input.TemporaryGrants...)
	if err != nil {
		return AuthorizationResult{}, err
	}
	decision, err := scope.Authorize(input.Access, input.Path)
	if err == nil {
		return AuthorizationResult{
			Status:   AuthorizationStatusAuthorized,
			Decision: decision,
		}, nil
	}
	if !isPermissionRequired(err) {
		return AuthorizationResult{}, err
	}
	if action, ok := s.matchExternalDirectoryPolicy(state, decision.ResolvedPath); ok {
		switch action {
		case permissionpolicy.ActionAllow:
			return AuthorizationResult{
				Status:   AuthorizationStatusAuthorized,
				Decision: decision,
				Grants:   policyTemporaryGrants(decision.ResolvedPath),
			}, nil
		case permissionpolicy.ActionDeny:
			return AuthorizationResult{}, PermissionPolicyDeniedError{
				Subject: "permissions.external_directory",
				Value:   decision.ResolvedPath,
			}
		}
	}
	if existing := findPendingPathPermission(state, decision.ResolvedPath, string(input.Access), input.ToolName); existing != nil {
		return AuthorizationResult{
			Status:    AuthorizationStatusPending,
			RequestID: existing.RequestID,
		}, nil
	}

	command := strings.TrimSpace(input.Command)
	if command == "" {
		command = input.ToolName + " " + decision.ResolvedPath
	}
	requestID := fmt.Sprintf("perm-%d", time.Now().UTC().UnixNano())
	if _, err := s.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypePermissionRequested,
		Payload: events.PermissionRequestedPayload{
			Kind:       events.PermissionRequestKindPath,
			RequestID:  requestID,
			ToolCallID: input.ToolCallID,
			Access:     string(input.Access),
			Path:       decision.ResolvedPath,
			ToolName:   input.ToolName,
			Command:    command,
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
