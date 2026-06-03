package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/workspace"
)

func (r *TurnRunner) runProviderRequest(ctx context.Context, request provider.Request, attribution turnProviderRequestAttribution, sessionID, turnID string, providerRequestIndex int, inputLimitTokens int, temporaryGrants []workspace.Grant, temporaryNetworkTargets []string, workflowBudget workflowTurnBudget, state *turnLoopState) (assistantRoundtripResult, error) {
	request.MaxOutputTokens = provider.EffectiveMaxOutputTokens(request)
	initialState := cloneTurnLoopState(*state)
	requestTokenSource := provider.TokenCountSourceEstimated
	outputContinuations := 0

	for attempt := 1; ; attempt++ {
		*state = cloneTurnLoopState(initialState)

		r.logProviderRequestStarted(request, providerRequestIndex, attempt)
		attemptResult := r.runProviderRequestAttempt(ctx, request, sessionID, turnID, providerRequestIndex, temporaryGrants, temporaryNetworkTargets, workflowBudget, state)
		attributedModel := providerUsageModelForTrace(request.Model, attemptResult.RouteTrace)
		attributedRequest := request
		attributedRequest.Model = attributedModel
		attributedRequest = provider.PreparePromptRequest(attributedRequest)
		requestBreakdown := provider.EstimateRequestTokenBreakdown(attributedRequest)
		requestTokens := requestBreakdown.TotalTokens
		r.logProviderRequestCompleted(request, providerRequestIndex, attempt, attemptResult, requestTokens)
		usage := estimateTurnProviderUsage(r.models, attributedModel, requestTokens, attemptResult.CompletionTokens)
		inputSavingsCost := estimateInputSavingsCost(r.models, attributedModel, attribution.InputTokensSaved())
		retryable := provider.RetryHintForError(attemptResult.Error).Retryable
		retry := providerRetryDecision{}
		var expandedAnthropicRequest provider.Request
		expandedAnthropicMaxTokens := false
		if attemptResult.Error != nil {
			if errors.Is(attemptResult.Error, provider.ErrAnthropicMaxTokensExceeded) && attemptResult.Result.ExecutedTools == 0 && attemptResult.Result.ReusedTools == 0 {
				expandedAnthropicRequest, expandedAnthropicMaxTokens = r.expandAnthropicMaxTokensRequest(request, attemptResult.RouteTrace)
			}
			if !expandedAnthropicMaxTokens {
				retry = r.providerRetryDecision(providerRetryInput{
					Err:              attemptResult.Error,
					Attempt:          attempt,
					RequestStarted:   attemptResult.RequestStarted,
					DurableProgress:  attemptResult.DurableProgress,
					CompletionTokens: attemptResult.CompletionTokens,
					ToolCallStarted:  attemptResult.ToolCallStarted,
					ExecutedTools:    attemptResult.Result.ExecutedTools,
					ReusedTools:      attemptResult.Result.ReusedTools,
				})
			}
		}
		if err := r.appendTurnProviderUsageRecorded(ctx, sessionID, turnID, events.TurnProviderUsageRecordedPayload{
			Model:                                 attributedModel.String(),
			Kind:                                  string(events.TurnProviderUsageKindAgent),
			RequestedModel:                        request.Model.String(),
			Step:                                  providerRequestIndex,
			Attempt:                               attempt,
			DurationMillis:                        int(attemptResult.Duration / time.Millisecond),
			RequestStarted:                        attemptResult.RequestStarted,
			RequestAPIMode:                        attemptResult.RequestTrace.APIMode,
			RequestParallelToolCalls:              attemptResult.RequestTrace.ParallelToolCalls,
			RouteAttempts:                         providerRouteAttemptPayloads(attemptResult.RouteTrace),
			EstimatedRequestTokens:                usage.RequestTokens,
			EstimatedPromptTokens:                 requestBreakdown.PromptTokens,
			EstimatedConversationTokens:           requestBreakdown.ConversationTokens,
			EstimatedToolNameTokens:               requestBreakdown.ToolNameTokens,
			EstimatedToolDescriptionTokens:        requestBreakdown.ToolDescriptionTokens,
			EstimatedToolSchemaTokens:             requestBreakdown.ToolSchemaTokens,
			EstimatedPromptCompactionTokensSaved:  attribution.PromptCompactionTokensSaved,
			EstimatedHistoryCompactionTokensSaved: attribution.HistoryCompactionTokensSaved,
			EstimatedCurrentTurnProjectionTokensSaved:  attribution.CurrentTurnProjectionTokensSaved,
			EstimatedToolDescriptionTokensSaved:        attribution.ToolDescriptionTokensSaved,
			EstimatedToolSchemaTokensSaved:             attribution.ToolSchemaTokensSaved,
			EstimatedDeterministicContextTokens:        attribution.DeterministicContextTokens,
			EstimatedDeterministicContextOmittedTokens: attribution.DeterministicContextOmittedTokens,
			EstimatedInputSavingsCost:                  inputSavingsCost,
			ToolCount:                                  requestBreakdown.ToolCount,
			RequestTokenSource:                         string(requestTokenSource),
			InputLimitTokens:                           max(inputLimitTokens, 0),
			EstimatedCompletionTokens:                  usage.CompletionTokens,
			EstimatedInputCost:                         usage.InputCost,
			EstimatedOutputCost:                        usage.OutputCost,
			Error:                                      errorString(attemptResult.Error),
			Retryable:                                  retryable,
			RetrySkippedReason:                         retry.SkippedReason,
			FinishReason:                               string(attemptResult.FinishReason),
			DurableProgress:                            attemptResult.DurableProgress,
			ExecutedTools:                              attemptResult.Result.ExecutedTools,
			ReusedTools:                                attemptResult.Result.ReusedTools,
		}); err != nil {
			return assistantRoundtripResult{}, err
		}
		if report := attemptResult.UsageReport; report != nil {
			reportedModel := providerReportedModelRef(attributedModel, report.Model)
			reportedUsage := estimateReportedTurnProviderUsage(r.models, reportedModel, *report, usage)
			cacheSavingsCost := estimateReportedCacheSavingsCost(r.models, reportedModel, *report)
			if err := r.appendTurnProviderUsageReported(ctx, sessionID, turnID, events.TurnProviderUsageReportedPayload{
				Model:                     providerReportedModelLabel(attributedModel, report.Model),
				Kind:                      string(events.TurnProviderUsageKindAgent),
				RequestID:                 strings.TrimSpace(report.RequestID),
				Step:                      providerRequestIndex,
				Attempt:                   attempt,
				InputTokens:               max(report.InputTokens, 0),
				CacheReadInputTokens:      max(report.CacheReadInputTokens, 0),
				CacheWriteInputTokens:     max(report.CacheWriteInputTokens, 0),
				OutputTokens:              max(report.OutputTokens, 0),
				ReasoningTokens:           max(report.ReasoningTokens, 0),
				TotalTokens:               max(report.TotalTokens, 0),
				EstimatedInputCost:        reportedUsage.InputCost,
				EstimatedOutputCost:       reportedUsage.OutputCost,
				EstimatedCacheSavingsCost: cacheSavingsCost,
				CachePricingApplied:       reportedUsage.CachePricingApplied,
				CachePricingMissing:       reportedUsage.CachePricingMissing,
			}); err != nil {
				return assistantRoundtripResult{}, err
			}
		}
		if attemptResult.Error == nil {
			if shouldAutoContinueOutput(attemptResult, outputContinuations, r.maxOutputContinuationAttempts()) {
				continuedRequest, ok := outputContinuationRequest(request, initialState, attemptResult.CommittedState)
				if ok {
					outputContinuations++
					request = continuedRequest
					*state = cloneTurnLoopState(attemptResult.CommittedState)
					initialState = cloneTurnLoopState(attemptResult.CommittedState)
					continue
				}
			}
			return attemptResult.Result, nil
		}

		*state = cloneTurnLoopState(attemptResult.CommittedState)
		if attemptResult.DurableProgress && attemptResult.Result.Outcome == "" && (attemptResult.Result.ExecutedTools > 0 || attemptResult.Result.ReusedTools > 0) {
			attemptResult.Result.Outcome = assistantRoundtripOutcomeToolResult
		}
		if errors.Is(attemptResult.Error, provider.ErrAnthropicMaxTokensExceeded) {
			if attemptResult.Result.ExecutedTools > 0 || attemptResult.Result.ReusedTools > 0 {
				return attemptResult.Result, nil
			}
			if expandedAnthropicMaxTokens {
				if err := r.appendAssistantPreviewReset(sessionID, turnID); err != nil {
					return assistantRoundtripResult{}, err
				}
				request = expandedAnthropicRequest
				initialState = cloneTurnLoopState(attemptResult.CommittedState)
				continue
			}
		}
		if provider.IsInputLimitExceeded(attemptResult.Error) {
			return assistantRoundtripResult{}, attemptResult.Error
		}

		hint := provider.RetryHintForError(attemptResult.Error)
		if hint.Retryable && attemptResult.DurableProgress && attemptResult.Result.ExecutedTools > 0 {
			if err := r.appendTurnRetryScheduled(ctx, sessionID, turnID, attemptResult.Error, attempt, max(r.maxProviderRetryAttempts(), attempt), r.providerRetryAt(0)); err != nil {
				return assistantRoundtripResult{}, err
			}
			return attemptResult.Result, nil
		}

		if retry.Retryable {
			retryAt := r.providerRetryAt(retry.Delay)
			if err := r.appendTurnRetryScheduled(ctx, sessionID, turnID, attemptResult.Error, attempt, r.maxProviderRetryAttempts(), retryAt); err != nil {
				return assistantRoundtripResult{}, err
			}
			if err := r.waitForProviderRetry(ctx, retry.Delay); err != nil {
				return assistantRoundtripResult{}, err
			}
			continue
		}

		args := []any{
			"session_id", sessionID,
			"turn_id", turnID,
			"model", request.Model.String(),
		}
		args = append(args, providerErrorLogFields(attemptResult.Error)...)
		r.logger.Error("provider request failed", attemptResult.Error, args...)
		if _, failErr := r.failTurn(ctx, sessionID, turnID, provider.ModelRoute{Primary: request.Model}, attemptResult.Error); failErr != nil {
			return assistantRoundtripResult{}, errors.Join(attemptResult.Error, failErr)
		}
		return assistantRoundtripResult{Outcome: assistantRoundtripOutcomeFailed}, nil
	}
}

