package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/repository"
)

var _ repository.AttachmentRepo = (*attachmentRepo)(nil)

type attachmentRepo struct {
	db *sql.DB
	mu sync.Mutex
}

func NewAttachmentRepo(db *sql.DB) repository.AttachmentRepo {
	return &attachmentRepo{db: db}
}

func (r *attachmentRepo) ApplyDeltas(ctx context.Context, deltas []repository.AttachmentRefDelta) ([]repository.AttachmentBlob, error) {
	if len(deltas) == 0 {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin attachment delta tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC().UnixNano()
	for _, delta := range deltas {
		if delta.StorageKey == "" || delta.Delta == 0 {
			continue
		}
		if delta.Delta > 0 {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO attachment_blobs (storage_key, mime_type, size, ref_count, updated_at)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(storage_key) DO UPDATE SET
					mime_type = excluded.mime_type,
					size = excluded.size,
					ref_count = attachment_blobs.ref_count + excluded.ref_count,
					updated_at = excluded.updated_at`,
				delta.StorageKey, delta.MimeType, delta.Size, delta.Delta, now,
			); err != nil {
				return nil, fmt.Errorf("upsert attachment blob %q: %w", delta.StorageKey, err)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE attachment_blobs
			SET ref_count = ref_count + ?, updated_at = ?
			WHERE storage_key = ?`,
			delta.Delta, now, delta.StorageKey,
		); err != nil {
			return nil, fmt.Errorf("decrement attachment blob %q: %w", delta.StorageKey, err)
		}
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT storage_key, mime_type, size, ref_count, updated_at
		FROM attachment_blobs
		WHERE ref_count <= 0`)
	if err != nil {
		return nil, fmt.Errorf("list zero-ref blobs: %w", err)
	}
	zero, err := scanAttachmentBlobs(rows)
	if err != nil {
		return nil, err
	}
	for _, blob := range zero {
		if blob.RefCount < 0 {
			log.Printf("attachment refs: storage_key=%s has negative ref_count=%d", blob.StorageKey, blob.RefCount)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit attachment delta tx: %w", err)
	}
	return zero, nil
}

func (r *attachmentRepo) List(ctx context.Context) ([]repository.AttachmentBlob, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT storage_key, mime_type, size, ref_count, updated_at
		FROM attachment_blobs
		ORDER BY storage_key ASC`)
	if err != nil {
		return nil, fmt.Errorf("list attachment blobs: %w", err)
	}
	return scanAttachmentBlobs(rows)
}

func (r *attachmentRepo) Delete(ctx context.Context, storageKeys []string) error {
	if len(storageKeys) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attachment delete tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM attachment_blobs WHERE storage_key = ?`)
	if err != nil {
		return fmt.Errorf("prepare attachment delete: %w", err)
	}
	defer stmt.Close() //nolint:errcheck
	for _, key := range storageKeys {
		if _, err := stmt.ExecContext(ctx, key); err != nil {
			return fmt.Errorf("delete attachment blob %q: %w", key, err)
		}
	}
	return tx.Commit()
}

func (r *attachmentRepo) Reconcile(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attachment reconcile tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `
		SELECT content
		FROM message_parts
		WHERE type = 'file'
		ORDER BY created_at ASC`)
	if err != nil {
		return fmt.Errorf("list attachment file refs for reconcile: %w", err)
	}

	counts, err := aggregateAttachmentBlobs(rows)
	if err != nil {
		return err
	}
	now := time.Now().UTC().UnixNano()
	for key, blob := range counts {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO attachment_blobs (storage_key, mime_type, size, ref_count, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(storage_key) DO UPDATE SET
				mime_type = excluded.mime_type,
				size = excluded.size,
				ref_count = excluded.ref_count,
				updated_at = excluded.updated_at`,
			key, blob.MimeType, blob.Size, blob.RefCount, now,
		); err != nil {
			return fmt.Errorf("reconcile attachment blob %q: %w", key, err)
		}
	}

	existingRows, err := tx.QueryContext(ctx, `
		SELECT storage_key
		FROM attachment_blobs
		ORDER BY storage_key ASC`)
	if err != nil {
		return fmt.Errorf("list existing attachment blobs: %w", err)
	}
	defer existingRows.Close() //nolint:errcheck

	var stale []string
	for existingRows.Next() {
		var key string
		if err := existingRows.Scan(&key); err != nil {
			return fmt.Errorf("scan existing attachment blob: %w", err)
		}
		if _, ok := counts[key]; !ok {
			stale = append(stale, key)
		}
	}
	if err := existingRows.Err(); err != nil {
		return fmt.Errorf("iterate existing attachment blobs: %w", err)
	}

	if len(stale) > 0 {
		stmt, err := tx.PrepareContext(ctx, `DELETE FROM attachment_blobs WHERE storage_key = ?`)
		if err != nil {
			return fmt.Errorf("prepare stale attachment delete: %w", err)
		}
		defer stmt.Close() //nolint:errcheck
		for _, key := range stale {
			if _, err := stmt.ExecContext(ctx, key); err != nil {
				return fmt.Errorf("delete stale attachment blob %q: %w", key, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit attachment reconcile tx: %w", err)
	}
	return nil
}

func scanAttachmentBlobs(rows *sql.Rows) ([]repository.AttachmentBlob, error) {
	defer rows.Close() //nolint:errcheck
	var blobs []repository.AttachmentBlob
	for rows.Next() {
		var (
			blob    repository.AttachmentBlob
			updated int64
		)
		if err := rows.Scan(&blob.StorageKey, &blob.MimeType, &blob.Size, &blob.RefCount, &updated); err != nil {
			return nil, fmt.Errorf("scan attachment blob: %w", err)
		}
		blob.UpdatedAt = time.Unix(0, updated).UTC()
		blobs = append(blobs, blob)
	}
	return blobs, rows.Err()
}

func aggregateAttachmentBlobs(rows *sql.Rows) (map[string]repository.AttachmentBlob, error) {
	defer rows.Close() //nolint:errcheck
	counts := make(map[string]repository.AttachmentBlob)
	now := time.Now().UTC()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan attachment file ref: %w", err)
		}
		var fc message.FileContent
		if err := json.Unmarshal([]byte(raw), &fc); err != nil || fc.StorageKey == "" {
			continue
		}
		blob := counts[fc.StorageKey]
		blob.StorageKey = fc.StorageKey
		blob.MimeType = fc.MimeType
		blob.Size = fc.Size
		blob.RefCount++
		blob.UpdatedAt = now
		counts[fc.StorageKey] = blob
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachment file refs: %w", err)
	}
	return counts, nil
}
