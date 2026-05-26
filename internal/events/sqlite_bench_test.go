package events

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func BenchmarkSQLiteStoreDeleteSessionWithArtifacts(b *testing.B) {
	root := b.TempDir()
	ctx := context.Background()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		path := filepath.Join(root, fmt.Sprintf("delete-%d.db", i))
		store, err := NewSQLiteStore(path)
		if err != nil {
			b.Fatalf("NewSQLiteStore() error = %v", err)
		}
		seedSQLiteBenchmarkSessionArtifacts(b, store, "session-1", 8, 4, 128*1024, 512*1024)
		b.StartTimer()
		if err := store.DeleteSession(ctx, "session-1"); err != nil {
			b.Fatalf("DeleteSession() error = %v", err)
		}
		b.StopTimer()
		_ = store.Close()
		removeSQLiteBenchmarkFiles(path)
	}
}

func BenchmarkSQLiteStorePurgeArtifactsBefore(b *testing.B) {
	root := b.TempDir()
	ctx := context.Background()
	cutoff := time.Now().Add(-7 * 24 * time.Hour)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		path := filepath.Join(root, fmt.Sprintf("purge-%d.db", i))
		store, err := NewSQLiteStore(path)
		if err != nil {
			b.Fatalf("NewSQLiteStore() error = %v", err)
		}
		oldBlobs, oldLogs := seedSQLiteBenchmarkExpiredArtifacts(b, store, "session-old", 8, 4, 128*1024, 512*1024, time.Now().Add(-14*24*time.Hour))
		seedSQLiteBenchmarkSessionArtifacts(b, store, "session-fresh", 8, 4, 128*1024, 512*1024)
		b.StartTimer()
		result, err := store.PurgeArtifactsBefore(ctx, cutoff)
		if err != nil {
			b.Fatalf("PurgeArtifactsBefore() error = %v", err)
		}
		if result.ToolResultBlobs != int64(oldBlobs) || result.BackgroundLogs != int64(oldLogs) {
			b.Fatalf("purged = %#v, want blobs=%d logs=%d", result, oldBlobs, oldLogs)
		}
		b.StopTimer()
		_ = store.Close()
		removeSQLiteBenchmarkFiles(path)
	}
}

func BenchmarkSQLiteArtifactStorageFootprint(b *testing.B) {
	for _, tc := range []struct {
		name      string
		blobCount int
		logCount  int
		blobBytes int
		logBytes  int
	}{
		{name: "moderate", blobCount: 4, logCount: 2, blobBytes: 256 * 1024, logBytes: 2 * 1024 * 1024},
		{name: "heavy", blobCount: 8, logCount: 4, blobBytes: 512 * 1024, logBytes: 4 * 1024 * 1024},
	} {
		b.Run(tc.name, func(b *testing.B) {
			root := b.TempDir()
			var totalDBBytes int64
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				path := filepath.Join(root, fmt.Sprintf("footprint-%d.db", i))
				store, err := NewSQLiteStore(path)
				if err != nil {
					b.Fatalf("NewSQLiteStore() error = %v", err)
				}
				seedSQLiteBenchmarkSessionArtifacts(b, store, fmt.Sprintf("session-%d", i), tc.blobCount, tc.logCount, tc.blobBytes, tc.logBytes)
				if err := store.Close(); err != nil {
					b.Fatalf("Close() error = %v", err)
				}
				totalDBBytes += sqliteBenchmarkFileBytes(path)
				removeSQLiteBenchmarkFiles(path)
			}
			b.ReportMetric(float64(totalDBBytes)/float64(max(b.N, 1)), "db_bytes")
		})
	}
}

func seedSQLiteBenchmarkSessionArtifacts(b *testing.B, store *SQLiteStore, sessionID string, blobCount, logCount, blobBytes, logBytes int) {
	b.Helper()
	ctx := context.Background()
	if _, err := store.Append(ctx, Draft{
		SessionID: sessionID,
		TurnID:    "_session",
		Type:      TypeSessionConfigured,
		Payload:   SessionConfiguredPayload{WorkspaceRoot: "/workspace"},
	}); err != nil {
		b.Fatalf("Append(session_configured) error = %v", err)
	}

	blobBody := strings.Repeat("b", blobBytes)
	for i := 0; i < blobCount; i++ {
		ref := fmt.Sprintf("%s/turn-1/call-%d-output.txt", sessionID, i)
		if _, err := store.SaveToolResultBlob(ctx, ref, sessionID, "turn-1", fmt.Sprintf("call-%d", i), "output", blobBody); err != nil {
			b.Fatalf("SaveToolResultBlob(%s) error = %v", ref, err)
		}
	}

	logChunk := []byte(strings.Repeat("l", min(logBytes, 256*1024)))
	for i := 0; i < logCount; i++ {
		ref := fmt.Sprintf("%s/turn-1/exec-%d.log", sessionID, i)
		if err := store.CreateBackgroundLog(ctx, ref, sessionID, "turn-1", fmt.Sprintf("exec-%d", i)); err != nil {
			b.Fatalf("CreateBackgroundLog(%s) error = %v", ref, err)
		}
		written := 0
		for written < logBytes {
			chunk := logChunk
			if remaining := logBytes - written; remaining < len(chunk) {
				chunk = chunk[:remaining]
			}
			if err := store.AppendBackgroundLogChunk(ctx, ref, chunk); err != nil {
				b.Fatalf("AppendBackgroundLogChunk(%s) error = %v", ref, err)
			}
			written += len(chunk)
		}
	}
}

func seedSQLiteBenchmarkExpiredArtifacts(b *testing.B, store *SQLiteStore, sessionID string, blobCount, logCount, blobBytes, logBytes int, at time.Time) (int, int) {
	b.Helper()
	seedSQLiteBenchmarkSessionArtifacts(b, store, sessionID, blobCount, logCount, blobBytes, logBytes)
	if _, err := store.db.Exec(`
UPDATE tool_result_blobs
SET created_at = ?
WHERE session_id = ?`, at.UnixNano(), sessionID); err != nil {
		b.Fatalf("UPDATE tool_result_blobs error = %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE background_logs
SET updated_at = ?
WHERE session_id = ?`, at.UnixNano(), sessionID); err != nil {
		b.Fatalf("UPDATE background_logs error = %v", err)
	}
	return blobCount, logCount
}

func sqliteBenchmarkFileBytes(path string) int64 {
	var total int64
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(name)
		if err == nil {
			total += info.Size()
		}
	}
	return total
}

func removeSQLiteBenchmarkFiles(path string) {
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Remove(name)
	}
}
