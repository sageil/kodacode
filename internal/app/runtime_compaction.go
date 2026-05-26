package app

import (
	"context"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

const (
	sessionHistoryManualCompactionReason = "manual_request"
	manualSessionCompactionInstructions  = "Continue the coding task using the preserved session history."
)

var ErrSessionCompactionTurnRunning = errors.New("finish the active turn before compacting history")
var ErrSessionCompactionBudgetUnavailable = errors.New("model input budget unavailable for session compaction")

type CompactSessionInput struct {
	SessionID string
	TurnID    string
}

type CompactSessionResult struct {
	TurnID       string
	Continuation *events.SessionHistoryContinuationUpdatedPayload
}

func (r *Runtime) CompactSessionHistory(ctx context.Context, input CompactSessionInput) (result CompactSessionResult, err error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return CompactSessionResult{}, ErrSessionIDRequired
	}
	turnID := strings.TrimSpace(input.TurnID)
	if turnID == "" {
		turnID = NewTurnID()
	}

	state, err := r.Sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return CompactSessionResult{}, err
	}
	inlineCompaction := false
	for _, existingTurnID := range state.TurnOrder {
		turn := state.Turns[existingTurnID]
		if turn == nil || turn.Status != events.TurnStatusRunning {
			continue
		}
		if existingTurnID == turnID {
			inlineCompaction = true
			continue
		}
		return CompactSessionResult{}, ErrSessionCompactionTurnRunning
	}

	modelRoute, err := r.Runner.loadSessionModelRoute(ctx, input.SessionID)
	if err != nil {
		return CompactSessionResult{}, err
	}

	request := sessionConversationRequest{
		SessionID:    input.SessionID,
		TurnID:       turnID,
		ModelRoute:   modelRoute,
		Instructions: manualSessionCompactionInstructions,
	}
	template, checkpointLoaded, replayedCount, err := r.Runner.loadSessionHistoryTemplateForRequest(ctx, request)
	if err != nil {
		return CompactSessionResult{}, err
	}
	history := r.Runner.projectSessionHistoryStateForRequest(ctx, request, template, checkpointLoaded, replayedCount)
	budget := resolveSessionHistoryBudget(modelRoute, r.Runner.models, r.Runner.sessionConfig)
	if !budget.resolved() {
		return CompactSessionResult{}, ErrSessionCompactionBudgetUnavailable
	}
	if sessionCompactionPrefixCount(history.CompletedOrder, history.ExistingContinuation) >= len(history.CompletedOrder) {
		return CompactSessionResult{}, nil
	}
	plan := buildSessionCompactionPlanForOrder(history.ExistingContinuation, history.CompletedOrder)
	if plan == nil {
		return CompactSessionResult{}, nil
	}
	plan.UpdateReason = sessionHistoryManualCompactionReason

	standaloneTurnOpen := !inlineCompaction
	if !inlineCompaction {
		defer func() {
			if !standaloneTurnOpen {
				return
			}
			if err != nil {
				if failErr := r.Runner.appendTurnError(ctx, input.SessionID, turnID, err); failErr != nil {
					err = errors.Join(err, failErr)
				}
				return
			}
			if finishErr := r.Runner.appendTurnDone(ctx, input.SessionID, turnID); finishErr != nil {
				err = finishErr
			}
		}()
	}

	if err := r.Runner.realizeSessionHistoryCompaction(ctx, request, &history, plan); err != nil {
		return CompactSessionResult{}, err
	}
	turnContext, err := r.Runner.loadTurnConversationContextState(ctx, input.SessionID, turnID)
	if err != nil {
		return CompactSessionResult{}, err
	}
	contextState := turnSessionConversationState{
		Pruning:             cloneContextPrunedPayload(&history.Conversation.Pruning),
		Continuation:        cloneCompactionPayload(history.Conversation.Continuation),
		DurablePruning:      cloneContextPrunedPayload(turnContext.SessionPruning),
		DurableContinuation: cloneCompactionPayload(turnContext.SessionContinuation),
	}
	if contextState.DurableContinuation == nil {
		contextState.DurableContinuation = cloneCompactionPayload(history.ExistingContinuation)
	}
	if err := r.Runner.commitTurnSessionConversationState(ctx, input.SessionID, turnID, &contextState); err != nil {
		return CompactSessionResult{}, err
	}
	if !inlineCompaction {
		if err := r.Runner.appendTurnDone(ctx, input.SessionID, turnID); err != nil {
			return CompactSessionResult{}, err
		}
		standaloneTurnOpen = false
	}
	if !inlineCompaction {
		if err := r.Runner.appendSessionHistoryCheckpoint(ctx, input.SessionID, modelRoute, history.Conversation.Continuation, nil, -1); err != nil {
			return CompactSessionResult{}, err
		}
	}

	return CompactSessionResult{
		TurnID:       turnID,
		Continuation: cloneCompactionPayload(history.Conversation.Continuation),
	}, nil
}
