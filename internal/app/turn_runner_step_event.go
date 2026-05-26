package app

import (
	"context"

	"github.com/sageil/kodacode/internal/provider"
)

type stepStreamEventHandler struct {
	r                       *TurnRunner
	ctx                     context.Context
	sessionID               string
	turnID                  string
	model                   provider.ModelRef
	state                   *turnLoopState
	collector               *stepToolCallCollector
	batch                   *stepToolBatch
	preview                 stepAssistantPreview
	stepConversationStart   *int
	executor                stepToolExecutor
	progress                *stepToolProgress
	capabilityResolver      stepToolCapabilityResolver
	result                  *assistantRoundtripResult
	stepHasBatchableResults *bool
	stepHasToolCalls        *bool
	completionTokens        *int
	durableProgress         *bool
	commitStepState         func()
}

type stepStreamEventHandlerInput struct {
	Runner                  *TurnRunner
	Context                 context.Context
	SessionID               string
	TurnID                  string
	Model                   provider.ModelRef
	State                   *turnLoopState
	Collector               *stepToolCallCollector
	Batch                   *stepToolBatch
	Preview                 stepAssistantPreview
	StepConversationStart   *int
	Executor                stepToolExecutor
	Progress                *stepToolProgress
	CapabilityResolver      stepToolCapabilityResolver
	Result                  *assistantRoundtripResult
	StepHasBatchableResults *bool
	StepHasToolCalls        *bool
	CompletionTokens        *int
	DurableProgress         *bool
	CommitStepState         func()
}

type stepStreamEventResult struct {
	Complete bool
	Result   assistantRoundtripResult
}

func newStepStreamEventHandler(input stepStreamEventHandlerInput) stepStreamEventHandler {
	return stepStreamEventHandler{
		r:                       input.Runner,
		ctx:                     input.Context,
		sessionID:               input.SessionID,
		turnID:                  input.TurnID,
		model:                   input.Model,
		state:                   input.State,
		collector:               input.Collector,
		batch:                   input.Batch,
		preview:                 input.Preview,
		stepConversationStart:   input.StepConversationStart,
		executor:                input.Executor,
		progress:                input.Progress,
		capabilityResolver:      input.CapabilityResolver,
		result:                  input.Result,
		stepHasBatchableResults: input.StepHasBatchableResults,
		stepHasToolCalls:        input.StepHasToolCalls,
		completionTokens:        input.CompletionTokens,
		durableProgress:         input.DurableProgress,
		commitStepState:         input.CommitStepState,
	}
}

func (h stepStreamEventHandler) Handle(event provider.Event) (stepStreamEventResult, error) {
	switch event.Kind {
	case provider.EventKindAssistantDelta:
		assistantDeltaResult, err := h.r.handleStepAssistantDelta(h.sessionID, h.turnID, event, h.hasBatchableResults(), h.hasToolCalls(), h.assistantSegment())
		if err != nil {
			return stepStreamEventResult{}, err
		}
		h.addCompletionTokens(assistantDeltaResult.CompletionTokens)
		if !assistantDeltaResult.Accepted {
			return h.executePendingToolBatchAndComplete()
		}
	case provider.EventKindReasoningDelta:
		reasoningDeltaResult, err := h.r.handleStepReasoningDelta(h.ctx, h.sessionID, h.turnID, event, h.hasBatchableResults(), h.collector)
		if err != nil {
			return stepStreamEventResult{}, err
		}
		h.addCompletionTokens(reasoningDeltaResult.CompletionTokens)
		if !reasoningDeltaResult.Accepted {
			return h.executePendingToolBatchAndComplete()
		}
		if reasoningDeltaResult.DurableProgress {
			h.markDurableProgress()
		}
	case provider.EventKindAnthropicThinkingCommitted:
		committedResult, err := h.r.handleStepAnthropicThinkingCommitted(h.ctx, h.sessionID, h.turnID, event, h.hasBatchableResults(), h.state, h.stepConversationStart, h.commitStepState)
		if err != nil {
			return stepStreamEventResult{}, err
		}
		if !committedResult.Accepted {
			return h.executePendingToolBatchAndComplete()
		}
		if committedResult.DurableProgress {
			h.markDurableProgress()
		}
	case provider.EventKindOpenAIReasoningCommitted:
		committedResult, err := h.r.handleStepOpenAIReasoningCommitted(h.ctx, h.sessionID, h.turnID, event, h.hasBatchableResults(), h.state, h.stepConversationStart, h.commitStepState)
		if err != nil {
			return stepStreamEventResult{}, err
		}
		if !committedResult.Accepted {
			return h.executePendingToolBatchAndComplete()
		}
		if committedResult.DurableProgress {
			h.markDurableProgress()
		}
	case provider.EventKindToolCallDelta:
		deltaResult, err := h.r.handleStepToolCallDelta(h.ctx, h.sessionID, h.turnID, event, h.hasBatchableResults(), h.collector, h.preview.StartToolStep)
		if err != nil {
			return stepStreamEventResult{}, err
		}
		if !deltaResult.Accepted {
			return h.executePendingToolBatchAndComplete()
		}
	case provider.EventKindToolCallDone:
		doneResult, err := h.r.handleStepToolCallDone(h.ctx, stepToolDoneInput{
			SessionID:          h.sessionID,
			TurnID:             h.turnID,
			Event:              event,
			Collector:          h.collector,
			Batch:              h.batch,
			Preview:            h.preview,
			CapabilityResolver: h.capabilityResolver,
		})
		if err != nil {
			return stepStreamEventResult{}, err
		}
		h.addCompletionTokens(doneResult.CompletionTokens)
		if !doneResult.Accepted {
			return h.executePendingToolBatchAndComplete()
		}
		if doneResult.ContinueCollecting {
			h.setBatchableResults(true)
			return stepStreamEventResult{}, nil
		}
		return h.executePendingToolBatchAndComplete()
	}
	return stepStreamEventResult{}, nil
}

