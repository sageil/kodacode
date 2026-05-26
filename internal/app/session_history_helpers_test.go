package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func buildSessionConversation(replayed []events.Event, currentTurnID string) (sessionConversation, error) {
	return buildSessionConversationWithBudget(
		replayed,
		currentTurnID,
		sessionHistoryBudgetFromInputLimit(2400, currentTurnInputLimitSourceDefault, SessionConfig{}),
	)
}

func buildSessionConversationWithBudget(replayed []events.Event, currentTurnID string, budget sessionHistoryBudget) (sessionConversation, error) {
	history, err := buildSessionConversationStateWithBudgetAndResolverAndBlobs(
		context.Background(),
		nil,
		replayed,
		currentTurnID,
		nil,
		budget,
		provider.Request{
			SessionID:    "session-history-test",
			TurnID:       currentTurnID,
			AgentID:      "session-history",
			Instructions: "Continue the coding task using the preserved session history.",
		},
		[]provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "continue",
		}},
		defaultSessionHistoryMutationPathResolver(),
	)
	if err != nil {
		return sessionConversation{}, err
	}
	if history.Conversation.Continuation == nil && history.PendingCompaction != nil {
		projected := buildSessionCompactionProjectionPayload(
			history.ExistingContinuation,
			history.PendingCompaction,
			history.Turns,
			budget.SummaryBudgetBytes,
		)
		if projected != nil {
			rawOrder := sessionHistoryRawOrder(history.CompletedOrder, projected)
			inputShape := buildSessionConversationInputsWithBudget(rawOrder, history.Turns, projected, budget)
			applySessionConversationShape(&history, rawOrder, inputShape.Inputs, inputShape.RawInputBytes, projected, budget.SummaryBudgetBytes)
		}
	}
	return history.Conversation, nil
}
