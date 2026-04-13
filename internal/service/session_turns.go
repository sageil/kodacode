package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/sageil/kodacode/v1/internal/repository"
)

// ErrSessionBusy is returned when a session already has an active turn.
var ErrSessionBusy = errors.New("session is already processing a turn")

// ErrNoActiveTurn is returned when cancellation is requested for a session
// that has no cancellable in-flight turn.
var ErrNoActiveTurn = errors.New("session has no active turn")

// ErrTurnOperationNotFound is returned when a caller asks for a specific turn
// operation that is not tracked for the session.
var ErrTurnOperationNotFound = errors.New("turn operation not found")

type sendReservationKey struct{}

type TurnState string

const (
	TurnStateQueued     TurnState = "queued"
	TurnStateIdle       TurnState = "idle"
	TurnStateRunning    TurnState = "running"
	TurnStateCancelling TurnState = "cancelling"
	TurnStateSucceeded  TurnState = "succeeded"
	TurnStateCancelled  TurnState = "cancelled"
	TurnStateFailed     TurnState = "failed"
)

type TurnStatus struct {
	SessionID         string
	OperationID       string
	State             TurnState
	Active            bool
	QueueDepth        int
	QueuedOperationID string
	CancelRequested   bool
	Error             string
	StartedAt         time.Time
	UpdatedAt         time.Time
	FinishedAt        time.Time
}

type sessionTurnBroker struct {
	mu         sync.Mutex
	active     map[string]*SendReservation
	current    map[string]TurnStatus
	mutating   map[string]int
	latest     map[string]string
	statuses   map[string]map[string]TurnStatus
	statusRepo repository.TurnOperationRepo
}

func newSessionTurnBroker(repos ...repository.TurnOperationRepo) *sessionTurnBroker {
	var repo repository.TurnOperationRepo
	if len(repos) > 0 {
		repo = repos[0]
	}
	b := &sessionTurnBroker{
		active:     make(map[string]*SendReservation),
		current:    make(map[string]TurnStatus),
		mutating:   make(map[string]int),
		statusRepo: repo,
	}
	if repo == nil {
		b.latest = make(map[string]string)
		b.statuses = make(map[string]map[string]TurnStatus)
	}
	return b
}

// SessionMutation reserves exclusive non-turn access for a session so branch
// and delete operations do not race active turns or each other.
type SessionMutation struct {
	sessionID string
	broker    *sessionTurnBroker
	released  atomic.Bool
}

func (m *SessionMutation) Release() {
	if m == nil || m.broker == nil || m.released.Swap(true) {
		return
	}
	m.broker.releaseMutation(m.sessionID)
}

// SendReservation reserves a single active turn slot for a session.
// It is primarily used by async callers so they can fail fast before
// dispatching work to a goroutine.
type SendReservation struct {
	sessionID   string
	operationID string
	broker      *sessionTurnBroker
	released    atomic.Bool

	mu              sync.Mutex
	cancel          context.CancelFunc
	cancelRequested bool
}

func (r *SendReservation) Context(ctx context.Context) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, sendReservationKey{}, r)
}

func (r *SendReservation) BindCancel(cancel context.CancelFunc) bool {
	if r == nil || cancel == nil {
		return true
	}
	r.mu.Lock()
	if r.released.Load() {
		r.mu.Unlock()
		return false
	}
	r.cancel = cancel
	cancelNow := r.cancelRequested
	r.mu.Unlock()
	if cancelNow {
		cancel()
	}
	return true
}

func (r *SendReservation) OperationID() string {
	if r == nil {
		return ""
	}
	return r.operationID
}

func (r *SendReservation) Status() TurnStatus {
	if r == nil || r.broker == nil {
		return TurnStatus{}
	}
	status, _ := r.broker.StatusByOperation(r.sessionID, r.operationID)
	return status
}

type cancelRequestState int

const (
	cancelRequestRejected cancelRequestState = iota
	cancelRequestQueued
	cancelRequestDelivered
	cancelRequestFinished
)

