package search

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

func TestWarmWorkspaceBuildsPersistedIndexAndReportsStatus(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	writeSearchFile(t, filepath.Join(root, "auth.go"), "package auth\n\nfunc CheckPermission() bool { return true }\n")
	writeSearchFile(t, filepath.Join(root, "billing.go"), "package billing\n\nfunc Charge() error { return nil }\n")

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"auth.go:1\npackage auth":                  {1, 0},
			"auth.go:3\nfunc CheckPermission() bool {": {1, 0},
			"auth.go:1\npackage auth\n":                {1, 0},
			"billing.go:1\npackage billing":            {1, 0},
			"billing.go:3\nfunc Charge() error {":      {1, 0},
			"billing.go:1\npackage billing\n":          {1, 0},
		},
	}
	service := NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	service.TrackWorkspace(root, TrackOptions{RefreshInterval: 10 * time.Millisecond})
	t.Cleanup(func() {
		_ = service.Close()
	})

	result, err := service.WarmWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("WarmWorkspace() error = %v", err)
	}
	if result.Files != 2 {
		t.Fatalf("WarmWorkspace().Files = %d, want 2", result.Files)
	}
	if result.IndexedFiles != 2 {
		t.Fatalf("WarmWorkspace().IndexedFiles = %d, want 2", result.IndexedFiles)
	}
	if result.IndexedChunks == 0 {
		t.Fatal("WarmWorkspace().IndexedChunks = 0, want indexed chunks")
	}
	if result.BuiltFiles == 0 {
		t.Fatal("WarmWorkspace().BuiltFiles = 0, want built persisted files")
	}

	status := service.WorkspaceStatus(root)
	if !status.Configured {
		t.Fatal("WorkspaceStatus().Configured = false, want true")
	}
	if !status.Tracking {
		t.Fatal("WorkspaceStatus().Tracking = false, want true")
	}
	if status.TrackedFiles != 2 {
		t.Fatalf("WorkspaceStatus().TrackedFiles = %d, want 2", status.TrackedFiles)
	}
	if status.IndexedFiles != 2 {
		t.Fatalf("WorkspaceStatus().IndexedFiles = %d, want 2", status.IndexedFiles)
	}
	if status.PendingFiles != 0 {
		t.Fatalf("WorkspaceStatus().PendingFiles = %d, want 0", status.PendingFiles)
	}
	if status.IndexedChunks == 0 {
		t.Fatal("WorkspaceStatus().IndexedChunks = 0, want indexed chunks")
	}
	if status.LastWarmupAt.IsZero() {
		t.Fatal("WorkspaceStatus().LastWarmupAt = zero, want warmup timestamp")
	}
}

func TestWarmWorkspaceReturnsUnavailableWithoutConfiguredEmbeddings(t *testing.T) {
	service := NewService(nil, provider.ModelRef{}, 0, t.TempDir(), nil)
	_, err := service.WarmWorkspace(context.Background(), t.TempDir())
	if !errors.Is(err, ErrWarmupUnavailable) {
		t.Fatalf("WarmWorkspace() error = %v, want ErrWarmupUnavailable", err)
	}
}
