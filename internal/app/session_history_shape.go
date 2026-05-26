package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func shapeSessionConversationState(history *sessionHistoryState, request provider.Request, currentTurnInputs []provider.Input, budget sessionHistoryBudget) {
	if history == nil {
		return
	}

	history.PendingCompaction = nil
	effectiveCompaction := cloneCompactionPayload(history.ExistingContinuation)
	rawOrder := sessionHistoryRawOrder(history.CompletedOrder, effectiveCompaction)
	inputShape := buildSessionConversationInputsWithBudget(rawOrder, history.Turns, effectiveCompaction, budget)
	inputs, rawInputBytes := inputShape.Inputs, inputShape.RawInputBytes
	if sessionHistoryRequestUsable(request, currentTurnInputs) {
		history.EstimatedTokens = estimateSessionRequestTokens(request, inputs, currentTurnInputs)
		history.RequestTokenSource = provider.TokenCountSourceEstimated
		history.CompactionSource = ""
		history.CompactedTokens = history.EstimatedTokens
		if budget.resolved() {
			if next := buildNextCompaction(history.ExistingContinuation, history.CompletedOrder, history.Turns, request, currentTurnInputs, budget); next != nil {
				history.PendingCompaction = cloneSessionCompactionPlan(
					sessionCompactionPlanFromPayload(history.ExistingContinuation, next, history.CompletedOrder),
				)
				history.CompactedTokens = estimateSessionRequestTokens(request, nextConversationInputs(history, next, budget), currentTurnInputs)
				history.CompactionSource = provider.TokenCountSourceEstimated
			}
		}
		if history.CompactionSource == "" {
			preserveSessionCompactionMeasurement(history, effectiveCompaction, history.EstimatedTokens, history.RequestTokenSource)
		}
	}
	applySessionConversationShape(history, rawOrder, inputs, rawInputBytes, effectiveCompaction, budget.SummaryBudgetBytes)
}

func nextConversationInputs(history *sessionHistoryState, payload *events.SessionHistoryContinuationUpdatedPayload, budget sessionHistoryBudget) []provider.Input {
	if history == nil || payload == nil {
		return nil
	}
	rawOrder := sessionHistoryRawOrder(history.CompletedOrder, payload)
	return buildSessionConversationInputsWithBudget(rawOrder, history.Turns, payload, budget).Inputs
}

func applySessionConversationShape(
	history *sessionHistoryState,
	rawOrder []string,
	inputs []provider.Input,
	rawInputBytes int,
	compaction *events.SessionHistoryContinuationUpdatedPayload,
	summaryBudgetBytes int,
) {
	if history == nil {
		return
	}
	priorInputBytes := replayedTurnsInputBytes(history.CompletedOrder, history.Turns)
	compactedPriorTurns := 0
	compactedInputBytes := 0
	if compaction != nil {
		compactedPriorTurns = compaction.ConsolidatedTurnCount
		compactedInputBytes = len(renderSessionCompactionConversationInput(compaction, summaryBudgetBytes))
	}

	conversation := sessionConversation{
		Inputs:             append([]provider.Input(nil), inputs...),
		Continuation:       cloneCompactionPayload(compaction),
		RequestTokenSource: history.RequestTokenSource,
		Pruning: events.ContextPrunedPayload{
			PriorTurns:          len(history.CompletedOrder),
			PriorInputBytes:     priorInputBytes,
			RawPriorTurns:       len(rawOrder),
			CompactedPriorTurns: compactedPriorTurns,
			CompactedInputBytes: compactedInputBytes,
			RawInputBytes:       rawInputBytes,
		},
	}
	conversation.Pruning.OmittedPriorTurns = conversation.Pruning.PriorTurns - conversation.Pruning.RawPriorTurns - conversation.Pruning.CompactedPriorTurns
	if conversation.Pruning.OmittedPriorTurns < 0 {
		conversation.Pruning.OmittedPriorTurns = 0
	}
	conversation.Pruning.OmittedInputBytes = conversation.Pruning.PriorInputBytes - conversation.Pruning.RawInputBytes
	if conversation.Pruning.OmittedInputBytes < 0 {
		conversation.Pruning.OmittedInputBytes = 0
	}
	history.Conversation = conversation
}

func buildSessionReplayInputs(order []string, turns map[string]*replayedSessionTurn) ([]provider.Input, int) {
	inputs := make([]provider.Input, 0, 16)
	totalBytes := 0
	for _, entry := range buildSessionReplayTimeline(order, turns) {
		if entry.TurnID == "" {
			input := provider.Input{
				Kind:    provider.InputKindAssistantMessage,
				Content: entry.Note.Content,
			}
			inputs = append(inputs, input)
			totalBytes += replayedInputBytes(input)
			continue
		}
		turn := turns[entry.TurnID]
		if turn == nil {
			continue
		}
		for _, input := range turn.replayInputs() {
			inputs = append(inputs, input)
			totalBytes += replayedInputBytes(input)
		}
	}
	return inputs, totalBytes
}

func buildSessionConversationInputs(order []string, turns map[string]*replayedSessionTurn, compaction *events.SessionHistoryContinuationUpdatedPayload, summaryBudgetBytes int) ([]provider.Input, int) {
	inputs, rawBytes := buildSessionReplayInputs(order, turns)
	conversation := sessionConversation{
		Inputs:       append([]provider.Input(nil), inputs...),
		Continuation: cloneCompactionPayload(compaction),
	}
	applySessionConversationCompactionInput(&conversation, summaryBudgetBytes)
	return conversation.Inputs, rawBytes
}

func completedTurnOrder(order []string, turns map[string]*replayedSessionTurn, compaction *events.SessionHistoryContinuationUpdatedPayload) []string {
	out := make([]string, 0, len(order))
	prefixCount := sessionCompactionPrefixCount(order, compaction)
	for _, turnID := range order {
		if len(out) < prefixCount {
			out = append(out, turnID)
			continue
		}
		turn := turns[turnID]
		if turn == nil || !turn.Terminal || !turn.hasReplayContent() {
			continue
		}
		out = append(out, turnID)
	}
	return out
}

func ensureCompactedSessionTurnPlaceholders(order []string, turns map[string]*replayedSessionTurn, compaction *events.SessionHistoryContinuationUpdatedPayload) {
	prefixCount := sessionCompactionPrefixCount(order, compaction)
	for index := 0; index < prefixCount; index++ {
		turnID := strings.TrimSpace(order[index])
		if turnID == "" {
			continue
		}
		turn, ok := turns[turnID]
		if !ok {
			turn = &replayedSessionTurn{TurnID: turnID}
			turns[turnID] = turn
		}
		turn.Terminal = true
	}
}

func turnIDInCompactedPrefix(order []string, compaction *events.SessionHistoryContinuationUpdatedPayload, turnID string) bool {
	if compaction == nil || strings.TrimSpace(turnID) == "" {
		return false
	}
	prefixCount := sessionCompactionPrefixCount(order, compaction)
	if prefixCount <= 0 {
		return false
	}
	for index := 0; index < prefixCount && index < len(order); index++ {
		if order[index] == turnID {
			return true
		}
	}
	return false
}