func (r *SendReservation) RequestCancel() cancelRequestState {
	if r == nil {
		return cancelRequestRejected
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.released.Load() {
		return cancelRequestFinished
	}
	r.cancelRequested = true
	cancel := r.cancel
	if cancel == nil {
		return cancelRequestQueued
	}
	cancel()
	return cancelRequestDelivered
}

func (r *SendReservation) Cancel() bool {
	state := r.RequestCancel()
	return state == cancelRequestQueued || state == cancelRequestDelivered
}

func (r *SendReservation) Complete(state TurnState, err error) {
	if r == nil || r.broker == nil {
		return
	}
	r.broker.complete(r, state, err)
}

func (r *SendReservation) Release() {
	if r == nil || r.broker == nil || r.released.Swap(true) {
		return
	}
	r.mu.Lock()
	r.cancel = nil
	r.mu.Unlock()
	r.broker.release(r)
}

func (b *sessionTurnBroker) Reserve(sessionID string) (*SendReservation, error) {
	b.mu.Lock()
	if _, ok := b.active[sessionID]; ok || b.mutating[sessionID] > 0 {
		b.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrSessionBusy, sessionID)
	}

	res := &SendReservation{
		sessionID:   sessionID,
		operationID: ulid.Make().String(),
		broker:      b,
	}
	b.active[sessionID] = res
	now := time.Now().UTC()
	status := TurnStatus{
		SessionID:   sessionID,
		OperationID: res.operationID,
		State:       TurnStateRunning,
		Active:      true,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	b.current[sessionID] = status
	if b.statusRepo == nil {
		if b.statuses[sessionID] == nil {
			b.statuses[sessionID] = make(map[string]TurnStatus)
		}
		b.latest[sessionID] = res.operationID
		b.statuses[sessionID][res.operationID] = status
	}
	b.mu.Unlock()

	if err := b.persistStatus(status); err != nil {
		b.mu.Lock()
		if current, ok := b.active[sessionID]; ok && current == res {
			delete(b.active, sessionID)
		}
		delete(b.current, sessionID)
		if b.statusRepo == nil {
			delete(b.latest, sessionID)
			if ops := b.statuses[sessionID]; ops != nil {
				delete(ops, res.operationID)
				if len(ops) == 0 {
					delete(b.statuses, sessionID)
				}
			}
		}
		b.mu.Unlock()
		return nil, err
	}
	return res, nil
}

func (b *sessionTurnBroker) release(res *SendReservation) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if current, ok := b.active[res.sessionID]; ok && current == res {
		delete(b.active, res.sessionID)
	}
}

func (b *sessionTurnBroker) AcquireMutation(sessionID string) (*SessionMutation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.active[sessionID]; ok || b.mutating[sessionID] > 0 {
		return nil, fmt.Errorf("%w: %s", ErrSessionBusy, sessionID)
	}
	b.mutating[sessionID]++
	return &SessionMutation{
		sessionID: sessionID,
		broker:    b,
	}, nil
}

func (b *sessionTurnBroker) releaseMutation(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n := b.mutating[sessionID]; n > 1 {
		b.mutating[sessionID] = n - 1
		return
	}
	delete(b.mutating, sessionID)
}

func (b *sessionTurnBroker) CleanupSession(sessionID string) {
	b.mu.Lock()
	delete(b.active, sessionID)
	delete(b.current, sessionID)
	delete(b.mutating, sessionID)
	delete(b.latest, sessionID)
	delete(b.statuses, sessionID)
	b.mu.Unlock()
}

func (b *sessionTurnBroker) Cancel(sessionID string) error {
	b.mu.Lock()
	res := b.active[sessionID]
	if res == nil {
		b.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNoActiveTurn, sessionID)
	}
	status, ok := b.current[sessionID]
	if !ok || status.OperationID != res.operationID {
		status = TurnStatus{
			SessionID:   sessionID,
			OperationID: res.operationID,
			State:       TurnStateCancelling,
			Active:      true,
			StartedAt:   time.Now().UTC(),
		}
	}
	status.State = TurnStateCancelling
	status.Active = true
	status.CancelRequested = true
	status.UpdatedAt = time.Now().UTC()
	b.current[sessionID] = status
	if b.statusRepo == nil {
		if b.statuses[sessionID] == nil {
			b.statuses[sessionID] = make(map[string]TurnStatus)
		}
		b.latest[sessionID] = res.operationID
		b.statuses[sessionID][res.operationID] = status
	}
	b.mu.Unlock()

	if err := b.persistStatus(status); err != nil {
		log.Printf("turns: persist cancelling status failed for %s/%s: %v", status.SessionID, status.OperationID, err)
	}

	switch res.RequestCancel() {
	case cancelRequestQueued, cancelRequestDelivered, cancelRequestFinished:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrNoActiveTurn, sessionID)
	}
}

