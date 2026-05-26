package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
)

func (r *TurnRunner) appendStepToolCallBatch(ctx context.Context, sessionID, turnID string, batch stepToolBatch) ([]string, error) {
	callIDs := batch.CallIDs()
	if len(callIDs) < 2 {
		return callIDs, nil
	}
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeToolCallBatch,
		Payload: events.ToolCallBatchPayload{
			CallIDs: callIDs,
		},
	})
	return callIDs, err
}
