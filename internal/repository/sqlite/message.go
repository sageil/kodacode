package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/sageil/kodacode/v1/internal/repository"
)

// Compile-time interface check.
var _ repository.MessageRepo = (*messageRepo)(nil)

type messageRepo struct {
	db *sql.DB
}

type execContexter interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// NewMessageRepo returns a MessageRepo backed by db.
func NewMessageRepo(db *sql.DB) repository.MessageRepo {
	return &messageRepo{db: db}
}

func newULID() string {
	return ulid.Make().String()
}

func (r *messageRepo) Create(ctx context.Context, m repository.Message) (repository.Message, error) {
	err := insertMessage(ctx, r.db, &m)
	if err != nil {
		return repository.Message{}, fmt.Errorf("create message: %w", err)
	}
	return m, nil
}

func (r *messageRepo) CreateWithParts(ctx context.Context, m repository.Message, parts []repository.MessagePart) (repository.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return repository.Message{}, fmt.Errorf("begin create message with parts: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertMessage(ctx, tx, &m); err != nil {
		return repository.Message{}, fmt.Errorf("create message with parts: %w", err)
	}
	if len(parts) > 0 {
		createdParts := make([]repository.MessagePart, len(parts))
		for i := range parts {
			parts[i].MessageID = m.ID
			if parts[i].SessionID == "" {
				parts[i].SessionID = m.SessionID
			}
			if err := insertPart(ctx, tx, &parts[i]); err != nil {
				return repository.Message{}, fmt.Errorf("create message with parts: %w", err)
			}
			createdParts[i] = parts[i]
		}
		m.Parts = createdParts
	}
	if err := tx.Commit(); err != nil {
		return repository.Message{}, fmt.Errorf("commit create message with parts: %w", err)
	}
	return m, nil
}

func (r *messageRepo) Get(ctx context.Context, id string) (repository.Message, error) {
	const q = `
SELECT id, session_id, role, compaction_parent_id, summary, created_at, updated_at
FROM   messages WHERE id = ?`

	row := r.db.QueryRowContext(ctx, q, id)
	m, err := scanMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.Message{}, repository.ErrNotFound
	}
	if err != nil {
		return repository.Message{}, fmt.Errorf("get message %q: %w", id, err)
	}
	return m, nil
}

func (r *messageRepo) ListBySession(ctx context.Context, sessionID string) ([]repository.Message, error) {
	const q = `
SELECT id, session_id, role, compaction_parent_id, summary, created_at, updated_at
FROM   messages WHERE session_id = ? ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, q, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var msgs []repository.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (r *messageRepo) ListMessagesWithParts(ctx context.Context, sessionID string) ([]repository.Message, error) {
	const q = `
SELECT
	m.id, m.session_id, m.role, m.compaction_parent_id, m.summary, m.created_at, m.updated_at,
	p.id, p.message_id, p.session_id, p.type, p.content, p.synthetic, p.compacted_at, p.created_at
FROM messages m
LEFT JOIN message_parts p ON p.message_id = m.id
WHERE m.session_id = ?
ORDER BY m.created_at ASC, p.created_at ASC`

	rows, err := r.db.QueryContext(ctx, q, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list messages with parts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var msgs []repository.Message
	indexByID := make(map[string]int)
	for rows.Next() {
		var (
			msg             repository.Message
			msgCreatedAt    int64
			msgUpdatedAt    int64
			summaryInt      int
			partID          sql.NullString
			partMessageID   sql.NullString
			partSessionID   sql.NullString
			partType        sql.NullString
			partContent     sql.NullString
			partSynthetic   sql.NullInt64
			partCompactedAt sql.NullInt64
			partCreatedAt   sql.NullInt64
		)
		if err := rows.Scan(
			&msg.ID, &msg.SessionID, &msg.Role, &msg.CompactionParentID, &summaryInt, &msgCreatedAt, &msgUpdatedAt,
			&partID, &partMessageID, &partSessionID, &partType, &partContent, &partSynthetic, &partCompactedAt, &partCreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan message with parts: %w", err)
		}

		idx, ok := indexByID[msg.ID]
		if !ok {
			msg.Summary = summaryInt != 0
			msg.CreatedAt = time.Unix(0, msgCreatedAt).UTC()
			msg.UpdatedAt = time.Unix(0, msgUpdatedAt).UTC()
			msgs = append(msgs, msg)
			idx = len(msgs) - 1
			indexByID[msg.ID] = idx
		}

		if !partID.Valid {
			continue
		}

		part := repository.MessagePart{
			ID:        partID.String,
			MessageID: partMessageID.String,
			SessionID: partSessionID.String,
			Type:      partType.String,
			Content:   partContent.String,
			Synthetic: partSynthetic.Valid && partSynthetic.Int64 != 0,
		}
		if partCreatedAt.Valid {
			part.CreatedAt = time.Unix(0, partCreatedAt.Int64).UTC()
		}
		if partCompactedAt.Valid {
			t := time.Unix(0, partCompactedAt.Int64).UTC()
			part.CompactedAt = &t
		}
		msgs[idx].Parts = append(msgs[idx].Parts, part)
	}

	return msgs, rows.Err()
}

func (r *messageRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM messages WHERE session_id = ?`, sessionID)
	return err
}

