package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSQLiteStoreAppendAssignsSequenceAndReplayReadsFromCursor(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx := context.Background()

	first, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeSessionConfigured,
		Payload:   SessionConfiguredPayload{WorkspaceRoot: "/repo"},
	})
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	second, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantCommit,
		Payload:   AssistantCommitPayload{Content: "done"},
	})
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if first.Sequence != 0 || second.Sequence != 1 {
		t.Fatalf("sequences = %d, %d; want 0, 1", first.Sequence, second.Sequence)
	}

	got, err := store.Replay(ctx, Query{SessionID: "session-1", AfterSequence: 0})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(got) != 1 || got[0].Sequence != 1 || got[0].Type != TypeAssistantCommit {
		t.Fatalf("replay = %#v", got)
	}
}

func TestSQLiteStoreSavesBranchSummaryAndDeletesWithSession(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx := context.Background()
	if _, err := store.Append(ctx, Draft{
		SessionID: "session-branch",
		TurnID:    "_session",
		Type:      TypeSessionConfigured,
		Payload:   SessionConfiguredPayload{WorkspaceRoot: "/repo"},
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	artifact := BranchSummaryArtifact{
		SessionID:        "session-branch",
		SourceSequence:   3,
		Summary:          "branch changed cache flow",
		Model:            "openai/gpt-5-mini",
		PromptTokens:     123,
		CompletionTokens: 45,
		CreatedAt:        time.Unix(1710000000, 0).UTC(),
		UpdatedAt:        time.Unix(1710000100, 0).UTC(),
	}
	if err := store.SaveBranchSummary(ctx, artifact); err != nil {
		t.Fatalf("SaveBranchSummary() error = %v", err)
	}
	got, ok, err := store.LoadBranchSummary(ctx, "session-branch")
	if err != nil {
		t.Fatalf("LoadBranchSummary() error = %v", err)
	}
	if !ok || got.Summary != artifact.Summary || got.SourceSequence != artifact.SourceSequence || got.Model != artifact.Model {
		t.Fatalf("branch summary = %#v, ok=%v", got, ok)
	}
}

func TestSQLiteStoreWatchReplaysThenStreamsWithoutGap(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx := context.Background()

	first, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "_session",
		Type:      TypeSessionConfigured,
		Payload:   SessionConfiguredPayload{WorkspaceRoot: "/repo"},
	})
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := store.Watch(watchCtx, Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	second, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantCommit,
		Payload:   AssistantCommitPayload{Content: "done"},
	})
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}

	gotFirst := mustReceiveEvent(t, stream)
	gotSecond := mustReceiveEvent(t, stream)
	if gotFirst.Sequence != first.Sequence || gotSecond.Sequence != second.Sequence {
		t.Fatalf("watch sequence order = %d, %d; want %d, %d", gotFirst.Sequence, gotSecond.Sequence, first.Sequence, second.Sequence)
	}
}

func TestSQLiteStoreLatestWorkspaceSessionIDReturnsMostRecentlyUpdated(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx := context.Background()

	if _, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "_session",
		Type:      TypeSessionConfigured,
		Payload:   SessionConfiguredPayload{WorkspaceRoot: "/repo"},
	}); err != nil {
		t.Fatalf("Append(session-1) error = %v", err)
	}
	time.Sleep(time.Millisecond)
	if _, err := store.Append(ctx, Draft{
		SessionID: "session-2",
		TurnID:    "_session",
		Type:      TypeSessionConfigured,
		Payload:   SessionConfiguredPayload{WorkspaceRoot: "/repo"},
	}); err != nil {
		t.Fatalf("Append(session-2) error = %v", err)
	}
	if _, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantCommit,
		Payload:   AssistantCommitPayload{Content: "latest"},
	}); err != nil {
		t.Fatalf("Append(update session-1) error = %v", err)
	}

	sessionID, ok, err := store.LatestWorkspaceSessionID(ctx, "/repo")
	if err != nil {
		t.Fatalf("LatestWorkspaceSessionID() error = %v", err)
	}
	if !ok || sessionID != "session-1" {
		t.Fatalf("LatestWorkspaceSessionID() = %q, %t; want session-1, true", sessionID, ok)
	}
}