func (r *TurnRunner) expandAnthropicMaxTokensRequest(request provider.Request, trace provider.RouteTrace) (provider.Request, bool) {
	selected := request.Model
	if model, ok := trace.SelectedModel(); ok {
		selected = model
	}
	if provider.CanonicalProviderID(selected.ProviderID) != "anthropic" {
		return provider.Request{}, false
	}

	selectedRequest := request
	selectedRequest.Model = selected
	current := provider.EffectiveMaxOutputTokens(selectedRequest)
	if current <= 0 {
		return provider.Request{}, false
	}

	next := modelMaxOutputTokenCeilingForModel(r.models, selected)
	if next <= current {
		return provider.Request{}, false
	}

	selectedRequest.MaxOutputTokens = next
	return selectedRequest, true
}

const outputContinuationInstruction = "Continue the previous assistant response exactly where it stopped. Do not repeat earlier text, do not add a preface, and finish the answer."
const outputContinuationNoVisibleOutputInstruction = "The previous assistant response reached the output limit before producing visible output. Continue now by using tools if work remains, or provide the final answer."

func shouldAutoContinueOutput(result providerRequestAttemptResult, continuations, limit int) bool {
	if limit <= 0 || continuations >= limit {
		return false
	}
	if provider.NormalizeFinishReason(result.FinishReason) != provider.FinishReasonLength {
		return false
	}
	if result.Error != nil || result.Result.Outcome != assistantRoundtripOutcomeAssistantDone {
		return false
	}
	if result.Result.ToolBatchSize != 0 || result.Result.ExecutedTools != 0 || result.Result.ReusedTools != 0 {
		return false
	}
	return true
}

