package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

func TestTrackWorkspaceInvalidatesChangedPersistedCache(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	path := filepath.Join(root, "auth.go")
	writeSearchFile(t, path, "package auth\n\nfunc CheckPermission() bool { return true }\n")

	service := NewService(&fakeEmbedder{
		vectors: map[string][]float32{},
	}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	service.TrackWorkspace(root, TrackOptions{RefreshInterval: 10 * time.Millisecond})
	t.Cleanup(func() {
		_ = service.Close()
	})

	if _, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	cachePath, ok := service.cachePath(root, path)
	if !ok {
		t.Fatal("cachePath() = false, want true")
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("Stat(cachePath) error = %v", err)
	}

	writeSearchFile(t, path, "package auth\n\nfunc CheckPermission() bool { return false }\n")
	waitForSearchCondition(t, func() bool {
		_, err := os.Stat(cachePath)
		return os.IsNotExist(err)
	})
}

func TestRefreshWorkspaceInvalidatesChangedPersistedCache(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	path := filepath.Join(root, "auth.go")
	writeSearchFile(t, path, "package auth\n\nfunc CheckPermission() bool { return true }\n")

	service := NewService(&fakeEmbedder{
		vectors: map[string][]float32{},
	}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	service.TrackWorkspace(root, TrackOptions{RefreshInterval: time.Hour})
	t.Cleanup(func() {
		_ = service.Close()
	})

	if _, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	cachePath, ok := service.cachePath(root, path)
	if !ok {
		t.Fatal("cachePath() = false, want true")
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("Stat(cachePath) error = %v", err)
	}

	writeSearchFile(t, path, "package auth\n\nfunc CheckPermission() bool { return false }\n")
	if ok := service.RefreshWorkspace(context.Background(), root); !ok {
		t.Fatal("RefreshWorkspace() = false, want true")
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("Stat(cachePath) error = %v, want not exists", err)
	}
}

func TestTrackWorkspaceInvalidatesDeletedPersistedCache(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	path := filepath.Join(root, "auth.go")
	writeSearchFile(t, path, "package auth\n\nfunc CheckPermission() bool { return true }\n")

	service := NewService(&fakeEmbedder{
		vectors: map[string][]float32{},
	}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	service.TrackWorkspace(root, TrackOptions{RefreshInterval: 10 * time.Millisecond})
	t.Cleanup(func() {
		_ = service.Close()
	})

	if _, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	cachePath, ok := service.cachePath(root, path)
	if !ok {
		t.Fatal("cachePath() = false, want true")
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove(%q) error = %v", path, err)
	}

	waitForSearchCondition(t, func() bool {
		_, err := os.Stat(cachePath)
		return os.IsNotExist(err)
	})
}

func TestTrackWorkspaceInvalidatesChangedCacheWithoutBackgroundReembed(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	path := filepath.Join(root, "auth.go")
	writeSearchFile(t, path, "package auth\n\nfunc CheckPermission() bool { return true }\n")

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"permission":                               {1, 0},
			"auth.go:1\npackage auth":                  {1, 0},
			"auth.go:3\nfunc CheckPermission() bool {": {1, 0},
			"auth.go:1\npackage auth\n":                {1, 0},
		},
	}
	service := NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	service.TrackWorkspace(root, TrackOptions{RefreshInterval: 10 * time.Millisecond})
	t.Cleanup(func() {
		_ = service.Close()
	})

	if _, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if embedder.CallCount() != 2 {
		t.Fatalf("initial embedder calls = %d, want 2", embedder.CallCount())
	}

	cachePath, ok := service.cachePath(root, path)
	if !ok {
		t.Fatal("cachePath() = false, want true")
	}
	writeSearchFile(t, path, "package auth\n\nfunc CheckPermission() bool { return false }\n")

	waitForSearchCondition(t, func() bool {
		_, err := os.Stat(cachePath)
		return os.IsNotExist(err)
	})
	if embedder.CallCount() != 2 {
		t.Fatalf("background invalidation should not re-embed, calls = %d", embedder.CallCount())
	}

	if _, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	}); err != nil {
		t.Fatalf("Search() after invalidation error = %v", err)
	}
	if embedder.CallCount() != 4 {
		t.Fatalf("second search should rebuild file and query embeddings, calls = %d", embedder.CallCount())
	}
}

func TestTrackWorkspacePrewarmsEmbeddingsWhenEnabled(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	path := filepath.Join(root, "auth.go")
	writeSearchFile(t, path, "package auth\n\nfunc CheckPermission() bool { return true }\n")

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"permission":                               {1, 0},
			"auth.go:1\npackage auth":                  {1, 0},
			"auth.go:3\nfunc CheckPermission() bool {": {1, 0},
			"auth.go:1\npackage auth\n":                {1, 0},
		},
	}
	service := NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	service.TrackWorkspace(root, TrackOptions{RefreshInterval: 10 * time.Millisecond, PrewarmEmbeddings: true})
	t.Cleanup(func() {
		_ = service.Close()
	})

	cachePath, ok := service.cachePath(root, path)
	if !ok {
		t.Fatal("cachePath() = false, want true")
	}
	waitForSearchCondition(t, func() bool {
		_, err := os.Stat(cachePath)
		return err == nil
	})
	if embedder.CallCount() == 0 {
		t.Fatal("expected background prewarm to embed file chunks")
	}
	if searchMemoryCacheHasPath(service, path) {
		t.Fatal("expected background prewarm to leave RAM cache empty")
	}

	initialCalls := embedder.CallCount()
	if _, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if embedder.CallCount() != initialCalls+1 {
		t.Fatalf("search after prewarm should only embed the query, calls = %d", embedder.CallCount())
	}
	if !searchMemoryCacheHasPath(service, path) {
		t.Fatal("expected semantic search to hydrate RAM cache from persisted prewarm data")
	}
}

func TestTrackWorkspacePrewarmsChangedAndAddedFilesWhenEnabled(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	firstPath := filepath.Join(root, "auth.go")
	secondPath := filepath.Join(root, "billing.go")
	writeSearchFile(t, firstPath, "package auth\n\nfunc CheckPermission() bool { return true }\n")

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
	service.TrackWorkspace(root, TrackOptions{RefreshInterval: 10 * time.Millisecond, PrewarmEmbeddings: true})
	t.Cleanup(func() {
		_ = service.Close()
	})

	firstCachePath, ok := service.cachePath(root, firstPath)
	if !ok {
		t.Fatal("cachePath(first) = false, want true")
	}
	secondCachePath, ok := service.cachePath(root, secondPath)
	if !ok {
		t.Fatal("cachePath(second) = false, want true")
	}
	waitForSearchCondition(t, func() bool {
		_, err := os.Stat(firstCachePath)
		return err == nil
	})

	initialCalls := embedder.CallCount()
	writeSearchFile(t, firstPath, "package auth\n\nfunc CheckPermission() bool { return false }\n")
	writeSearchFile(t, secondPath, "package billing\n\nfunc Charge() error { return nil }\n")

	waitForSearchCondition(t, func() bool {
		_, firstErr := os.Stat(firstCachePath)
		_, secondErr := os.Stat(secondCachePath)
		return firstErr == nil && secondErr == nil && embedder.CallCount() >= initialCalls+2
	})
}

func waitForSearchCondition(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func searchMemoryCacheHasPath(service *Service, path string) bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	_, ok := service.files[path]
	return ok
}
