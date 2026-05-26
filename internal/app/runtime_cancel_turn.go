package app

import (
	"context"
	"errors"
	"sync"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

var (
	ErrTurnNotRunning     = errors.New("turn is not running")
	ErrTurnAlreadyRunning = errors.New("turn is already running")
	ErrTurnCanceledByUser = errors.New("turn canceled by user")
)

type CancelSessionTurnInput struct {
	SessionID string
	TurnID    string
}

type activeTurnKey struct {
	sessionID string
	turnID    string
}

type activeTurnHandle struct {
	cancel context.CancelFunc

	mu              sync.Mutex
	cancelRequested bool
}

func (h *activeTurnHandle) requestCancel() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.cancelRequested = true
	h.mu.Unlock()
	h.cancel()
}

func (h *activeTurnHandle) canceled() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cancelRequested
}

type activeTurnRegistry struct {
	mu    sync.Mutex
	turns map[activeTurnKey]*activeTurnHandle
}

func (r *Runtime) beginCancelableTurnContext(
	ctx context.Context,
	sessionID, turnID string,
) (context.Context, *activeTurnHandle, func(), error) {
	key := activeTurnKey{
		sessionID: sessionID,
		turnID:    turnID,
	}
	childCtx, cancel := context.WithCancel(ctx)
	handle := &activeTurnHandle{cancel: cancel}

	r.activeTurns.mu.Lock()
	defer r.activeTurns.mu.Unlock()
	if r.activeTurns.turns == nil {
		r.activeTurns.turns = make(map[activeTurnKey]*activeTurnHandle)
	}
	if _, exists := r.activeTurns.turns[key]; exists {
		cancel()
		return nil, nil, nil, ErrTurnAlreadyRunning
	}
	r.activeTurns.turns[key] = handle

	cleanup := func() {
		cancel()
		r.activeTurns.mu.Lock()
		defer r.activeTurns.mu.Unlock()
		if r.activeTurns.turns[key] == handle {
			delete(r.activeTurns.turns, key)
		}
	}
	return childCtx, handle, cleanup, nil
}

func (r *Runtime) CancelSessionTurn(ctx context.Context, input CancelSessionTurnInput) error {
	if input.SessionID == "" {
		return ErrSessionIDRequired
	}
	if input.TurnID == "" {
		return ErrTurnIDRequired
	}

	key := activeTurnKey{
		sessionID: input.SessionID,
		turnID:    input.TurnID,
	}

	r.activeTurns.mu.Lock()
	handle := r.activeTurns.turns[key]
	r.activeTurns.mu.Unlock()
	if handle == nil {
		return r.cancelPendingSessionTurn(ctx, input.SessionID, input.TurnID)
	}

	r.log("runtime").Debug("session turn cancellation signaling",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
	)
	handle.requestCancel()
	r.log("runtime").Op("session turn cancellation requested",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
	)
	return nil
}

func (r *Runtime) cancelPendingSessionTurn(ctx context.Context, sessionID, turnID string) error {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	if !turnHasPendingInteraction(state, turnID) {
		r.log("runtime").Debug("session turn cancellation ignored; turn not running",
			"session_id", sessionID,
			"turn_id", turnID,
		)
		return ErrTurnNotRunning
	}
	if err := r.Runner.appendTurnCanceled(ctx, sessionID, turnID, ErrTurnCanceledByUser); err != nil {
		return err
	}
	r.log("runtime").Op("session pending interaction canceled",
		"session_id", sessionID,
		"turn_id", turnID,
	)
	return nil
}

func turnHasPendingInteraction(state events.SessionState, turnID string) bool {
	turn := state.Turns[turnID]
	if turnID == "" || turn == nil {
		return false
	}
	switch turn.Status {
	case events.TurnStatusCompleted, events.TurnStatusCanceled, events.TurnStatusFailed:
		return false
	}
	for _, request := range state.PendingExecutions {
		if request != nil && request.TurnID == turnID {
			return true
		}
	}
	for _, request := range state.PendingPermissions {
		if request != nil && request.TurnID == turnID {
			return true
		}
	}
	for _, request := range state.PendingQuestions {
		if request != nil && request.TurnID == turnID {
			return true
		}
	}
	if _, handoff := pendingDelegatedHandoffForTurn(state, turnID); handoff != nil {
		return true
	}
	return false
}

func pendingDelegatedHandoffForTurn(state events.SessionState, turnID string) (string, *events.AgentHandoffState) {
	turn := state.Turns[turnID]
	if turn == nil {
		return "", nil
	}
	for _, handoffID := range turn.HandoffOrder {
		handoff := turn.Handoffs[handoffID]
		if delegatedHandoffPending(handoff) {
			return handoffID, handoff
		}
	}
	return "", nil
}

func (r *Runtime) loadCanceledSessionTurnResult(sessionID, turnID, userText string, attachments []provider.Attachment) (RunSessionResult, error) {
	ctx := context.Background()
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	turn := state.Turns[turnID]
	if shouldAppendUserMessage(turn, userText, attachments) {
		if err := r.Runner.appendUserMessage(ctx, sessionID, turnID, userText, attachments); err != nil {
			return RunSessionResult{}, err
		}
	}
	if err := r.Runner.appendTurnCanceled(ctx, sessionID, turnID, ErrTurnCanceledByUser); err != nil {
		return RunSessionResult{}, err
	}
	return r.loadSessionTurnResult(ctx, sessionID, turnID, RunTurnResult{Status: TurnRunStatusCanceled})
}
