package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/sageil/kodacode/v1/internal/repository"
)

var _ repository.TraceRepo = (*traceRepo)(nil)

type traceRepo struct {
	db *sql.DB
}

func NewTraceRepo(db *sql.DB) repository.TraceRepo {
	return &traceRepo{db: db}
}

func (r *traceRepo) Save(ctx context.Context, sessionID string, turnIndex int, data string) error {
	now := time.Now().UnixMilli()
	const q = `INSERT OR REPLACE INTO session_traces (session_id, turn_index, data, created_at) VALUES (?, ?, ?, ?)`
	if _, err := r.db.ExecContext(ctx, q, sessionID, turnIndex, data, now); err != nil {
		return fmt.Errorf("save trace: %w", err)
	}
	return nil
}

func (r *traceRepo) ListBySession(ctx context.Context, sessionID string) ([]repository.TraceEntry, error) {
	const q = `SELECT session_id, turn_index, data, created_at FROM session_traces WHERE session_id = ? ORDER BY turn_index ASC`
	rows, err := r.db.QueryContext(ctx, q, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list traces: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var entries []repository.TraceEntry
	for rows.Next() {
		var e repository.TraceEntry
		var createdAt int64
		if err := rows.Scan(&e.SessionID, &e.TurnIndex, &e.Data, &createdAt); err != nil {
			return nil, fmt.Errorf("scan trace: %w", err)
		}
		e.CreatedAt = time.UnixMilli(createdAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *traceRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	const q = `DELETE FROM session_traces WHERE session_id = ?`
	if _, err := r.db.ExecContext(ctx, q, sessionID); err != nil {
		return fmt.Errorf("delete traces by session: %w", err)
	}
	return nil
}
