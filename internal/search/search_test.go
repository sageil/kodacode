package search

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

type fakeEmbedder struct {
	mu      sync.Mutex
	vectors map[string][]float32
	calls   int
	inputs  []string
}

func (f *fakeEmbedder) Embed(_ context.Context, req provider.EmbeddingRequest) ([][]float32, error) {
	f.mu.Lock()
	f.calls++
	f.inputs = append(f.inputs, req.Inputs...)
	f.mu.Unlock()
	out := make([][]float32, len(req.Inputs))
	for idx, input := range req.Inputs {
		vector := f.vectors[input]
		if vector == nil {
			vector = []float32{0, 0}
		}
		out[idx] = append([]float32(nil), vector...)
	}
	return out, nil
}

func (f *fakeEmbedder) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeEmbedder) Inputs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.inputs...)
}

func TestLexicalSearchFindsLiteralMatches(t *testing.T) {
	root := t.TempDir()
	writeSearchFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {\n\tprintln(\"Hello\")\n}\n")

	results, err := Lexical(Request{
		Query:         "hello",
		RootPath:      root,
		WorkspaceRoot: root,
		Glob:          "*.go",
		CaseSensitive: false,
		MaxResults:    10,
		Mode:          ModeLexical,
	})
	if err != nil {
		t.Fatalf("Lexical() error = %v", err)
	}
	if len(results) != 1 || results[0].Path != "main.go" || results[0].Line != 4 {
		t.Fatalf("results = %#v", results)
	}
}

func TestLexicalSearchFindsRegexMatches(t *testing.T) {
	root := t.TempDir()
	writeSearchFile(t, filepath.Join(root, "tasks.txt"), "TODO-123 fix search\nTODO-456 add locate\n")

	results, err := Lexical(Request{
		Query:         `TODO-\d{3}`,
		RootPath:      root,
		WorkspaceRoot: root,
		Regex:         true,
		CaseSensitive: true,
		MaxResults:    10,
		Mode:          ModeLexical,
	})
	if err != nil {
		t.Fatalf("Lexical() error = %v", err)
	}
	if len(results) != 2 || results[0].Path != "tasks.txt" || results[0].Line != 1 || results[1].Line != 2 {
		t.Fatalf("results = %#v", results)
	}
}

func TestHybridSearchFallsBackWithoutEmbedder(t *testing.T) {
	root := t.TempDir()
	writeSearchFile(t, filepath.Join(root, "notes.txt"), "permission check\n")

	service := NewService(nil, provider.ModelRef{}, 0, t.TempDir(), nil)
	response, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !response.Fallback || !strings.Contains(response.Notice, "not configured") {
		t.Fatalf("response = %#v", response)
	}
	if len(response.Results) != 1 || response.Results[0].Source != SourceLexical {
		t.Fatalf("results = %#v", response.Results)
	}
}

func TestHybridSearchMergesLexicalAndSemanticResults(t *testing.T) {
	root := t.TempDir()
	writeSearchFile(t, filepath.Join(root, "auth.go"), "check permission before write\n")
	writeSearchFile(t, filepath.Join(root, "notes.txt"), "authorization guard\n")

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"permission": {1, 0},
			"auth.go:1\ncheck permission before write": {1, 0},
			"notes.txt:1\nauthorization guard":         {1, 0},
		},
	}
	service := NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, t.TempDir(), nil)

	response, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(response.Results) != 2 {
		t.Fatalf("results = %#v", response.Results)
	}
	if response.Results[0].Source != SourceMerged {
		t.Fatalf("first result source = %q, want merged", response.Results[0].Source)
	}
}

func TestHybridSearchMergesChunkIdentityAndPrefersLexicalDisplayLine(t *testing.T) {
	root := t.TempDir()
	content := `func CheckPermission(user string) bool {
	allowed := user != ""
	return hasPermission(user)
}
`
	path := filepath.Join(root, "auth.go")
	writeSearchFile(t, path, content)
	inputs := buildChunkInputs(root, path, content)
	if len(inputs) != 1 {
		t.Fatalf("len(inputs) = %d, want 1", len(inputs))
	}

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"hasPermission": {1, 0},
			inputs[0].Text:  {1, 0},
		},
	}
	service := NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, t.TempDir(), nil)

	response, err := service.Search(context.Background(), Request{
		Query:         "hasPermission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    5,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want single merged chunk result", response.Results)
	}
	if response.Results[0].Source != SourceMerged {
		t.Fatalf("result source = %q, want merged", response.Results[0].Source)
	}
	if response.Results[0].Line != 3 {
		t.Fatalf("result line = %d, want lexical hit line 3", response.Results[0].Line)
	}
	if !strings.Contains(response.Results[0].Snippet, "hasPermission") {
		t.Fatalf("result snippet = %q, want lexical hit snippet", response.Results[0].Snippet)
	}
}

