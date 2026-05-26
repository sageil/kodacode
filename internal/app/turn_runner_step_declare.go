package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
)

func (r *TurnRunner) appendStepToolCallDeclared(ctx context.Context, sessionID, turnID string, call stepToolCall) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeToolCallDeclared,
		Payload: events.ToolCallDeclaredPayload{
			CallID:                 call.CallID,
			ToolName:               call.ToolName,
			ToolKind:               string(normalizeStepToolKind(call.ToolKind)),
			Input:                  call.Arguments,
			GoogleThoughtSignature: append([]byte(nil), call.GoogleThoughtSignature...),
			OpenAIReasoningContent: call.OpenAIReasoningContent,
		},
	})
	return err
}
