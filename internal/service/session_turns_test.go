package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/repository"
)

type fakeTurnOperationRepo struct {
	ops map[string]repository.TurnOperation
}

func (r *fakeTurnOperationRepo) SaveTurnOperation(_ context.Context, op repository.TurnOperation) error {
	if r.ops == nil {
		r.ops = make(map[string]repository.TurnOperation)
	}
	r.ops[op.SessionID+"/"+op.OperationID] = op
	return nil
}

func (r *fakeTurnOperationRepo) GetTurnOperation(_ context.Context, sessionID, operationID string) (repository.TurnOperation, error) {
	op, ok := r.ops[sessionID+"/"+operationID]
	if !ok {
		return repository.TurnOperation{}, repository.ErrNotFound
	}
	return op, nil
}

func (r *fakeTurnOperationRepo) LatestTurnOperation(_ context.Context, sessionID string) (repository.TurnOperation, error) {
	var (
		latest repository.TurnOperation
		found  bool
	)
	for _, op := range r.ops {
		if op.SessionID != sessionID {
			continue
		}
		if !found || op.UpdatedAt.After(latest.UpdatedAt) || (op.UpdatedAt.Equal(latest.UpdatedAt) && op.StartedAt.After(latest.StartedAt)) {
			latest = op
			found = true
		}
	}
	if !found {
		return repository.TurnOperation{}, repository.ErrNotFound
	}
	return latest, nil
}

func TestSendReservationCancelCancelsBoundContext(t *testing.T) {
	broker := newSessionTurnBroker()
	reservation, err := broker.Reserve("s1")
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if !reservation.BindCancel(cancel) {
		t.Fatal("BindCancel() = false, want true")
	}

	if err := broker.Cancel("s1"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("ctx.Err() = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("bound context was not cancelled")
	}
}

func TestSendReservationCancelReturnsNoActiveTurnAfterRelease(t *testing.T) {
	broker := newSessionTurnBroker()
	reservation, err := broker.Reserve("s1")
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	reservation.Release()

	err = broker.Cancel("s1")
	if !errors.Is(err, ErrNoActiveTurn) {
		t.Fatalf("Cancel() error = %v, want ErrNoActiveTurn", err)
	}
}

func TestSendReservationCancelBeforeBindQueuesCancellation(t *testing.T) {
	broker := newSessionTurnBroker()
	reservation, err := broker.Reserve("s1")
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	if err := broker.Cancel("s1"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !reservation.BindCancel(cancel) {
		t.Fatal("BindCancel() = false, want true")
	}

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("ctx.Err() = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("bound context was not cancelled after queued cancel request")
	}
}

func TestSendReservationTracksTurnStatusLifecycle(t *testing.T) {
	broker := newSessionTurnBroker()
	reservation, err := broker.Reserve("s1")
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !reservation.BindCancel(cancel) {
		t.Fatal("BindCancel() = false, want true")
	}
	_ = ctx

	running := broker.Status("s1")
	if running.OperationID == "" {
		t.Fatal("OperationID = empty, want generated operation id")
	}
	if running.State != TurnStateRunning || !running.Active {
		t.Fatalf("running status = %+v, want active running turn", running)
	}

	if err := broker.Cancel("s1"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	cancelling := broker.Status("s1")
	if cancelling.State != TurnStateCancelling {
		t.Fatalf("State = %q, want %q", cancelling.State, TurnStateCancelling)
	}
	if !cancelling.CancelRequested {
		t.Fatal("CancelRequested = false, want true")
	}

	reservation.Complete(TurnStateCancelled, nil)
	reservation.Release()

	done := broker.Status("s1")
	if done.Active {
		t.Fatal("Active = true, want false after completion")
	}
	if done.State != TurnStateCancelled {
		t.Fatalf("State = %q, want %q", done.State, TurnStateCancelled)
	}
	if done.FinishedAt.IsZero() {
		t.Fatal("FinishedAt = zero, want completion timestamp")
	}
}

func TestSendReservationStatusByOperation(t *testing.T) {
	broker := newSessionTurnBroker()
	reservation, err := broker.Reserve("s1")
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	got, err := broker.StatusByOperation("s1", reservation.OperationID())
	if err != nil {
		t.Fatalf("StatusByOperation() error = %v", err)
	}
	if got.OperationID != reservation.OperationID() {
		t.Fatalf("OperationID = %q, want %q", got.OperationID, reservation.OperationID())
	}

	if _, err := broker.StatusByOperation("s1", "missing"); !errors.Is(err, ErrTurnOperationNotFound) {
		t.Fatalf("StatusByOperation(missing) error = %v, want ErrTurnOperationNotFound", err)
	}
}

func TestSendReservationStatusPersistsAcrossBrokerRestart(t *testing.T) {
	repo := &fakeTurnOperationRepo{}
	broker := newSessionTurnBroker(repo)
	reservation, err := broker.Reserve("s1")
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	reservation.Complete(TurnStateSucceeded, nil)
	reservation.Release()

	restarted := newSessionTurnBroker(repo)
	status := restarted.Status("s1")
	if status.State != TurnStateSucceeded || status.OperationID != reservation.OperationID() {
		t.Fatalf("Status() after restart = %+v, want persisted succeeded status", status)
	}

	byOp, err := restarted.StatusByOperation("s1", reservation.OperationID())
	if err != nil {
		t.Fatalf("StatusByOperation() after restart error = %v", err)
	}
	if byOp.State != TurnStateSucceeded {
		t.Fatalf("StatusByOperation().State = %q, want %q", byOp.State, TurnStateSucceeded)
	}
}