func TestHybridSearchFallsBackForBroadScope(t *testing.T) {
	root := t.TempDir()
	for idx := range semanticChunkLimit + 1 {
		writeSearchFile(t, filepath.Join(root, fmt.Sprintf("file-%03d.txt", idx)), "line\n")
	}
	service := NewService(&fakeEmbedder{}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, t.TempDir(), nil)

	response, err := service.Search(context.Background(), Request{
		Query:         "line",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    3,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !response.Fallback || !strings.Contains(response.Notice, ErrSemanticScopeTooLarge.Error()) {
		t.Fatalf("response = %#v", response)
	}
}

func TestHybridSearchAutoWarmsTrackedBroadScopeAfterFallback(t *testing.T) {
	root := t.TempDir()
	for idx := range semanticChunkLimit + 1 {
		writeSearchFile(t, filepath.Join(root, fmt.Sprintf("file-%03d.txt", idx)), "line\n")
	}
	indexDir := t.TempDir()
	service := NewService(&fakeEmbedder{
		vectors: map[string][]float32{
			"line": {1, 0},
		},
	}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	service.TrackWorkspace(root, TrackOptions{RefreshInterval: time.Hour})
	t.Cleanup(func() {
		_ = service.Close()
	})

	response, err := service.Search(context.Background(), Request{
		Query:         "line",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    3,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !response.Fallback {
		t.Fatalf("response = %#v, want initial lexical fallback", response)
	}
	if !strings.Contains(response.Notice, "warming workspace index in background") {
		t.Fatalf("notice = %q, want auto-warm notice", response.Notice)
	}

	waitForSearchCondition(t, func() bool {
		status := service.WorkspaceStatus(root)
		return status.IndexedFiles == semanticChunkLimit+1 && status.PendingFiles == 0
	})

	response, err = service.Search(context.Background(), Request{
		Query:         "line",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    3,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("Search() after auto-warm error = %v", err)
	}
	if response.Fallback {
		t.Fatalf("response = %#v, want hybrid results after auto-warm", response)
	}
}

func TestHybridSearchUsesPrewarmedCacheForBroadScope(t *testing.T) {
	root := t.TempDir()
	for idx := range semanticChunkLimit + 1 {
		writeSearchFile(t, filepath.Join(root, fmt.Sprintf("file-%03d.txt", idx)), "line\n")
	}
	indexDir := t.TempDir()

	prewarm := NewService(&fakeEmbedder{
		vectors: map[string][]float32{
			"line": {1, 0},
		},
	}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	paths, err := semanticPaths(Request{RootPath: root, WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("semanticPaths() error = %v", err)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		if _, _, err := prewarm.ensurePersistedFileCache(context.Background(), root, path, info); err != nil {
			t.Fatalf("ensurePersistedFileCache(%q) error = %v", path, err)
		}
	}

	queryEmbedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"line": {1, 0},
		},
	}
	service := NewService(queryEmbedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)

	response, err := service.Search(context.Background(), Request{
		Query:         "line",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    3,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if response.Fallback {
		t.Fatalf("response = %#v, want semantic search from prewarmed cache", response)
	}
	if len(response.Results) != 3 {
		t.Fatalf("results = %#v", response.Results)
	}
	if queryEmbedder.CallCount() != 1 {
		t.Fatalf("query embedder calls = %d, want 1 query embedding call", queryEmbedder.CallCount())
	}
}

func TestHybridSearchUsesPrewarmedCacheForVeryLargeScope(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("line\n", 1000)
	for idx := 0; idx < 161; idx++ {
		writeSearchFile(t, filepath.Join(root, fmt.Sprintf("file-%03d.txt", idx)), content)
	}
	indexDir := t.TempDir()

	prewarm := NewService(&fakeEmbedder{
		vectors: map[string][]float32{
			"line": {1, 0},
		},
	}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)
	paths, err := semanticPaths(Request{RootPath: root, WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("semanticPaths() error = %v", err)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		if _, _, err := prewarm.ensurePersistedFileCache(context.Background(), root, path, info); err != nil {
			t.Fatalf("ensurePersistedFileCache(%q) error = %v", path, err)
		}
	}

	queryEmbedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"line": {1, 0},
		},
	}
	service := NewService(queryEmbedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, indexDir, nil)

	response, err := service.Search(context.Background(), Request{
		Query:         "line",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    3,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if response.Fallback {
		t.Fatalf("response = %#v, want semantic search from very large prewarmed cache", response)
	}
	if len(response.Results) != 3 {
		t.Fatalf("results = %#v", response.Results)
	}
	if queryEmbedder.CallCount() != 1 {
		t.Fatalf("query embedder calls = %d, want 1 query embedding call", queryEmbedder.CallCount())
	}
}

func TestRequestValidateRejectsInvalidMode(t *testing.T) {
	err := (Request{Query: "x", RootPath: ".", Mode: "bad"}).Validate()
	if !errors.Is(err, ErrModeInvalid) {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRequestValidateRejectsRegexHybridMode(t *testing.T) {
	err := (Request{Query: "x", RootPath: ".", Mode: ModeHybrid, Regex: true}).Validate()
	if !errors.Is(err, ErrRegexUnsupportedInHybrid) {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestReciprocalRankFusionScoreUsesStandardK(t *testing.T) {
	if got, want := reciprocalRankFusionScore(standardRRFK, 1), 1.0/61.0; got != want {
		t.Fatalf("reciprocalRankFusionScore(1) = %v, want %v", got, want)
	}
	if reciprocalRankFusionScore(standardRRFK, 10, 10) <= reciprocalRankFusionScore(standardRRFK, 1) {
		t.Fatal("combined lower-ranked evidence should outrank one top-ranked source under standard RRF")
	}
}

func writeSearchFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