func (b *sessionTurnBroker) Status(sessionID string) TurnStatus {
	b.mu.Lock()
	if status, ok := b.current[sessionID]; ok {
		b.mu.Unlock()
		return status
	}
	if b.statusRepo == nil {
		defer b.mu.Unlock()
		operationID := b.latest[sessionID]
		if operationID == "" {
			return TurnStatus{
				SessionID: sessionID,
				State:     TurnStateIdle,
			}
		}
		if status, ok := b.statuses[sessionID][operationID]; ok {
			return status
		}
		return TurnStatus{
			SessionID: sessionID,
			State:     TurnStateIdle,
		}
	}
	b.mu.Unlock()
	if status, err := b.loadLatestStatus(sessionID); err == nil {
		return status
	}
	return TurnStatus{
		SessionID: sessionID,
		State:     TurnStateIdle,
	}
}

func (b *sessionTurnBroker) StatusByOperation(sessionID, operationID string) (TurnStatus, error) {
	b.mu.Lock()
	if operationID == "" {
		if status, ok := b.current[sessionID]; ok {
			b.mu.Unlock()
			return status, nil
		}
		if b.statusRepo == nil {
			defer b.mu.Unlock()
			return b.statusLocked(sessionID), nil
		}
		b.mu.Unlock()
		if status, err := b.loadLatestStatus(sessionID); err == nil {
			return status, nil
		}
		return TurnStatus{SessionID: sessionID, State: TurnStateIdle}, nil
	}
	if status, ok := b.current[sessionID]; ok && status.OperationID == operationID {
		b.mu.Unlock()
		return status, nil
	}
	if b.statusRepo == nil {
		defer b.mu.Unlock()
		if status, ok := b.statuses[sessionID][operationID]; ok {
			return status, nil
		}
		return TurnStatus{}, fmt.Errorf("%w: %s/%s", ErrTurnOperationNotFound, sessionID, operationID)
	}
	b.mu.Unlock()
	status, err := b.loadStatusByOperation(sessionID, operationID)
	if err != nil {
		return TurnStatus{}, err
	}
	return status, nil
}

func (b *sessionTurnBroker) statusLocked(sessionID string) TurnStatus {
	operationID := b.latest[sessionID]
	if operationID == "" {
		return TurnStatus{
			SessionID: sessionID,
			State:     TurnStateIdle,
		}
	}
	if status, ok := b.statuses[sessionID][operationID]; ok {
		return status
	}
	return TurnStatus{
		SessionID: sessionID,
		State:     TurnStateIdle,
	}
}

func (b *sessionTurnBroker) complete(res *SendReservation, state TurnState, err error) {
	b.mu.Lock()
	status, ok := b.current[res.sessionID]
	if !ok || status.OperationID != res.operationID {
		status = TurnStatus{
			SessionID:   res.sessionID,
			OperationID: res.operationID,
			StartedAt:   time.Now().UTC(),
		}
	}
	now := time.Now().UTC()
	status.State = state
	status.Active = false
	status.UpdatedAt = now
	status.FinishedAt = now
	if err != nil {
		status.Error = err.Error()
	} else {
		status.Error = ""
	}
	b.current[res.sessionID] = status
	if b.statusRepo == nil {
		if b.statuses[res.sessionID] == nil {
			b.statuses[res.sessionID] = make(map[string]TurnStatus)
		}
		b.latest[res.sessionID] = res.operationID
		b.statuses[res.sessionID][res.operationID] = status
	}
	b.mu.Unlock()

	if persistErr := b.persistStatus(status); persistErr != nil {
		log.Printf("turns: persist terminal status failed for %s/%s: %v", status.SessionID, status.OperationID, persistErr)
	}
}