func TestSQLiteStoreUsesDedicatedSessionIndexTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kodacode.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := db.Exec(`
CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    updated_at INTEGER NOT NULL
)`); err != nil {
		_ = db.Close()
		t.Fatalf("Exec() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx := context.Background()

	if _, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "_session",
		Type:      TypeSessionConfigured,
		Payload:   SessionConfiguredPayload{WorkspaceRoot: "/repo"},
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	sessionID, ok, err := store.LatestWorkspaceSessionID(ctx, "/repo")
	if err != nil {
		t.Fatalf("LatestWorkspaceSessionID() error = %v", err)
	}
	if !ok || sessionID != "session-1" {
		t.Fatalf("LatestWorkspaceSessionID() = %q, %t; want session-1, true", sessionID, ok)
	}
}

func TestSQLiteStorePurgeArtifactsBeforeRemovesExpiredRowsOnly(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx := context.Background()

	if _, err := store.SaveToolResultBlob(ctx, "session-1/turn-1/call-1-output.txt", "session-1", "turn-1", "call-1", "output", "old"); err != nil {
		t.Fatalf("SaveToolResultBlob(old) error = %v", err)
	}
	if _, err := store.SaveToolResultBlob(ctx, "session-1/turn-1/call-2-output.txt", "session-1", "turn-1", "call-2", "output", "fresh"); err != nil {
		t.Fatalf("SaveToolResultBlob(fresh) error = %v", err)
	}
	if err := store.CreateBackgroundLog(ctx, "session-1/turn-1/exec-old.log", "session-1", "turn-1", "exec-old"); err != nil {
		t.Fatalf("CreateBackgroundLog(old) error = %v", err)
	}
	if err := store.CreateBackgroundLog(ctx, "session-1/turn-1/exec-fresh.log", "session-1", "turn-1", "exec-fresh"); err != nil {
		t.Fatalf("CreateBackgroundLog(fresh) error = %v", err)
	}
	if err := store.AppendBackgroundLogChunk(ctx, "session-1/turn-1/exec-old.log", []byte("old")); err != nil {
		t.Fatalf("AppendBackgroundLogChunk(old) error = %v", err)
	}
	if err := store.AppendBackgroundLogChunk(ctx, "session-1/turn-1/exec-fresh.log", []byte("fresh")); err != nil {
		t.Fatalf("AppendBackgroundLogChunk(fresh) error = %v", err)
	}

	oldTime := time.Now().Add(-10 * 24 * time.Hour).UnixNano()
	if _, err := store.db.Exec(`
UPDATE tool_result_blobs
SET created_at = ?
WHERE ref = ?`, oldTime, "session-1/turn-1/call-1-output.txt"); err != nil {
		t.Fatalf("UPDATE tool_result_blobs error = %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE background_logs
SET updated_at = ?
WHERE ref = ?`, oldTime, "session-1/turn-1/exec-old.log"); err != nil {
		t.Fatalf("UPDATE background_logs error = %v", err)
	}

	result, err := store.PurgeArtifactsBefore(ctx, time.Now().Add(-7*24*time.Hour))
	if err != nil {
		t.Fatalf("PurgeArtifactsBefore() error = %v", err)
	}
	if result.ToolResultBlobs != 1 {
		t.Fatalf("ToolResultBlobs purged = %d, want 1", result.ToolResultBlobs)
	}
	if result.BackgroundLogs != 1 {
		t.Fatalf("BackgroundLogs purged = %d, want 1", result.BackgroundLogs)
	}

	if _, err := store.LoadToolResultBlob(ctx, "session-1/turn-1/call-1-output.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadToolResultBlob(old) error = %v, want os.ErrNotExist", err)
	}
	if text, err := store.LoadToolResultBlob(ctx, "session-1/turn-1/call-2-output.txt"); err != nil || text != "fresh" {
		t.Fatalf("LoadToolResultBlob(fresh) = %q, %v", text, err)
	}
	if _, _, err := store.ReadBackgroundLogFrom(ctx, "session-1/turn-1/exec-old.log", 0, 1024); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadBackgroundLogFrom(old) error = %v, want os.ErrNotExist", err)
	}
	if chunk, _, err := store.ReadBackgroundLogFrom(ctx, "session-1/turn-1/exec-fresh.log", 0, 1024); err != nil || chunk != "fresh" {
		t.Fatalf("ReadBackgroundLogFrom(fresh) = %q, %v", chunk, err)
	}
}

func TestSQLiteStoreAppendSerializesConcurrentArtifactWrites(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx := context.Background()

	if _, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "_session",
		Type:      TypeSessionConfigured,
		Payload:   SessionConfiguredPayload{WorkspaceRoot: "/repo"},
	}); err != nil {
		t.Fatalf("Append(session_configured) error = %v", err)
	}

	const workers = 4
	for i := 0; i < workers; i++ {
		ref := fmt.Sprintf("session-1/turn-1/exec-%d.log", i)
		if err := store.CreateBackgroundLog(ctx, ref, "session-1", "turn-1", fmt.Sprintf("exec-%d", i)); err != nil {
			t.Fatalf("CreateBackgroundLog(%d) error = %v", i, err)
		}
	}

	errCh := make(chan error, workers)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		ref := fmt.Sprintf("session-1/turn-1/exec-%d.log", i)
		wg.Add(1)
		go func(logRef string) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if err := store.AppendBackgroundLogChunk(ctx, logRef, []byte("background chunk\n")); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
			}
		}(ref)
	}

	for i := 0; i < 200; i++ {
		if _, err := store.Append(ctx, Draft{
			SessionID: "session-1",
			TurnID:    fmt.Sprintf("turn-%d", i),
			Type:      TypeAssistantCommit,
			Payload:   AssistantCommitPayload{Content: "done"},
		}); err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("Append(%d) error = %v", i, err)
		}
	}

	close(stop)
	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatalf("concurrent artifact write error = %v", err)
	default:
	}
}
