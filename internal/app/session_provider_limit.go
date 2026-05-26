package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func (s *SessionService) SetProviderRequestLimitDisabled(ctx context.Context, sessionID, turnID string, disabled bool) error {
	if strings.TrimSpace(sessionID) == "" {
		return ErrSessionIDRequired
	}
	if strings.TrimSpace(turnID) == "" {
		return ErrTurnIDRequired
	}
	state, err := s.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	if state.ProviderRequestLimitDisabled == disabled {
		return nil
	}
	_, err = s.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeSessionProviderLimitUpdated,
		Payload: events.SessionProviderLimitUpdatedPayload{
			ProviderRequestLimitDisabled: disabled,
		},
	})
	return err
}
