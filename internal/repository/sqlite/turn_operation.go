package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sageil/kodacode/v1/internal/repository"
)

var _ repository.TurnOperationRepo = (*sessionRepo)(nil)

func (r *sessionRepo) SaveTurnOperation(ctx context.Context, op repository.TurnOperation) error {
	if op.SessionID == "" || op.OperationID == "" {
		return fmt.Errorf("save turn operation: session_id and operation_id are required")
	}
	if op.StartedAt.IsZero() {
		return fmt.Errorf("save turn operation %q/%q: started_at is required", op.SessionID, op.OperationID)
	}
	if op.UpdatedAt.IsZero() {
		return fmt.Errorf("save turn operation %q/%q: updated_at is required", op.SessionID, op.OperationID)
	}

	const q = `
INSERT INTO turn_operations (
    session_id, operation_id, state, active, cancel_requested, error, started_at, updated_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id, operation_id) DO UPDATE SET
    state = excluded.state,
    active = excluded.active,
    cancel_requested = excluded.cancel_requested,
    error = excluded.error,
    started_at = excluded.started_at,
    updated_at = excluded.updated_at,
    finished_at = excluded.finished_at`

	_, err := r.db.ExecContext(
		ctx,
		q,
		op.SessionID,
		op.OperationID,
		op.State,
		boolToInt(op.Active),
		boolToInt(op.CancelRequested),
		op.Error,
		op.StartedAt.UnixNano(),
		op.UpdatedAt.UnixNano(),
		timeToUnixNano(op.FinishedAt),
	)
	if err != nil {
		return fmt.Errorf("save turn operation %q/%q: %w", op.SessionID, op.OperationID, err)
	}
	return nil
}

func (r *sessionRepo) GetTurnOperation(ctx context.Context, sessionID, operationID string) (repository.TurnOperation, error) {
	const q = `
SELECT session_id, operation_id, state, active, cancel_requested, error, started_at, updated_at, finished_at
FROM   turn_operations
WHERE  session_id = ? AND operation_id = ?`

	row := r.db.QueryRowContext(ctx, q, sessionID, operationID)
	op, err := scanTurnOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.TurnOperation{}, repository.ErrNotFound
	}
	if err != nil {
		return repository.TurnOperation{}, fmt.Errorf("get turn operation %q/%q: %w", sessionID, operationID, err)
	}
	return op, nil
}

func (r *sessionRepo) LatestTurnOperation(ctx context.Context, sessionID string) (repository.TurnOperation, error) {
	const q = `
SELECT session_id, operation_id, state, active, cancel_requested, error, started_at, updated_at, finished_at
FROM   turn_operations
WHERE  session_id = ?
ORDER  BY updated_at DESC, started_at DESC
LIMIT  1`

	row := r.db.QueryRowContext(ctx, q, sessionID)
	op, err := scanTurnOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.TurnOperation{}, repository.ErrNotFound
	}
	if err != nil {
		return repository.TurnOperation{}, fmt.Errorf("latest turn operation %q: %w", sessionID, err)
	}
	return op, nil
}

type turnOperationScanner interface {
	Scan(dest ...any) error
}

func scanTurnOperation(s turnOperationScanner) (repository.TurnOperation, error) {
	var (
		op             repository.TurnOperation
		activeInt      int
		cancelReqInt   int
		startedAtNano  int64
		updatedAtNano  int64
		finishedAtNano int64
	)
	if err := s.Scan(
		&op.SessionID,
		&op.OperationID,
		&op.State,
		&activeInt,
		&cancelReqInt,
		&op.Error,
		&startedAtNano,
		&updatedAtNano,
		&finishedAtNano,
	); err != nil {
		return repository.TurnOperation{}, err
	}
	op.Active = activeInt != 0
	op.CancelRequested = cancelReqInt != 0
	op.StartedAt = time.Unix(0, startedAtNano).UTC()
	op.UpdatedAt = time.Unix(0, updatedAtNano).UTC()
	if finishedAtNano > 0 {
		op.FinishedAt = time.Unix(0, finishedAtNano).UTC()
	}
	return op, nil
}

func timeToUnixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixNano()
}
