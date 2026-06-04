package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func buildSessionConversationState(replayed []events.Event, currentTurnID string, checkpoint *sessionHistoryCheckpoint) (sessionHistoryState, error) {
	return buildSessionConversationStateWithBudget(replayed, currentTurnID, checkpoint, defaultSessionHistoryBudget())
}

func buildSessionConversationStateWithBudget(replayed []events.Event, currentTurnID string, checkpoint *sessionHistoryCheckpoint, budget sessionHistoryBudget) (sessionHistoryState, error) {
	return buildSessionConversationStateWithBudgetAndResolver(
		replayed,
		currentTurnID,
		checkpoint,
		budget,
		provider.Request{},
		nil,
		defaultSessionHistoryMutationPathResolver(),
	)
}

func buildSessionConversationStateWithBudgetAndResolver(
	replayed []events.Event,
	currentTurnID string,
	checkpoint *sessionHistoryCheckpoint,
	budget sessionHistoryBudget,
	request provider.Request,
	currentTurnInputs []provider.Input,
	mutationPaths sessionHistoryMutationPathResolver,
) (sessionHistoryState, error) {
	return buildSessionConversationStateWithBudgetAndResolverAndBlobs(
		context.Background(),
		nil,
		replayed,
		currentTurnID,
		checkpoint,
		budget,
		request,
		currentTurnInputs,
		mutationPaths,
	)
}

