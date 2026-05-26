package app

import "github.com/sageil/kodacode/internal/provider"

type stepAssistantDeltaResult struct {
	Accepted         bool
	CompletionTokens int
}

func (r *TurnRunner) handleStepAssistantDelta(sessionID, turnID string, event provider.Event, hasBatchableResults, hasToolCalls bool, assistantSegment *string) (stepAssistantDeltaResult, error) {
	result := stepAssistantDeltaResult{
		CompletionTokens: provider.EstimateTextTokens(event.AssistantDelta),
	}
	if hasBatchableResults {
		result.Accepted = true
		return result, nil
	}
	if hasToolCalls {
		result.Accepted = true
		return result, nil
	}
	if assistantSegment != nil {
		*assistantSegment += event.AssistantDelta
	}
	if err := r.appendAssistantPreviewDelta(sessionID, turnID, event.AssistantDelta); err != nil {
		return stepAssistantDeltaResult{}, err
	}
	result.Accepted = true
	return result, nil
}
