package app

import (
	"context"
	"errors"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func appendCompactionRuntimeNotes(compaction *events.SessionHistoryContinuationUpdatedPayload, completedOrder []string, turns map[string]*replayedSessionTurn, budgetBytes int) *events.SessionHistoryContinuationUpdatedPayload {
	if compaction == nil {
		return nil
	}
	copyPayload := cloneCompactionPayload(compaction)
	for _, turnID := range compactedPrefixTurnOrder(completedOrder, copyPayload) {
		turn := turns[turnID]
		if turn == nil {
			continue
		}
		for _, note := range turn.postTerminalRuntimeNotes() {
			copyPayload = appendRuntimeNoteToCompaction(copyPayload, turnID, note.Content, budgetBytes)
		}
	}
	return copyPayload
}

func appendRuntimeNoteToCompaction(compaction *events.SessionHistoryContinuationUpdatedPayload, turnID string, note string, budgetBytes int) *events.SessionHistoryContinuationUpdatedPayload {
	return appendRuntimeNoteToCompactionArtifact(compaction, turnID, note, budgetBytes)
}

func applySessionConversationCompactionInput(conversation *sessionConversation, budgetBytes int) {
	if conversation == nil {
		return
	}
	content := renderSessionCompactionConversationInput(conversation.Continuation, budgetBytes)
	hasCompactionInput := conversation.Pruning.CompactedInputBytes > 0 &&
		len(conversation.Inputs) > 0 &&
		conversation.Inputs[0].Kind == provider.InputKindAssistantMessage
	if content == "" {
		if hasCompactionInput {
			conversation.Inputs = append([]provider.Input(nil), conversation.Inputs[1:]...)
		}
		conversation.Pruning.CompactedInputBytes = 0
		return
	}
	compactionInput := provider.Input{
		Kind:    provider.InputKindAssistantMessage,
		Content: content,
	}
	if hasCompactionInput {
		conversation.Inputs[0] = compactionInput
	} else {
		conversation.Inputs = append([]provider.Input{compactionInput}, conversation.Inputs...)
	}
	conversation.Pruning.CompactedInputBytes = len(content)
}

func cloneCompactionPayload(payload *events.SessionHistoryContinuationUpdatedPayload) *events.SessionHistoryContinuationUpdatedPayload {
	if payload == nil {
		return nil
	}
	copyPayload := *payload
	if payload.InputBudget != nil {
		budget := *payload.InputBudget
		copyPayload.InputBudget = &budget
	}
	copyPayload.Artifact = cloneHistoryCompactionArtifact(payload.Artifact)
	return &copyPayload
}

func (r *TurnRunner) realizeSessionHistoryCompaction(
	ctx context.Context,
	input sessionConversationRequest,
	history *sessionHistoryState,
	plan *sessionCompactionPlan,
) error {
	if r == nil || history == nil || plan == nil {
		return nil
	}

	budget := resolveSessionHistoryBudget(input.ModelRoute, r.models, r.sessionConfig)
	rawTokens := max(history.EstimatedTokens, 0)
	if err := r.appendContextCompactionStarted(ctx, input.SessionID, input.TurnID, events.ContextCompactionStartedPayload{
		Scope:                  events.CompactionScopeHistory,
		InputLimitTokens:       budget.InputLimitTokens,
		TriggerTokens:          budget.TriggerTokens,
		TargetTokens:           budget.TargetTokens,
		EstimatedRequestTokens: rawTokens,
	}); err != nil {
		return err
	}

	artifact, err := r.generateSessionCompactionArtifact(ctx, input, history, plan)
	if err != nil {
		if appendErr := r.appendContextCompactionFailed(ctx, input.SessionID, input.TurnID, events.ContextCompactionFailedPayload{
			Scope:                  events.CompactionScopeHistory,
			Reason:                 "artifact_generation_failed",
			Detail:                 err.Error(),
			InputLimitTokens:       budget.InputLimitTokens,
			TriggerTokens:          budget.TriggerTokens,
			TargetTokens:           budget.TargetTokens,
			EstimatedRequestTokens: rawTokens,
		}); appendErr != nil {
			return errors.Join(err, appendErr)
		}
		return err
	}

	payload := buildSessionCompactionPayloadFromArtifact(plan, artifact, budget.SummaryBudgetBytes)
	if payload == nil {
		return errors.New("empty session compaction payload")
	}
	rawOrder := sessionHistoryRawOrder(history.CompletedOrder, payload)
	inputShape := buildSessionConversationInputsWithBudget(rawOrder, history.Turns, payload, budget)
	inputs, rawInputBytes := inputShape.Inputs, inputShape.RawInputBytes
	history.CompactionSource = provider.TokenCountSourceEstimated
	compactedTokens := estimateSessionRequestTokens(input.providerRequest(), inputs, input.CurrentInputs)
	payload = refreshSessionCompactionPayloadMetadata(
		payload,
		nil,
		input.ModelRoute,
		budget,
		rawTokens,
		compactedTokens,
		history.CompactionSource,
		"artifact_renderer",
	)
	history.CompactedTokens = continuationCompactedRequestTokens(payload)
	applySessionConversationShape(history, rawOrder, inputs, rawInputBytes, payload, budget.SummaryBudgetBytes)

	return nil
}
