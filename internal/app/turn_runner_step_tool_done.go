package app

import (
	"context"

	"github.com/sageil/kodacode/internal/provider"
)

type stepToolDoneInput struct {
	SessionID          string
	TurnID             string
	Event              provider.Event
	Collector          *stepToolCallCollector
	Batch              *stepToolBatch
	Preview            stepAssistantPreview
	CapabilityResolver stepToolCapabilityResolver
}

type stepToolDoneResult struct {
	Accepted           bool
	CompletionTokens   int
	ContinueCollecting bool
}

func (r *TurnRunner) handleStepToolCallDone(ctx context.Context, input stepToolDoneInput) (stepToolDoneResult, error) {
	if err := input.Preview.StartToolStep(input.Event.ToolName); err != nil {
		return stepToolDoneResult{}, err
	}
	stepCall := input.Collector.completeToolCall(input.Event)
	if err := handleStepQuestionToolPreview(input.Event.ToolName, stepCall.Arguments, input.Preview.CommitAssistant, input.Preview.Discard); err != nil {
		return stepToolDoneResult{}, err
	}

	admission := admitStepToolCallWithResolver(input.Batch, stepCall, input.CapabilityResolver)
	result := stepToolDoneResult{
		Accepted:         admission.Accepted,
		CompletionTokens: admission.CompletionTokens,
	}
	if !admission.Accepted {
		return result, nil
	}

	result.ContinueCollecting = true
	return result, nil
}
