package app

import (
	"context"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type RecordWorkspaceWriteRestoreInput struct {
	SessionID    string
	SourceTurnID string
	Restores     []events.WorkspaceWriteRestoreItem
}

func (s *SessionService) RecordWorkspaceWriteRestore(ctx context.Context, input RecordWorkspaceWriteRestoreInput) (events.Event, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return events.Event{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.SourceTurnID) == "" {
		return events.Event{}, ErrTurnIDRequired
	}
	if len(input.Restores) == 0 {
		return events.Event{}, errors.New("restores is required")
	}
	return s.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeWorkspaceWriteRestored,
		Payload: events.WorkspaceWriteRestoredPayload{
			SourceTurnID: input.SourceTurnID,
			Restores:     append([]events.WorkspaceWriteRestoreItem(nil), input.Restores...),
		},
	})
}
