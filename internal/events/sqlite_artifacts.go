package events

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path"
	"strings"
	"time"
)

func (s *SQLiteStore) SaveToolResultBlob(
	ctx context.Context,
	ref string,
	sessionID string,
	turnID string,
	callID string,
	stream string,
	text string,
) (*ToolResultBlobRef, error) {
	if s == nil || s.db == nil || strings.TrimSpace(text) == "" {
		return nil, nil
	}
	clean, err := cleanSQLiteArtifactRef(ref)
	if err != nil {
		return nil, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := time.Now().UTC().UnixNano()
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO tool_result_blobs (
    ref, session_id, turn_id, call_id, stream, byte_count, content, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(ref) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    call_id = excluded.call_id,
    stream = excluded.stream,
    byte_count = excluded.byte_count,
    content = excluded.content,
    created_at = excluded.created_at
`,
		clean,
		sessionID,
		turnID,
		callID,
		stream,
		len(text),
		[]byte(text),
		now,
	); err != nil {
		return nil, err
	}
	return &ToolResultBlobRef{
		Ref:   clean,
		Bytes: len(text),
	}, nil
}

func (s *SQLiteStore) LoadToolResultBlob(ctx context.Context, ref string) (string, error) {
	if s == nil || s.db == nil {
		return "", os.ErrNotExist
	}
	clean, err := cleanSQLiteArtifactRef(ref)
	if err != nil {
		return "", err
	}
	var content []byte
	if err := s.db.QueryRowContext(ctx, `
SELECT content
FROM tool_result_blobs
WHERE ref = ?`,
		clean,
	).Scan(&content); err != nil {
		if err == sql.ErrNoRows {
			return "", os.ErrNotExist
		}
		return "", err
	}
	return string(content), nil
}

func (s *SQLiteStore) SaveBranchSummary(ctx context.Context, artifact BranchSummaryArtifact) error {
	if s == nil || s.db == nil {
		return os.ErrInvalid
	}
	sessionID := strings.TrimSpace(artifact.SessionID)
	summary := strings.TrimSpace(artifact.Summary)
	if sessionID == "" || summary == "" {
		return nil
	}
	now := time.Now().UTC()
	createdAt := artifact.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := artifact.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO branch_summaries (
    session_id, source_sequence, summary, model, prompt_tokens, completion_tokens, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    source_sequence = excluded.source_sequence,
    summary = excluded.summary,
    model = excluded.model,
    prompt_tokens = excluded.prompt_tokens,
    completion_tokens = excluded.completion_tokens,
    updated_at = excluded.updated_at
`,
		sessionID,
		max(artifact.SourceSequence, 0),
		summary,
		strings.TrimSpace(artifact.Model),
		max(artifact.PromptTokens, 0),
		max(artifact.CompletionTokens, 0),
		createdAt.UnixNano(),
		updatedAt.UnixNano(),
	)
	return err
}

func (s *SQLiteStore) LoadBranchSummary(ctx context.Context, sessionID string) (BranchSummaryArtifact, bool, error) {
	if s == nil || s.db == nil {
		return BranchSummaryArtifact{}, false, os.ErrInvalid
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return BranchSummaryArtifact{}, false, nil
	}
	var artifact BranchSummaryArtifact
	var createdAt int64
	var updatedAt int64
	if err := s.db.QueryRowContext(ctx, `
SELECT session_id, source_sequence, summary, model, prompt_tokens, completion_tokens, created_at, updated_at
FROM branch_summaries
WHERE session_id = ?`,
		sessionID,
	).Scan(
		&artifact.SessionID,
		&artifact.SourceSequence,
		&artifact.Summary,
		&artifact.Model,
		&artifact.PromptTokens,
		&artifact.CompletionTokens,
		&createdAt,
		&updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return BranchSummaryArtifact{}, false, nil
		}
		return BranchSummaryArtifact{}, false, err
	}
	artifact.CreatedAt = time.Unix(0, createdAt).UTC()
	artifact.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return artifact, true, nil
}