func (b *sessionTurnBroker) persistStatus(status TurnStatus) error {
	if b == nil || b.statusRepo == nil {
		return nil
	}
	return b.statusRepo.SaveTurnOperation(context.Background(), repository.TurnOperation{
		SessionID:       status.SessionID,
		OperationID:     status.OperationID,
		State:           string(status.State),
		Active:          status.Active,
		CancelRequested: status.CancelRequested,
		Error:           status.Error,
		StartedAt:       status.StartedAt,
		UpdatedAt:       status.UpdatedAt,
		FinishedAt:      status.FinishedAt,
	})
}

func (b *sessionTurnBroker) loadLatestStatus(sessionID string) (TurnStatus, error) {
	if b == nil || b.statusRepo == nil {
		return TurnStatus{}, repository.ErrNotFound
	}
	op, err := b.statusRepo.LatestTurnOperation(context.Background(), sessionID)
	if errors.Is(err, repository.ErrNotFound) {
		return TurnStatus{}, err
	}
	if err != nil {
		log.Printf("turns: latest status lookup failed for %s: %v", sessionID, err)
		return TurnStatus{}, err
	}
	return turnStatusFromRepo(op), nil
}

func (b *sessionTurnBroker) loadStatusByOperation(sessionID, operationID string) (TurnStatus, error) {
	if b == nil || b.statusRepo == nil {
		return TurnStatus{}, fmt.Errorf("%w: %s/%s", ErrTurnOperationNotFound, sessionID, operationID)
	}
	op, err := b.statusRepo.GetTurnOperation(context.Background(), sessionID, operationID)
	if errors.Is(err, repository.ErrNotFound) {
		return TurnStatus{}, fmt.Errorf("%w: %s/%s", ErrTurnOperationNotFound, sessionID, operationID)
	}
	if err != nil {
		return TurnStatus{}, err
	}
	return turnStatusFromRepo(op), nil
}

func turnStatusFromRepo(op repository.TurnOperation) TurnStatus {
	return TurnStatus{
		SessionID:       op.SessionID,
		OperationID:     op.OperationID,
		State:           TurnState(op.State),
		Active:          op.Active,
		CancelRequested: op.CancelRequested,
		Error:           op.Error,
		StartedAt:       op.StartedAt,
		UpdatedAt:       op.UpdatedAt,
		FinishedAt:      op.FinishedAt,
	}
}

func turnStateForError(err error) TurnState {
	switch {
	case err == nil:
		return TurnStateSucceeded
	case errors.Is(err, context.Canceled):
		return TurnStateCancelled
	default:
		return TurnStateFailed
	}
}

func completeTurnReservation(reservation *SendReservation, err error) {
	if reservation == nil {
		return
	}
	reservation.Complete(turnStateForError(err), err)
}

func (s *SessionService) ReserveSend(sessionID string) (*SendReservation, error) {
	if s == nil || s.state == nil {
		return nil, nil
	}
	return s.state.ReserveSend(sessionID)
}

func (s *SessionService) CancelTurn(_ context.Context, sessionID string) error {
	if s == nil || s.state == nil {
		return fmt.Errorf("%w: %s", ErrNoActiveTurn, sessionID)
	}
	return s.state.CancelTurn(sessionID)
}

func (s *SessionService) TurnStatus(_ context.Context, sessionID string) (TurnStatus, error) {
	if s == nil || s.state == nil {
		return TurnStatus{SessionID: sessionID, State: TurnStateIdle}, nil
	}
	return s.state.TurnStatus(sessionID), nil
}

func (s *SessionService) TurnStatusByOperation(_ context.Context, sessionID, operationID string) (TurnStatus, error) {
	if s == nil || s.state == nil {
		if operationID == "" {
			return TurnStatus{SessionID: sessionID, State: TurnStateIdle}, nil
		}
		return TurnStatus{}, fmt.Errorf("%w: %s/%s", ErrTurnOperationNotFound, sessionID, operationID)
	}
	return s.state.TurnStatusByOperation(sessionID, operationID)
}

func (s *SessionService) reserveSend(ctx context.Context, sessionID string) (*SendReservation, bool, error) {
	if s == nil || s.state == nil {
		return nil, false, nil
	}
	if res, ok := ctx.Value(sendReservationKey{}).(*SendReservation); ok && res != nil && res.sessionID == sessionID {
		return res, false, nil
	}
	res, err := s.state.ReserveSend(sessionID)
	if err != nil {
		return nil, false, err
	}
	return res, true, nil
}
