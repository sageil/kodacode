package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
)

func TestHybridSearchReusesPersistedIndexAcrossServices(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	writeSearchFile(t, filepath.Join(root, "auth.go"), "check permission before write\n")

	firstEmbedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"permission": {1, 0},
			"auth.go:1\ncheck permission before write": {1, 0},
		},
	}
	first := NewService(firstEmbedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	response, err := first.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("first Search() error = %v", err)
	}
	if len(response.Results) == 0 {
		t.Fatalf("first results = %#v", response.Results)
	}
	if firstEmbedder.CallCount() == 0 {
		t.Fatal("expected first embedder to be used")
	}

	secondEmbedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"permission": {1, 0},
		},
	}
	second := NewService(secondEmbedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	response, err = second.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("second Search() error = %v", err)
	}
	if len(response.Results) == 0 {
		t.Fatalf("second results = %#v", response.Results)
	}
	if secondEmbedder.CallCount() != 1 {
		t.Fatalf("second embedder calls = %d, want only query embedding", secondEmbedder.CallCount())
	}
}

func TestHybridSearchRefreshesPersistedIndexWhenFileChanges(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	path := filepath.Join(root, "auth.go")
	writeSearchFile(t, path, "check permission before write\n")

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"permission": {1, 0},
			"auth.go:1\ncheck permission before write": {1, 0},
			"auth.go:1\ncheck authorization token":     {1, 0},
		},
	}
	service := NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	if _, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	}); err != nil {
		t.Fatalf("initial Search() error = %v", err)
	}

	writeSearchFile(t, path, "check authorization token\n")
	service = NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	response, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("refreshed Search() error = %v", err)
	}
	if len(response.Results) == 0 || !strings.Contains(response.Results[0].Snippet, "authorization token") {
		t.Fatalf("results = %#v", response.Results)
	}
	if embedder.CallCount() < 4 {
		t.Fatalf("embedder calls = %d, want refresh to trigger file re-embed", embedder.CallCount())
	}
}

func TestHybridSearchLogsCacheRebuildCost(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "search-index")
	logDir := t.TempDir()
	path := filepath.Join(root, "auth.go")
	writeSearchFile(t, path, "check permission before write\n")

	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"permission": {1, 0},
			"auth.go:1\ncheck permission before write": {1, 0},
		},
	}
	service := NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, logger)
	if _, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("logger.Close() error = %v", err)
	}

	debugLog, err := os.ReadFile(filepath.Join(logDir, observability.DebugLogName))
	if err != nil {
		t.Fatalf("ReadFile(debug log) error = %v", err)
	}
	body := string(debugLog)
	if !strings.Contains(body, "search cache rebuilt") {
		t.Fatalf("debug log missing cache rebuild entry: %q", body)
	}
	if !strings.Contains(body, "file_bytes=") || !strings.Contains(body, "chunks=") || !strings.Contains(body, "embed_chars=") || !strings.Contains(body, "duration_ms=") {
		t.Fatalf("debug log missing cache rebuild cost fields: %q", body)
	}
	if !strings.Contains(body, path) {
		t.Fatalf("debug log missing rebuilt path %q: %q", path, body)
	}
}
