package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func (r *TurnRunner) appendSessionHistoryCheckpoint(ctx context.Context, sessionID string, modelRoute provider.ModelRoute, currentCompaction *events.SessionHistoryContinuationUpdatedPayload, prepared *sessionHistoryState, replayAfterSequence int64) error {
	var err error
	var history sessionHistoryState
	if prepared != nil {
		history, err = r.loadSessionHistoryStateFromBase(ctx, sessionID, "", modelRoute, *prepared, replayAfterSequence)
		if err != nil {
			return err
		}
	} else {
		history, err = r.loadSessionHistoryStateForModel(ctx, sessionID, "", modelRoute)
		if err != nil {
			return err
		}
	}
	if history.ThroughSequence < 0 {
		r.logger.Debug("session history checkpoint skipped",
			"session_id", sessionID,
			"reason", "negative_through_sequence",
			"through_sequence", history.ThroughSequence,
		)
		return nil
	}
	latest, err := r.loadLatestSessionHistoryCheckpoint(ctx, sessionID)
	if err != nil {
		return err
	}
	if latest != nil && latest.ThroughSequence == history.ThroughSequence {
		r.logger.Debug("session history checkpoint skipped",
			"session_id", sessionID,
			"reason", "already_current",
			"through_sequence", history.ThroughSequence,
		)
		return nil
	}
	budget := resolveSessionHistoryBudget(modelRoute, r.models, r.sessionConfig)

	payload := events.SessionHistoryCheckpointPayload{
		ThroughSequence:  history.ThroughSequence,
		CompletedTurnIDs: append([]string(nil), history.CompletedOrder...),
		Turns:            make([]events.SessionHistoryTurnPayload, 0, len(history.CompletedOrder)),
	}
	effectiveCompaction := cloneCompactionPayload(history.ExistingContinuation)
	if history.Conversation.Continuation != nil {
		effectiveCompaction = cloneCompactionPayload(history.Conversation.Continuation)
	}
	if sameSessionCompactionScope(currentCompaction, effectiveCompaction) {
		effectiveCompaction = overlaySessionCompactionPayloadMetadata(effectiveCompaction, currentCompaction)
	}
	payload.Continuation = checkpointPersistedCompaction(
		effectiveCompaction,
		budget,
		history.CompletedOrder,
	)

	rawOrder := sessionHistoryRawOrder(history.CompletedOrder, payload.Continuation)
	for _, turnID := range rawOrder {
		turn := history.Turns[turnID]
		if turn == nil {
			continue
		}
		payload.Turns = append(payload.Turns, checkpointTurnPayload(turn))
	}
	_, err = r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionHistoryCheckpoint,
		Payload:   payload,
	})
	if err == nil {
		r.logSessionHistoryCheckpoint(sessionID, payload, len(rawOrder))
	}
	return err
}

func checkpointPersistedCompaction(
	payload *events.SessionHistoryContinuationUpdatedPayload,
	budget sessionHistoryBudget,
	completedOrder []string,
) *events.SessionHistoryContinuationUpdatedPayload {
	normalized := normalizeSessionCompactionPayload(payload, budget, completedOrder)
	if normalized == nil {
		return nil
	}
	return normalized
}

func (r *TurnRunner) loadSessionHistoryStateFromBase(ctx context.Context, sessionID, currentTurnID string, modelRoute provider.ModelRoute, base sessionHistoryState, replayAfterSequence int64) (sessionHistoryState, error) {
	if replayAfterSequence < -1 {
		replayAfterSequence = -1
	}
	replayed, err := r.sessions.store.Replay(ctx, events.Query{
		SessionID:     sessionID,
		AfterSequence: replayAfterSequence,
		ExcludeTypes:  []events.Type{events.TypePromptCompiled, events.TypeSessionHistoryCheckpoint, events.TypeSessionStateSnapshot},
	})
	if err != nil {
		return sessionHistoryState{}, err
	}
	checkpoint := sessionHistoryCheckpointFromState(base)
	request := sessionConversationRequest{
		SessionID:  sessionID,
		TurnID:     currentTurnID,
		ModelRoute: modelRoute,
	}.providerRequest()
	return buildSessionConversationStateWithBudgetAndResolverAndBlobs(
		ctx,
		r.sessions.blobs,
		replayed,
		currentTurnID,
		checkpoint,
		resolveSessionHistoryBudget(modelRoute, r.models, r.sessionConfig),
		request,
		nil,
		sessionHistoryMutationPathResolverForExecutor(r.tools),
	)
}

func sessionHistoryCheckpointFromState(history sessionHistoryState) *sessionHistoryCheckpoint {
	if history.ThroughSequence < 0 && len(history.CompletedOrder) == 0 && history.ExistingContinuation == nil {
		return nil
	}
	checkpoint := &sessionHistoryCheckpoint{
		ThroughSequence: history.ThroughSequence,
		Continuation:    cloneCompactionPayload(history.ExistingContinuation),
		CompletedOrder:  append([]string(nil), history.CompletedOrder...),
	}
	if len(history.Turns) > 0 {
		checkpoint.Turns = make(map[string]*replayedSessionTurn, len(history.Turns))
		for turnID, turn := range history.Turns {
			checkpoint.Turns[turnID] = cloneReplayedSessionTurn(turn)
		}
	}
	return checkpoint
}

func (r *TurnRunner) loadSessionModelRoute(ctx context.Context, sessionID string) (provider.ModelRoute, error) {
	if r == nil || r.sessions == nil {
		return provider.ModelRoute{}, nil
	}
	state, err := r.sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return provider.ModelRoute{}, err
	}
	if strings.TrimSpace(state.Model) == "" {
		return provider.ModelRoute{}, nil
	}
	primary, err := provider.ParseModelRef(state.Model)
	if err != nil {
		return provider.ModelRoute{}, err
	}
	route := provider.ModelRoute{Primary: primary}
	if err := route.Validate(); err != nil {
		return provider.ModelRoute{}, err
	}
	return route, nil
}