func outputContinuationRequest(request provider.Request, before, after turnLoopState) (provider.Request, bool) {
	segment := assistantTextDelta(before.AssistantText, after.AssistantText)
	continued := request
	inputs := cloneProviderInputs(request.Inputs)
	if len(inputs) > 0 && inputs[len(inputs)-1].Kind == provider.InputKindUserMessage && isOutputContinuationInstruction(inputs[len(inputs)-1].Content) {
		inputs = inputs[:len(inputs)-1]
	}
	instruction := outputContinuationInstruction
	if strings.TrimSpace(segment) != "" {
		if len(inputs) > 0 && inputs[len(inputs)-1].Kind == provider.InputKindAssistantMessage {
			inputs[len(inputs)-1].Content += segment
		} else {
			inputs = append(inputs, provider.Input{
				Kind:    provider.InputKindAssistantMessage,
				Content: segment,
			})
		}
	} else {
		instruction = outputContinuationNoVisibleOutputInstruction
	}
	continued.Inputs = append(inputs, provider.Input{
		Kind:    provider.InputKindUserMessage,
		Content: instruction,
	})
	return continued, true
}

func isOutputContinuationRequest(request provider.Request) bool {
	if len(request.Inputs) == 0 {
		return false
	}
	last := request.Inputs[len(request.Inputs)-1]
	return last.Kind == provider.InputKindUserMessage && isOutputContinuationInstruction(last.Content)
}

