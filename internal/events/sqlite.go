package events

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

type SQLiteStore struct {
	db *sql.DB

	mu       sync.Mutex
	writeMu  sync.Mutex
	watchers map[string]map[*watcher]struct{}
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	return &SQLiteStore{
		db:       db,
		watchers: make(map[string]map[*watcher]struct{}),
	}, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) Append(ctx context.Context, draft Draft) (Event, error) {
	if err := draft.Validate(); err != nil {
		return Event{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	sequence, err := nextSQLiteSequence(ctx, tx, draft.SessionID)
	if err != nil {
		return Event{}, err
	}
	event := Event{
		ID:        fmt.Sprintf("%s:%d", draft.SessionID, sequence),
		SessionID: draft.SessionID,
		TurnID:    draft.TurnID,
		Sequence:  sequence,
		Time:      time.Now().UTC(),
		Type:      draft.Type,
		Payload:   draft.Payload,
	}
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	record, err := encodeEvent(event)
	if err != nil {
		return Event{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO session_events (
    session_id, sequence, event_id, turn_id, created_at, type, record
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.SessionID,
		event.Sequence,
		event.ID,
		event.TurnID,
		event.Time.UnixNano(),
		string(event.Type),
		record,
	); err != nil {
		return Event{}, err
	}
	if err := upsertSQLiteSession(ctx, tx, event); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	for w := range s.watchers[draft.SessionID] {
		w.push(event)
	}
	return event, nil
}

func (s *SQLiteStore) Replay(ctx context.Context, query Query) ([]Event, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	return s.replay(ctx, query)
}

func (s *SQLiteStore) Latest(ctx context.Context, query LatestQuery) (Event, bool, error) {
	if err := query.Validate(); err != nil {
		return Event{}, false, err
	}

	args := make([]any, 0, 1+len(query.Types))
	args = append(args, query.SessionID)
	placeholders := make([]string, 0, len(query.Types))
	for _, typ := range query.Types {
		placeholders = append(placeholders, "?")
		args = append(args, string(typ))
	}

	row := s.db.QueryRowContext(ctx, `
SELECT record
FROM session_events
WHERE session_id = ? AND type IN (`+strings.Join(placeholders, ",")+`)
ORDER BY sequence DESC
LIMIT 1`, args...)

	var record []byte
	if err := row.Scan(&record); err != nil {
		if err == sql.ErrNoRows {
			return Event{}, false, nil
		}
		return Event{}, false, err
	}
	event, err := decodeEvent(record)
	if err != nil {
		return Event{}, false, err
	}
	return event, true, nil
}

func (s *SQLiteStore) Watch(ctx context.Context, query Query) (<-chan Event, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	w := newWatcher(query)

	s.mu.Lock()
	events, err := s.replay(ctx, query)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	w.queue = events
	if s.watchers[query.SessionID] == nil {
		s.watchers[query.SessionID] = make(map[*watcher]struct{})
	}
	s.watchers[query.SessionID][w] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.watchers[query.SessionID], w)
			if len(s.watchers[query.SessionID]) == 0 {
				delete(s.watchers, query.SessionID)
			}
			s.mu.Unlock()
		}()
		w.run(ctx)
	}()

	return w.out, nil
}

func (s *SQLiteStore) LatestWorkspaceSessionID(ctx context.Context, workspaceRoot string) (string, bool, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return "", false, nil
	}
	var sessionID string
	err := s.db.QueryRowContext(ctx, `
SELECT session_id
FROM kodacode_session_index
WHERE workspace_root = ?
ORDER BY updated_at DESC
LIMIT 1`,
		workspaceRoot,
	).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return sessionID, true, nil
}

func (s *SQLiteStore) ListWorkspaceSessions(ctx context.Context, workspaceRoot string) ([]SessionIndexEntry, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT session_id, workspace_root, updated_at
FROM kodacode_session_index
WHERE workspace_root = ?
ORDER BY updated_at DESC`,
		workspaceRoot,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var sessions []SessionIndexEntry
	for rows.Next() {
		var sessionID string
		var root string
		var updatedAt int64
		if err := rows.Scan(&sessionID, &root, &updatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, SessionIndexEntry{
			SessionID:     sessionID,
			WorkspaceRoot: root,
			UpdatedAt:     time.Unix(0, updatedAt).UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *SQLiteStore) ListSessions(ctx context.Context) ([]SessionIndexEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT session_id, workspace_root, updated_at
FROM kodacode_session_index
ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var sessions []SessionIndexEntry
	for rows.Next() {
		var sessionID string
		var root string
		var updatedAt int64
		if err := rows.Scan(&sessionID, &root, &updatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, SessionIndexEntry{
			SessionID:     sessionID,
			WorkspaceRoot: root,
			UpdatedAt:     time.Unix(0, updatedAt).UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM session_events WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tool_result_blobs WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM background_logs WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM branch_summaries WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kodacode_session_index WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	for w := range s.watchers[sessionID] {
		w.close()
	}
	delete(s.watchers, sessionID)
	return nil
}

func (s *SQLiteStore) replay(ctx context.Context, query Query) ([]Event, error) {
	sqlText := `
SELECT record
FROM session_events
WHERE session_id = ? AND sequence > ?`
	args := []any{query.SessionID, query.AfterSequence}
	if len(query.ExcludeTypes) > 0 {
		placeholders := make([]string, 0, len(query.ExcludeTypes))
		for _, typ := range query.ExcludeTypes {
			placeholders = append(placeholders, "?")
			args = append(args, string(typ))
		}
		sqlText += ` AND type NOT IN (` + strings.Join(placeholders, ",") + `)`
	}
	sqlText += `
ORDER BY sequence ASC`
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var events []Event
	for rows.Next() {
		var record []byte
		if err := rows.Scan(&record); err != nil {
			return nil, err
		}
		event, err := decodeEvent(record)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func nextSQLiteSequence(ctx context.Context, tx *sql.Tx, sessionID string) (int64, error) {
	var sequence int64
	err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(sequence) + 1, 0)
FROM session_events
WHERE session_id = ?`,
		sessionID,
	).Scan(&sequence)
	return sequence, err
}

func upsertSQLiteSession(ctx context.Context, tx *sql.Tx, event Event) error {
	updatedAt := event.Time.UnixNano()
	if event.Type == TypeSessionConfigured {
		payload, ok := event.Payload.(SessionConfiguredPayload)
		if !ok {
			return fmt.Errorf("session_configured payload = %T", event.Payload)
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO kodacode_session_index (session_id, workspace_root, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    workspace_root = excluded.workspace_root,
    updated_at = excluded.updated_at`,
			event.SessionID,
			payload.WorkspaceRoot,
			updatedAt,
		)
		return err
	}
	if event.Type == TypeSessionStateSnapshot {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
UPDATE kodacode_session_index
SET updated_at = ?
WHERE session_id = ?`,
		updatedAt,
		event.SessionID,
	)
	return err
}
