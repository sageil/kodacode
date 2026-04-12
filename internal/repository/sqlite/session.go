package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sageil/kodacode/v1/internal/repository"
)

// Compile-time interface check.
var _ repository.SessionRepo = (*sessionRepo)(nil)

type sessionRepo struct {
	db *sql.DB
}

// NewSessionRepo returns a SessionRepo backed by db.
func NewSessionRepo(db *sql.DB) repository.SessionRepo {
	return &sessionRepo{db: db}
}

func (r *sessionRepo) Create(ctx context.Context, s repository.Session) (repository.Session, error) {
	now := time.Now().UTC()
	s.ID = uuid.New().String()
	s.CreatedAt = now
	s.UpdatedAt = now

	const q = `
INSERT INTO sessions
    (id, title, agent_id, model_id, parent_id, branch_point_message_id, ephemeral, workflow_state, created_at, updated_at)
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, q,
		s.ID, s.Title, s.AgentID, s.ModelID,
		s.ParentID, s.BranchPointMessageID, boolToInt(s.Ephemeral), s.WorkflowState,
		s.CreatedAt.UnixNano(), s.UpdatedAt.UnixNano(),
	)
	if err != nil {
		return repository.Session{}, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

func (r *sessionRepo) Get(ctx context.Context, id string) (repository.Session, error) {
	const q = `
SELECT id, title, agent_id, model_id, parent_id, branch_point_message_id, ephemeral, total_cost, total_input_tokens, total_output_tokens, last_input_tokens, workflow_state, created_at, updated_at
FROM   sessions
WHERE  id = ?`

	row := r.db.QueryRowContext(ctx, q, id)
	s, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.Session{}, repository.ErrNotFound
	}
	if err != nil {
		return repository.Session{}, fmt.Errorf("get session %q: %w", id, err)
	}
	return s, nil
}

func (r *sessionRepo) List(ctx context.Context) ([]repository.Session, error) {
	const q = `
SELECT id, title, agent_id, model_id, parent_id, branch_point_message_id, ephemeral, total_cost, total_input_tokens, total_output_tokens, last_input_tokens, workflow_state, created_at, updated_at
FROM   sessions
WHERE  ephemeral = 0
ORDER  BY updated_at DESC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sessions []repository.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	return sessions, nil
}

func (r *sessionRepo) Update(ctx context.Context, s repository.Session) error {
	s.UpdatedAt = time.Now().UTC()
	const q = `
UPDATE sessions
SET    title = ?, agent_id = ?, model_id = ?, workflow_state = ?, updated_at = ?
WHERE  id = ?`

	res, err := r.db.ExecContext(ctx, q,
		s.Title, s.AgentID, s.ModelID, s.WorkflowState,
		s.UpdatedAt.UnixNano(),
		s.ID,
	)
	if err != nil {
		return fmt.Errorf("update session %q: %w", s.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update session rows affected: %w", err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *sessionRepo) Delete(ctx context.Context, id string) error {
	// Messages are removed by the ON DELETE CASCADE foreign key.
	const q = `DELETE FROM sessions WHERE id = ?`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete session %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete session rows affected: %w", err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *sessionRepo) DeleteEphemeral(ctx context.Context) (int, error) {
	const q = `DELETE FROM sessions WHERE ephemeral = 1`
	res, err := r.db.ExecContext(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("delete ephemeral sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanSession(s scanner) (repository.Session, error) {
	var (
		sess          repository.Session
		ephemeralInt  int
		createdAtNano int64
		updatedAtNano int64
	)
	err := s.Scan(
		&sess.ID, &sess.Title, &sess.AgentID, &sess.ModelID,
		&sess.ParentID, &sess.BranchPointMessageID, &ephemeralInt,
		&sess.TotalCost, &sess.TotalInputTokens, &sess.TotalOutputTokens,
		&sess.LastInputTokens, &sess.WorkflowState, &createdAtNano, &updatedAtNano,
	)
	if err != nil {
		return repository.Session{}, err
	}
	sess.Ephemeral = ephemeralInt != 0
	sess.CreatedAt = time.Unix(0, createdAtNano).UTC()
	sess.UpdatedAt = time.Unix(0, updatedAtNano).UTC()
	return sess, nil
}

func (r *sessionRepo) UpdateCost(ctx context.Context, id string, inputTokens, outputTokens, lastInputTokens int, totalCost float64) error {
	if inputTokens < 0 || outputTokens < 0 || lastInputTokens < 0 || totalCost < 0 {
		return fmt.Errorf("update session cost %q: negative values not allowed", id)
	}

	const q = `
UPDATE sessions
SET    total_input_tokens = ?, total_output_tokens = ?, last_input_tokens = ?, total_cost = ?, updated_at = ?
WHERE  id = ?`

	res, err := r.db.ExecContext(ctx, q, inputTokens, outputTokens, lastInputTokens, totalCost, time.Now().UTC().UnixNano(), id)
	if err != nil {
		return fmt.Errorf("update session cost %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *sessionRepo) UpdateWorkflow(ctx context.Context, id, workflowState string) error {
	const q = `
UPDATE sessions
SET    workflow_state = ?, updated_at = ?
WHERE  id = ?`

	res, err := r.db.ExecContext(ctx, q, workflowState, time.Now().UTC().UnixNano(), id)
	if err != nil {
		return fmt.Errorf("update session workflow %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return repository.ErrNotFound
	}
	return nil
}
