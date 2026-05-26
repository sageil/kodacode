package events

import (
	"context"
	"database/sql"
	"os"
	"time"
)

type ArtifactPurgeResult struct {
	ToolResultBlobs int64
	BackgroundLogs  int64
}

func (s *SQLiteStore) PurgeArtifactsBefore(ctx context.Context, cutoff time.Time) (ArtifactPurgeResult, error) {
	if s == nil || s.db == nil {
		return ArtifactPurgeResult{}, os.ErrInvalid
	}
	if cutoff.IsZero() {
		return ArtifactPurgeResult{}, nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ArtifactPurgeResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var result ArtifactPurgeResult
	toolResult, err := tx.ExecContext(ctx, `
DELETE FROM tool_result_blobs
WHERE created_at < ?`,
		cutoff.UnixNano(),
	)
	if err != nil {
		return ArtifactPurgeResult{}, err
	}
	result.ToolResultBlobs, err = affectedRows(toolResult)
	if err != nil {
		return ArtifactPurgeResult{}, err
	}

	backgroundLogs, err := tx.ExecContext(ctx, `
DELETE FROM background_logs
WHERE updated_at < ?`,
		cutoff.UnixNano(),
	)
	if err != nil {
		return ArtifactPurgeResult{}, err
	}
	result.BackgroundLogs, err = affectedRows(backgroundLogs)
	if err != nil {
		return ArtifactPurgeResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return ArtifactPurgeResult{}, err
	}
	return result, nil
}

func affectedRows(result sql.Result) (int64, error) {
	if result == nil {
		return 0, nil
	}
	return result.RowsAffected()
}
