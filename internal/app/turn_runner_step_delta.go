package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

type stepToolDeltaResult struct {
	Accepted bool
}

func (r *TurnRunner) handleStepToolCallDelta(ctx context.Context, sessionID, turnID string, event provider.Event, _ bool, collector *stepToolCallCollector, startToolStep func(string) error) (stepToolDeltaResult, error) {
	if startToolStep != nil {
		if err := startToolStep(event.ToolName); err != nil {
			return stepToolDeltaResult{}, err
		}
	}
	if collector != nil {
		collector.appendToolCallDelta(event)
	}
	if err := r.sessions.publishEphemeral(sessionID, turnID, events.TypeToolCallDelta, events.ToolCallDeltaPayload{
		CallID:     event.ToolCallID,
		ToolName:   event.ToolName,
		ToolKind:   string(inputToolKindOrDefault(event.ToolKind)),
		InputDelta: event.InputDelta,
	}); err != nil {
		return stepToolDeltaResult{}, err
	}
	return stepToolDeltaResult{Accepted: true}, nil
}