func buildSessionConversationStateWithBudgetAndResolverAndBlobs(
	ctx context.Context,
	blobs ToolResultBlobStore,
	replayed []events.Event,
	currentTurnID string,
	checkpoint *sessionHistoryCheckpoint,
	budget sessionHistoryBudget,
	request provider.Request,
	currentTurnInputs []provider.Input,
	mutationPaths sessionHistoryMutationPathResolver,
) (sessionHistoryState, error) {
	if mutationPaths == nil {
		mutationPaths = defaultSessionHistoryMutationPathResolver()
	}
	order := make([]string, 0, 8)
	turns := make(map[string]*replayedSessionTurn)
	var existingCompaction *events.SessionHistoryContinuationUpdatedPayload
	throughSequence := int64(-1)
	if checkpoint != nil {
		order = append(order, checkpoint.CompletedOrder...)
		for turnID, turn := range checkpoint.Turns {
			turns[turnID] = cloneReplayedSessionTurn(turn)
		}
		existingCompaction = cloneCompactionPayload(checkpoint.Continuation)
		throughSequence = checkpoint.ThroughSequence
		ensureCompactedSessionTurnPlaceholders(order, turns, existingCompaction)
	}

	for _, event := range replayed {
		if !event.Ephemeral && event.Sequence > throughSequence {
			throughSequence = event.Sequence
		}
		if payload, ok := event.Payload.(events.SessionHistoryContinuationUpdatedPayload); ok {
			existingCompaction = cloneCompactionPayload(&payload)
			continue
		}
		if _, ok := event.Payload.(events.ContextCompactionStartedPayload); ok {
			continue
		}
		if event.TurnID == sessionTurnID || event.TurnID == currentTurnID {
			continue
		}

		turn := ensureSessionTurn(turns, &order, event.TurnID)
		switch payload := event.Payload.(type) {
		case events.UserMessagePayload:
			turn.UserText = payload.Content
			turn.UserAttachments = attachmentsFromUserMessagePayload(payload.Attachments)
			turn.Inputs = append(turn.Inputs, provider.Input{
				Kind:        provider.InputKindUserMessage,
				Content:     payload.Content,
				Attachments: cloneProviderAttachments(turn.UserAttachments),
			})
		case events.AssistantCommitPayload:
			if !strings.HasPrefix(payload.Content, turn.committedAssistant) {
				return sessionHistoryState{}, ErrAssistantCommitOutOfOrder
			}
			segment := strings.TrimPrefix(payload.Content, turn.committedAssistant)
			if segment != "" {
				turn.Inputs = append(turn.Inputs, provider.Input{
					Kind:    provider.InputKindAssistantMessage,
					Content: segment,
				})
			}
			turn.committedAssistant = payload.Content
			turn.AssistantText = payload.Content
		case events.ReasoningDeltaPayload:
			appendReplayedTurnReasoning(turn, payload.Content)
		case events.AnthropicThinkingCommittedPayload:
			turn.Inputs = append(turn.Inputs, provider.Input{
				Kind: provider.InputKindAnthropicThinking,
				AnthropicThinking: &provider.AnthropicThinkingBlock{
					Type:      payload.Type,
					Thinking:  payload.Thinking,
					Signature: payload.Signature,
					Data:      payload.Data,
				},
			})
		case events.OpenAIReasoningCommittedPayload:
			turn.Inputs = append(turn.Inputs, providerOpenAIReasoningInput(payload.Item))
		case events.TurnContinuationStartedPayload:
			turn.RuntimeNotes = append(turn.RuntimeNotes, replayedSessionRuntimeNote{
				Sequence: event.Sequence,
				Content:  continuedTurnRuntimeNote(payload),
			})
			if summaryInput := renderTurnWorkSummaryInput(turnWorkSummary{
				Objective:     payload.Summary.Objective,
				Decisions:     append([]string(nil), payload.Summary.Decisions...),
				TouchedPaths:  append([]string(nil), payload.Summary.TouchedPaths...),
				CompletedWork: append([]string(nil), payload.Summary.CompletedWork...),
				Verification:  append([]string(nil), payload.Summary.Verification...),
				Failures:      append([]string(nil), payload.Summary.Failures...),
				OpenItems:     append([]string(nil), payload.Summary.OpenItems...),
			}); summaryInput != nil {
				turn.Inputs = append(turn.Inputs, *summaryInput)
			}
		case events.ToolCallDeclaredPayload:
			turn.ToolCallCount++
			turn.ToolNames = appendUniqueToolName(turn.ToolNames, payload.ToolName)
			turn.WorkspacePaths = appendUniqueValues(turn.WorkspacePaths, mutationPaths(payload.ToolName, payload.Input))
			turn.Inputs = append(turn.Inputs, providerToolCallInputWithContext(
				payload.CallID,
				payload.ToolName,
				provider.ToolKind(payload.ToolKind),
				payload.Input,
				payload.GoogleThoughtSignature,
				payload.OpenAIReasoningContent,
			))
		case events.ToolCallBatchPayload:
			turn.Inputs = normalizeToolCallBatch(turn.Inputs, payload.CallIDs)
		case events.ExecutionDeclaredPayload:
			turn.recordExecution(replayedExecution{
				ExecutionID:      payload.ExecutionID,
				ToolName:         payload.ToolName,
				Intent:           payload.Intent,
				Effect:           payload.Effect,
				CommandPreview:   payload.CommandPreview,
				WorkingDirectory: payload.WorkingDirectory,
			}, payload.ToolCallID)
		case events.ToolExecEndPayload:
			markTurnToolOutcome(turn, payload.ToolName, !payload.Successful())
			result := replayedToolResult{
				CallID:              payload.CallID,
				ToolName:            payload.ToolName,
				ToolKind:            provider.ToolKind(payload.ToolKind),
				ReusedFromCallID:    payload.ReusedFromCallID,
				ReusedFromSessionID: payload.ReusedFromSessionID,
				ReusedFromTurnID:    payload.ReusedFromTurnID,
				RetryOfCallID:       payload.RetryOfCallID,
				Output:              payload.Output,
				Error:               payload.Error,
				StructuredResult:    cloneStructuredResult(payload.StructuredResult),
				OutputBlob:          cloneToolResultBlobRef(payload.OutputBlob),
				ErrorBlob:           cloneToolResultBlobRef(payload.ErrorBlob),
				Succeeded:           payload.Successful(),
			}
			turn.recordToolResult(result)
			turn.Inputs = append(turn.Inputs, replayedToolResultInput(ctx, blobs, result))
		case events.ExecutionBackgroundReadyPayload:
			appendBackgroundRuntimeNote(turn, order, &existingCompaction, budget.SummaryBudgetBytes, event.Sequence, executionBackgroundReadyNote(turn.execution(payload.ToolCallID), payload))
		case events.ExecutionBackgroundExitedPayload:
			appendBackgroundRuntimeNote(turn, order, &existingCompaction, budget.SummaryBudgetBytes, event.Sequence, executionBackgroundExitedNote(turn.execution(payload.ToolCallID), payload))
		case events.ExecutionBackgroundLostPayload:
			appendBackgroundRuntimeNote(turn, order, &existingCompaction, budget.SummaryBudgetBytes, event.Sequence, executionBackgroundLostNote(turn.execution(payload.ToolCallID), payload))
		case events.TurnDonePayload:
			turn.Terminal = true
			turn.TerminalSequence = event.Sequence
			turn.TerminalStatus = "completed"
		case events.TurnCanceledPayload:
			turn.Terminal = true
			turn.TerminalSequence = event.Sequence
			turn.TerminalStatus = "canceled"
			turn.TerminalError = ""
			turn.TerminalRetryable = false
		case events.TurnErrorPayload:
			turn.Terminal = true
			turn.TerminalSequence = event.Sequence
			turn.TerminalStatus = "failed"
			turn.TerminalError = payload.Message
			turn.TerminalRetryable = payload.Retryable
		}
	}

	for _, turn := range turns {
		if turn == nil {
			continue
		}
		turn.Inputs = normalizePendingToolConversation(turn.Inputs)
	}

	completedOrder := completedTurnOrder(order, turns, existingCompaction)
	existingCompaction = normalizeSessionCompactionPayload(existingCompaction, budget, completedOrder)
	existingCompaction = appendCompactionRuntimeNotes(existingCompaction, completedOrder, turns, budget.SummaryBudgetBytes)

	history := sessionHistoryState{
		CompletedOrder:       completedOrder,
		Turns:                turns,
		ThroughSequence:      throughSequence,
		ExistingContinuation: cloneCompactionPayload(existingCompaction),
	}
	shapeSessionConversationState(&history, request, currentTurnInputs, budget)
	return history, nil
}

func ensureSessionTurn(turns map[string]*replayedSessionTurn, order *[]string, turnID string) *replayedSessionTurn {
	if turn, ok := turns[turnID]; ok {
		return turn
	}

	turn := &replayedSessionTurn{TurnID: turnID}
	turns[turnID] = turn
	*order = append(*order, turnID)
	return turn
}

func appendUniqueToolName(names []string, name string) []string {
	if strings.TrimSpace(name) == "" {
		return names
	}
	return appendUniqueValues(names, []string{name})
}

func markTurnToolOutcome(turn *replayedSessionTurn, toolName string, failed bool) {
	if turn == nil {
		return
	}
	if failed {
		turn.FailedToolCalls++
		turn.FailedToolNames = appendUniqueToolName(turn.FailedToolNames, toolName)
		return
	}
	turn.SuccessfulToolCalls++
}

func continuedTurnRuntimeNote(payload events.TurnContinuationStartedPayload) string {
	switch strings.TrimSpace(payload.Reason) {
	case events.TurnContinuationReasonContextLimit:
		return "Continued automatically after the previous turn reached the model input limit."
	default:
		return "Continued automatically from the previous turn."
	}
}