func isOutputContinuationInstruction(content string) bool {
	switch strings.TrimSpace(content) {
	case outputContinuationInstruction, outputContinuationNoVisibleOutputInstruction:
		return true
	default:
		return false
	}
}

func assistantTextDelta(before, after string) string {
	if before == "" {
		return after
	}
	if strings.HasPrefix(after, before) {
		return after[len(before):]
	}
	return after
}

func (r *TurnRunner) runProviderRequestAttempt(ctx context.Context, request provider.Request, sessionID, turnID string, providerRequestIndex int, temporaryGrants []workspace.Grant, temporaryNetworkTargets []string, workflowBudget workflowTurnBudget, state *turnLoopState) providerRequestAttemptResult {
	startedAt := time.Now()
	request.RawSSEObserver = r.providerRawSSEObserver(request, providerRequestIndex)
	stream, err := r.provider.Stream(ctx, request)
	trace := providerAttemptRouteTrace(request, stream, err)
	requestTrace, _ := provider.StreamRequestTrace(stream)
	requestStarted := providerAttemptRequestStarted(request, stream, err)

	assistantSegment := ""
	completionTokens := 0
	finishReason := provider.FinishReasonUnknown
	result := assistantRoundtripResult{}
	progress := newStepToolProgress()
	stepBatch := stepToolBatch{StepIndex: providerRequestIndex, Calls: make([]stepToolCall, 0, 4)}
	stepHasBatchableResults := false
	toolCalls := newStepToolCallCollector(provider.RequiresOpenAIReasoningContentReplay(request))
	stepConversationStart := -1
	committedToolBatchSize := 0
	durableProgress := false
	stepHasToolCalls := false
	committedState := cloneTurnLoopState(*state)
	streamClosed := false
	stepExecutor := newStepToolExecutor(r.tools, requestToolNames(request.Tools), request.Inputs, temporaryGrants, temporaryNetworkTargets)
	stepExecutor.workflowBudget = workflowBudget
	capabilityResolver := newStepToolCapabilityResolver(r.tools)
	completer := newStepAttemptCompleter(stepAttemptCompleterInput{
		Stream:           stream,
		StreamClosed:     &streamClosed,
		RequestStarted:   requestStarted,
		DurableProgress:  &durableProgress,
		CommittedState:   &committedState,
		CompletionTokens: &completionTokens,
		ToolCallStarted:  &stepHasToolCalls,
		RouteTrace:       trace,
		StartedAt:        startedAt,
		FinishReason:     &finishReason,
		RequestTrace:     requestTrace,
	})
	complete := completer.Complete
	if err != nil {
		return complete(assistantRoundtripResult{}, err)
	}
	commitStepState := func() {
		committedState = cloneTurnLoopState(*state)
	}
	markDurableProgress := func() {
		durableProgress = true
	}
	toolBoundary := newStepToolBoundary(stepToolBoundaryInput{
		Runner:                 r,
		Context:                ctx,
		SessionID:              sessionID,
		TurnID:                 turnID,
		State:                  state,
		Batch:                  &stepBatch,
		CommittedToolBatchSize: &committedToolBatchSize,
		CommitStepState:        commitStepState,
	})

	assistantPreview := newStepAssistantPreview(stepAssistantPreviewInput{
		Runner:              r,
		Context:             ctx,
		SessionID:           sessionID,
		TurnID:              turnID,
		State:               state,
		Segment:             &assistantSegment,
		HasToolCalls:        &stepHasToolCalls,
		TrimOverlap:         isOutputContinuationRequest(request),
		MarkDurableProgress: markDurableProgress,
		CommitStepState:     commitStepState,
	})
	finalizeToolStepBatch := func() error {
		return toolBoundary.Commit()
	}
	eventHandler := newStepStreamEventHandler(stepStreamEventHandlerInput{
		Runner:                  r,
		Context:                 ctx,
		SessionID:               sessionID,
		TurnID:                  turnID,
		Model:                   request.Model,
		State:                   state,
		Collector:               toolCalls,
		Batch:                   &stepBatch,
		Preview:                 assistantPreview,
		StepConversationStart:   &stepConversationStart,
		Executor:                stepExecutor,
		Progress:                &progress,
		CapabilityResolver:      capabilityResolver,
		Result:                  &result,
		StepHasBatchableResults: &stepHasBatchableResults,
		StepHasToolCalls:        &stepHasToolCalls,
		CompletionTokens:        &completionTokens,
		DurableProgress:         &durableProgress,
		CommitStepState:         commitStepState,
	})

	for {
		event, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if eventHandler.hasPendingToolBatch() {
				eventResult, batchErr := eventHandler.executeInterruptedStreamSafeToolBatchAndComplete()
				if batchErr != nil {
					return complete(result, batchErr)
				}
				result = eventResult.Result
			}
			return complete(result, err)
		}
		if err := event.Validate(); err != nil {
			return complete(result, err)
		}

		eventResult, err := eventHandler.Handle(event)
		if err != nil {
			return complete(result, err)
		}
		if eventResult.Complete {
			return complete(eventResult.Result, nil)
		}
	}

	if eventHandler.hasPendingToolBatch() {
		eventResult, err := eventHandler.executePendingToolBatchAndComplete()
		if err != nil {
			return complete(result, err)
		}
		return complete(eventResult.Result, nil)
	}

	eofResult, err := completeProviderRequestAfterStreamEOF(providerRequestEOFInput{
		Progress:               &progress,
		ToolBatchSize:          stepBatch.Len(),
		PendingRequestID:       result.PendingRequestID,
		Preview:                assistantPreview,
		CommitToolStepBoundary: finalizeToolStepBatch,
	})
	if err != nil {
		return complete(result, err)
	}
	return complete(eofResult, nil)
}