func (s *SQLiteStore) CreateBackgroundLog(ctx context.Context, ref, sessionID, turnID, executionID string) error {
	if s == nil || s.db == nil {
		return os.ErrInvalid
	}
	clean, err := cleanSQLiteArtifactRef(ref)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := time.Now().UTC().UnixNano()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if _, err := tx.ExecContext(ctx, `DELETE FROM background_logs WHERE ref = ?`, clean); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO background_logs (
    ref, session_id, turn_id, execution_id, byte_count, created_at, updated_at
) VALUES (?, ?, ?, ?, 0, ?, ?)`,
		clean,
		sessionID,
		turnID,
		executionID,
		now,
		now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) AppendBackgroundLogChunk(ctx context.Context, ref string, chunk []byte) error {
	if s == nil || s.db == nil {
		return os.ErrInvalid
	}
	if len(chunk) == 0 {
		return nil
	}
	clean, err := cleanSQLiteArtifactRef(ref)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var offset int64
	if err := tx.QueryRowContext(ctx, `
SELECT byte_count
FROM background_logs
WHERE ref = ?`,
		clean,
	).Scan(&offset); err != nil {
		if err == sql.ErrNoRows {
			return os.ErrNotExist
		}
		return err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO background_log_chunks (
    log_ref, start_offset, byte_count, content
) VALUES (?, ?, ?, ?)`,
		clean,
		offset,
		len(chunk),
		chunk,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE background_logs
SET byte_count = ?, updated_at = ?
WHERE ref = ?`,
		offset+int64(len(chunk)),
		time.Now().UTC().UnixNano(),
		clean,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ReadBackgroundLogTail(ctx context.Context, ref string, limit int) (string, int64, error) {
	size, err := s.backgroundLogSize(ctx, ref)
	if err != nil {
		return "", 0, err
	}
	if limit <= 0 || size == 0 {
		return "", size, nil
	}
	offset := size - int64(limit)
	if offset < 0 {
		offset = 0
	}
	return s.ReadBackgroundLogFrom(ctx, ref, offset, limit)
}

func (s *SQLiteStore) ReadBackgroundLogPrefix(ctx context.Context, ref string, limit int) (string, int64, error) {
	return s.ReadBackgroundLogFrom(ctx, ref, 0, limit)
}

func (s *SQLiteStore) ReadBackgroundLogFrom(ctx context.Context, ref string, offset int64, limit int) (string, int64, error) {
	if s == nil || s.db == nil {
		return "", 0, os.ErrInvalid
	}
	clean, err := cleanSQLiteArtifactRef(ref)
	if err != nil {
		return "", 0, err
	}
	size, err := s.backgroundLogSize(ctx, clean)
	if err != nil {
		return "", 0, err
	}
	if offset < 0 {
		offset = 0
	}
	if offset > size {
		offset = size
	}
	if limit <= 0 || size == 0 || offset >= size {
		return "", size, nil
	}
	limit = min(limit, int(size-offset))
	if limit == 0 {
		return "", size, nil
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT start_offset, content
FROM background_log_chunks
WHERE log_ref = ?
  AND start_offset < ?
  AND (start_offset + byte_count) > ?
ORDER BY start_offset ASC`,
		clean,
		offset+int64(limit),
		offset,
	)
	if err != nil {
		return "", size, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var buffer bytes.Buffer
	buffer.Grow(limit)
	remaining := limit
	for rows.Next() {
		var startOffset int64
		var content []byte
		if err := rows.Scan(&startOffset, &content); err != nil {
			return "", size, err
		}
		skip := 0
		if offset > startOffset {
			skip = int(offset - startOffset)
		}
		if skip >= len(content) {
			continue
		}
		content = content[skip:]
		if len(content) > remaining {
			content = content[:remaining]
		}
		buffer.Write(content)
		remaining -= len(content)
		if remaining == 0 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return "", size, err
	}
	return buffer.String(), size, nil
}

func (s *SQLiteStore) backgroundLogSize(ctx context.Context, ref string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, os.ErrInvalid
	}
	clean, err := cleanSQLiteArtifactRef(ref)
	if err != nil {
		return 0, err
	}
	var size int64
	if err := s.db.QueryRowContext(ctx, `
SELECT byte_count
FROM background_logs
WHERE ref = ?`,
		clean,
	).Scan(&size); err != nil {
		if err == sql.ErrNoRows {
			return 0, os.ErrNotExist
		}
		return 0, err
	}
	return size, nil
}

func cleanSQLiteArtifactRef(ref string) (string, error) {
	clean := path.Clean(strings.TrimSpace(ref))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", os.ErrPermission
	}
	return clean, nil
}
