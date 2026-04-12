package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestNewRuntimeAppliesDefaults(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{})
	if rt.debounce != 750*time.Millisecond {
		t.Fatalf("debounce = %s, want %s", rt.debounce, 750*time.Millisecond)
	}
	if rt.fullRescanEvery != 0 {
		t.Fatalf("fullRescanEvery = %s, want disabled by default", rt.fullRescanEvery)
	}
}

func TestRuntimeShouldIgnorePath(t *testing.T) {
	projectDir := filepath.Join(string(filepath.Separator), "proj")
	rt := NewRuntime(RuntimeConfig{
		ProjectDir:     projectDir,
		IgnorePatterns: []string{"tmp/**", "**/*.gen.go"},
	})

	tests := []struct {
		path string
		want bool
	}{
		{path: filepath.Join(projectDir, "tmp", "note.txt"), want: true},
		{path: filepath.Join(projectDir, "pkg", "model.gen.go"), want: true},
		{path: filepath.Join(projectDir, "pkg", "model.go"), want: false},
	}

	for _, tt := range tests {
		if got := rt.shouldIgnorePath(tt.path); got != tt.want {
			t.Fatalf("shouldIgnorePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestRuntimeShouldWatchDirSkipsKnownAndIgnoredDirectories(t *testing.T) {
	projectDir := filepath.Join(string(filepath.Separator), "proj")
	rt := NewRuntime(RuntimeConfig{
		ProjectDir:     projectDir,
		IgnorePatterns: []string{"generated/**"},
	})

	tests := []struct {
		path string
		want bool
	}{
		{path: filepath.Join(projectDir, ".git"), want: false},
		{path: filepath.Join(projectDir, "node_modules"), want: false},
		{path: filepath.Join(projectDir, "vendor"), want: false},
		{path: filepath.Join(projectDir, "generated"), want: false},
		{path: filepath.Join(projectDir, "pkg"), want: true},
	}

	for _, tt := range tests {
		if got := rt.shouldWatchDir(tt.path); got != tt.want {
			t.Fatalf("shouldWatchDir(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestRuntimeShouldForceFullSync(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{})
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Errorf("watcher.Close() error = %v", err)
		}
	}()

	if got := rt.shouldForceFullSync(watcher, fsnotify.Event{Op: fsnotify.Write}, false); got {
		t.Fatal("write event for a file should stay path-scoped")
	}
	if got := rt.shouldForceFullSync(watcher, fsnotify.Event{Op: fsnotify.Remove, Name: "/proj/file.go"}, false); got {
		t.Fatal("file remove event should stay path-scoped")
	}
	if got := rt.shouldForceFullSync(watcher, fsnotify.Event{Op: fsnotify.Create}, true); !got {
		t.Fatal("directory creation should trigger a full rescan")
	}

	dir := t.TempDir()
	if err := watcher.Add(dir); err != nil {
		t.Fatalf("watcher.Add() error = %v", err)
	}
	if got := rt.shouldForceFullSync(watcher, fsnotify.Event{Op: fsnotify.Rename, Name: dir}, false); !got {
		t.Fatal("watched directory rename should trigger a full rescan")
	}
}

func TestRuntimeSyncPathsScopesEmbeddingBackfillToChangedFiles(t *testing.T) {
	ctx := context.Background()
	db, projectDir := setupIndexerTest(t)
	indexer := NewIndexer(db, projectDir, IndexerConfig{})
	if _, err := indexer.Index(ctx); err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	embedder := &mockEmbedder{dims: 4}
	embeddingIndexer := NewEmbeddingIndexer(EmbeddingIndexerConfig{
		DB:         db,
		Embedder:   embedder,
		Model:      "test-model",
		BatchSize:  100,
		ProjectDir: projectDir,
	})
	rt := NewRuntime(RuntimeConfig{
		ProjectDir:       projectDir,
		Indexer:          indexer,
		EmbeddingIndexer: embeddingIndexer,
	})

	authPath := filepath.Join(projectDir, "auth.go")
	updated := `package example

// Authorize verifies the user has the given permission.
func Authorize(ctx context.Context, perm string) error {
	return nil
}
`
	if err := os.WriteFile(authPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	rt.syncPaths(ctx, map[string]struct{}{authPath: {}})

	if got := embedder.calls.Load(); got != 1 {
		t.Fatalf("Embed() calls = %d, want 1 path-scoped batch", got)
	}
	wantChangedFileEmbeddings := countSymbolsForFile(t, db, authPath)
	count, err := embeddingIndexer.EmbeddingCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != wantChangedFileEmbeddings {
		t.Fatalf("EmbeddingCount() = %d, want %d changed-file embeddings", count, wantChangedFileEmbeddings)
	}

	sessionPath := filepath.Join(projectDir, "session.go")
	if got := embeddingCountForFile(t, db, "test-model", sessionPath); got != 0 {
		t.Fatalf("session.go embeddings = %d, want 0 for untouched file", got)
	}
}
