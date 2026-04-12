package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/sageil/kodacode/v1/internal/repository"
)

var _ repository.TaskRepo = (*taskRepo)(nil)

type taskRepo struct {
	db *sql.DB
}

func NewTaskRepo(db *sql.DB) repository.TaskRepo {
	return &taskRepo{db: db}
}

func (r *taskRepo) Create(ctx context.Context, t repository.Task) (repository.Task, error) {
	now := time.Now().UnixMilli()
	t.CreatedAt = time.UnixMilli(now)
	t.UpdatedAt = t.CreatedAt
	const q = `INSERT INTO tasks (id, session_id, title, kind, status, notes, progress, review_status, block_reason, last_review_summary, sort_order, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := r.db.ExecContext(ctx, q, t.ID, t.SessionID, t.Title, t.Kind, t.Status, t.Notes, t.Progress, t.ReviewStatus, t.BlockReason, t.LastReviewSummary, t.SortOrder, now, now); err != nil {
		return t, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

func (r *taskRepo) Update(ctx context.Context, t repository.Task) error {
	now := time.Now().UnixMilli()
	const q = `UPDATE tasks SET title = ?, kind = ?, status = ?, notes = ?, progress = ?, review_status = ?, block_reason = ?, last_review_summary = ?, updated_at = ? WHERE session_id = ? AND id = ?`
	res, err := r.db.ExecContext(ctx, q, t.Title, t.Kind, t.Status, t.Notes, t.Progress, t.ReviewStatus, t.BlockReason, t.LastReviewSummary, now, t.SessionID, t.ID)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *taskRepo) Delete(ctx context.Context, sessionID, taskID string) error {
	const q = `DELETE FROM tasks WHERE session_id = ? AND id = ?`
	res, err := r.db.ExecContext(ctx, q, sessionID, taskID)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *taskRepo) ListBySession(ctx context.Context, sessionID string) ([]repository.Task, error) {
	const q = `SELECT id, session_id, title, kind, status, notes, progress, review_status, block_reason, last_review_summary, sort_order, created_at, updated_at FROM tasks WHERE session_id = ? ORDER BY sort_order ASC, created_at ASC`
	rows, err := r.db.QueryContext(ctx, q, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var tasks []repository.Task
	for rows.Next() {
		var t repository.Task
		var createdAt, updatedAt int64
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Title, &t.Kind, &t.Status, &t.Notes, &t.Progress, &t.ReviewStatus, &t.BlockReason, &t.LastReviewSummary, &t.SortOrder, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		t.CreatedAt = time.UnixMilli(createdAt)
		t.UpdatedAt = time.UnixMilli(updatedAt)
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (r *taskRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	const q = `DELETE FROM tasks WHERE session_id = ?`
	_, err := r.db.ExecContext(ctx, q, sessionID)
	if err != nil {
		return fmt.Errorf("delete tasks by session: %w", err)
	}
	return nil
}