func scanMessage(s scanner) (repository.Message, error) {
	var (
		m           repository.Message
		nano        int64
		updatedNano int64
		summaryInt  int
	)
	err := s.Scan(&m.ID, &m.SessionID, &m.Role, &m.CompactionParentID, &summaryInt, &nano, &updatedNano)
	if err != nil {
		return repository.Message{}, err
	}
	m.Summary = summaryInt != 0
	m.CreatedAt = time.Unix(0, nano).UTC()
	m.UpdatedAt = time.Unix(0, updatedNano).UTC()
	return m, nil
}

func (r *messageRepo) CreatePart(ctx context.Context, p repository.MessagePart) (repository.MessagePart, error) {
	err := insertPart(ctx, r.db, &p)
	if err != nil {
		return repository.MessagePart{}, fmt.Errorf("create part: %w", err)
	}
	return p, nil
}

func (r *messageRepo) ListPartsByMessage(ctx context.Context, messageID string) ([]repository.MessagePart, error) {
	const q = `
SELECT id, message_id, session_id, type, content, synthetic, compacted_at, created_at
FROM   message_parts WHERE message_id = ? ORDER BY created_at ASC`
	return r.queryParts(ctx, q, messageID)
}

func (r *messageRepo) ListPartsBySession(ctx context.Context, sessionID string) ([]repository.MessagePart, error) {
	const q = `
SELECT id, message_id, session_id, type, content, synthetic, compacted_at, created_at
FROM   message_parts WHERE session_id = ? ORDER BY created_at ASC`
	return r.queryParts(ctx, q, sessionID)
}

func (r *messageRepo) UpdatePart(ctx context.Context, p repository.MessagePart) error {
	var compactedNano *int64
	if p.CompactedAt != nil {
		n := p.CompactedAt.UnixNano()
		compactedNano = &n
	}
	const q = `UPDATE message_parts SET content = ?, compacted_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, q, p.Content, compactedNano, p.ID)
	return err
}

func (r *messageRepo) BatchUpdateParts(ctx context.Context, parts []repository.MessagePart) error {
	if len(parts) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `UPDATE message_parts SET content = ?, compacted_at = ? WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare batch update: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, p := range parts {
		var compactedNano *int64
		if p.CompactedAt != nil {
			n := p.CompactedAt.UnixNano()
			compactedNano = &n
		}
		if _, err := stmt.ExecContext(ctx, p.Content, compactedNano, p.ID); err != nil {
			return fmt.Errorf("batch update part %s: %w", p.ID, err)
		}
	}
	return tx.Commit()
}

func (r *messageRepo) DeletePart(ctx context.Context, partID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM message_parts WHERE id = ?`, partID)
	return err
}

func (r *messageRepo) DeletePartsBySession(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM message_parts WHERE session_id = ?`, sessionID)
	return err
}

func insertMessage(ctx context.Context, exec execContexter, m *repository.Message) error {
	m.ID = newULID()
	m.CreatedAt = time.Now().UTC()
	m.UpdatedAt = m.CreatedAt

	const q = `
INSERT INTO messages (id, session_id, role, compaction_parent_id, summary, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`

	summaryInt := 0
	if m.Summary {
		summaryInt = 1
	}
	_, err := exec.ExecContext(ctx, q,
		m.ID, m.SessionID, m.Role, m.CompactionParentID, summaryInt, m.CreatedAt.UnixNano(), m.UpdatedAt.UnixNano(),
	)
	return err
}

func insertPart(ctx context.Context, exec execContexter, p *repository.MessagePart) error {
	p.ID = newULID()
	p.CreatedAt = time.Now().UTC()

	const q = `
INSERT INTO message_parts (id, message_id, session_id, type, content, synthetic, compacted_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	synthInt := 0
	if p.Synthetic {
		synthInt = 1
	}
	var compactedNano *int64
	if p.CompactedAt != nil {
		n := p.CompactedAt.UnixNano()
		compactedNano = &n
	}
	_, err := exec.ExecContext(ctx, q,
		p.ID, p.MessageID, p.SessionID, p.Type, p.Content, synthInt, compactedNano, p.CreatedAt.UnixNano(),
	)
	return err
}

func (r *messageRepo) queryParts(ctx context.Context, q, arg string) ([]repository.MessagePart, error) {
	rows, err := r.db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, fmt.Errorf("query parts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var parts []repository.MessagePart
	for rows.Next() {
		p, err := scanPart(rows)
		if err != nil {
			return nil, fmt.Errorf("scan part: %w", err)
		}
		parts = append(parts, p)
	}
	return parts, rows.Err()
}

func scanPart(s scanner) (repository.MessagePart, error) {
	var (
		p             repository.MessagePart
		nano          int64
		synthInt      int
		compactedNano *int64
	)
	err := s.Scan(&p.ID, &p.MessageID, &p.SessionID, &p.Type, &p.Content, &synthInt, &compactedNano, &nano)
	if err != nil {
		return repository.MessagePart{}, err
	}
	p.Synthetic = synthInt != 0
	p.CreatedAt = time.Unix(0, nano).UTC()
	if compactedNano != nil {
		t := time.Unix(0, *compactedNano).UTC()
		p.CompactedAt = &t
	}
	return p, nil
}
