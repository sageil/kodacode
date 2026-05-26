package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

func (s *SessionService) AuthorizeExecution(ctx context.Context, input ExecutionAuthorizationInput) (AuthorizationResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return AuthorizationResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return AuthorizationResult{}, ErrTurnIDRequired
	}
	if strings.TrimSpace(input.ExecutionID) == "" {
		return AuthorizationResult{}, errors.New("execution_id is required")
	}

	state, err := s.Snapshot(ctx, input.SessionID)
	if err != nil {
		return AuthorizationResult{}, err
	}
	if existing := findPendingExecutionPermission(state, input.ExecutionID, input.ToolName); existing != nil {
		return AuthorizationResult{
			Status:    AuthorizationStatusPending,
			RequestID: existing.RequestID,
		}, nil
	}

	requestID := fmt.Sprintf("perm-%d", time.Now().UTC().UnixNano())
	if _, err := s.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeExecutionApprovalRequested,
		Payload: events.ExecutionApprovalRequestedPayload{
			RequestID:             requestID,
			ExecutionID:           input.ExecutionID,
			ToolCallID:            input.ToolCallID,
			WorkingDirectory:      input.WorkingDir,
			ToolName:              input.ToolName,
			Command:               input.Command,
			Reason:                input.Reason,
			PrefixRule:            append([]string(nil), input.PrefixRule...),
			SessionGrantPaths:     append([]string(nil), input.SessionGrantPaths...),
			NetworkTargets:        append([]string(nil), input.NetworkTargets...),
			AvailableDecisions:    append([]events.ExecutionApprovalDecision(nil), input.AvailableDecisions...),
			ProposedExecPolicy:    input.ProposedExecPolicy,
			ProposedNetworkPolicy: input.ProposedNetworkPolicy,
		},
	}); err != nil {
		return AuthorizationResult{}, err
	}
	return AuthorizationResult{
		Status:    AuthorizationStatusPending,
		RequestID: requestID,
	}, nil
}
