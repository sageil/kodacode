package app

import (
	"context"

	"github.com/sageil/kodacode/internal/provider"
)

type stepReasoningDeltaResult struct {
	Accepted         bool
	DurableProgress  bool
	CompletionTokens int
}

func (r *TurnRunner) handleStepReasoningDelta(ctx context.Context, sessionID, turnID string, event provider.Event, hasBatchableResults bool, collector *stepToolCallCollector) (stepReasoningDeltaResult, error) {
	result := stepReasoningDeltaResult{
		CompletionTokens: provider.EstimateTextTokens(event.ReasoningDelta),
	}
	if hasBatchableResults {
		result.Accepted = true
		return result, nil
	}
	if collector != nil {
		collector.appendOpenAIReasoningDelta(event.ReasoningDelta)
	}
	if err := r.appendReasoningDelta(ctx, sessionID, turnID, event.ReasoningDelta, event.ReasoningSegmentID); err != nil {
		return stepReasoningDeltaResult{}, err
	}
	result.Accepted = true
	result.DurableProgress = true
	return result, nil
}