func (h stepStreamEventHandler) assistantSegment() *string {
	if h.preview.segment == nil {
		empty := ""
		return &empty
	}
	return h.preview.segment
}

func (h stepStreamEventHandler) hasBatchableResults() bool {
	if h.stepHasBatchableResults == nil {
		return false
	}
	return *h.stepHasBatchableResults
}

func (h stepStreamEventHandler) hasToolCalls() bool {
	if h.stepHasToolCalls == nil {
		return false
	}
	return *h.stepHasToolCalls
}

func (h stepStreamEventHandler) addCompletionTokens(tokens int) {
	if h.completionTokens != nil {
		*h.completionTokens += tokens
	}
}

func (h stepStreamEventHandler) markDurableProgress() {
	if h.durableProgress != nil {
		*h.durableProgress = true
	}
}

func (h stepStreamEventHandler) setBatchableResults(value bool) {
	if h.stepHasBatchableResults != nil {
		*h.stepHasBatchableResults = value
	}
}

func (h stepStreamEventHandler) hasPendingToolBatch() bool {
	if h.batch == nil || h.batch.Len() == 0 {
		return false
	}
	if h.progress == nil {
		return true
	}
	return h.progress.ExecutedTools == 0 && h.progress.ReusedTools == 0 && h.progress.FailedTools == 0
}

func (h stepStreamEventHandler) executePendingToolBatchAndComplete() (stepStreamEventResult, error) {
	if !h.hasPendingToolBatch() {
		return h.complete(assistantRoundtripOutcomeToolResult), nil
	}
	return h.executeToolBatchAndComplete(*h.batch)
}

func (h stepStreamEventHandler) executeInterruptedStreamSafeToolBatchAndComplete() (stepStreamEventResult, error) {
	if !h.hasPendingToolBatch() {
		return h.complete(assistantRoundtripOutcomeToolResult), nil
	}
	safe := interruptedStreamSafeStepToolBatchWithResolver(h.capabilityResolver, *h.batch)
	if safe.Len() == 0 {
		return stepStreamEventResult{}, nil
	}
	return h.executeToolBatchAndComplete(safe)
}

func (h stepStreamEventHandler) executeToolBatchAndComplete(batch stepToolBatch) (stepStreamEventResult, error) {
	runResult, err := h.r.executeStepToolBatch(h.ctx, stepToolBatchRunInput{
		SessionID:             h.sessionID,
		TurnID:                h.turnID,
		Model:                 h.model,
		State:                 h.state,
		StepConversationStart: h.stepConversationStart,
		Executor:              h.executor,
		CapabilityResolver:    h.capabilityResolver,
		Progress:              h.progress,
		Batch:                 batch,
		CommitStepState:       h.commitStepState,
	})
	if runResult.DurableProgress {
		h.markDurableProgress()
	}
	if err != nil {
		return stepStreamEventResult{}, err
	}
	if runResult.Failed {
		return h.complete(assistantRoundtripOutcomeFailed), nil
	}
	if runResult.PendingRequestID != "" {
		return stepStreamEventResult{
			Complete: true,
			Result: assistantRoundtripResult{
				Outcome:          assistantRoundtripOutcomePendingExternal,
				PendingRequestID: runResult.PendingRequestID,
			},
		}, nil
	}
	h.setCurrentResult(assistantRoundtripOutcomeToolResult)
	return h.complete(assistantRoundtripOutcomeToolResult), nil
}

func (h stepStreamEventHandler) setCurrentResult(outcome assistantRoundtripOutcome) {
	if h.result != nil {
		*h.result = h.currentResult(outcome)
	}
}

func (h stepStreamEventHandler) complete(outcome assistantRoundtripOutcome) stepStreamEventResult {
	return stepStreamEventResult{Complete: true, Result: h.currentResult(outcome)}
}

func (h stepStreamEventHandler) currentResult(outcome assistantRoundtripOutcome) assistantRoundtripResult {
	progress := h.progress
	if progress == nil {
		defaultProgress := newStepToolProgress()
		progress = &defaultProgress
	}
	toolBatchSize := 0
	if h.batch != nil {
		toolBatchSize = h.batch.Len()
	}
	pendingRequestID := ""
	if h.result != nil {
		pendingRequestID = h.result.PendingRequestID
	}
	return progress.Result(outcome, toolBatchSize, pendingRequestID)
}
