package search

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestLexicalObservedSkipsDefaultDirectories(t *testing.T) {
	root := t.TempDir()
	writeSearchFile(t, filepath.Join(root, "app", "main.txt"), "permission check\n")
	writeSearchFile(t, filepath.Join(root, "vendor", "dep.txt"), "permission check\n")

	response, err := LexicalObserved(Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    10,
		Mode:          ModeLexical,
	})
	if err != nil {
		t.Fatalf("LexicalObserved() error = %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want one non-skipped file", response.Results)
	}
	if response.Results[0].Path != "app/main.txt" {
		t.Fatalf("result path = %q, want app/main.txt", response.Results[0].Path)
	}
}

func TestConfiguredSkipDirsApplyAcrossSearchAndTracking(t *testing.T) {
	root := t.TempDir()
	keptPath := filepath.Join(root, "app", "main.txt")
	extraSkippedPath := filepath.Join(root, "coverage", "report.txt")
	defaultSkippedPath := filepath.Join(root, "vendor", "dep.txt")
	content := "permission check\n"
	writeSearchFile(t, keptPath, content)
	writeSearchFile(t, extraSkippedPath, content)
	writeSearchFile(t, defaultSkippedPath, content)

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"permission":                              {1, 0},
			"app/main.txt:1\npermission check":        {1, 0},
			"coverage/report.txt:1\npermission check": {1, 0},
			"vendor/dep.txt:1\npermission check":      {1, 0},
		},
	}
	service := NewService(
		embedder,
		provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"},
		0,
		t.TempDir(),
		nil,
		WithSkipDirs([]string{"coverage"}),
	)

	lexical, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    10,
		Mode:          ModeLexical,
	})
	if err != nil {
		t.Fatalf("lexical Search() error = %v", err)
	}
	if len(lexical.Results) != 1 || lexical.Results[0].Path != "app/main.txt" {
		t.Fatalf("lexical results = %#v, want only app/main.txt", lexical.Results)
	}

	hybrid, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    10,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("hybrid Search() error = %v", err)
	}
	if len(hybrid.Results) != 1 || hybrid.Results[0].Path != "app/main.txt" {
		t.Fatalf("hybrid results = %#v, want only app/main.txt", hybrid.Results)
	}

	tracked := service.scanWorkspace(root)
	if len(tracked) != 1 {
		t.Fatalf("tracked files = %#v, want only non-skipped file", tracked)
	}
	if _, ok := tracked[keptPath]; !ok {
		t.Fatalf("tracked files missing %q: %#v", keptPath, tracked)
	}
}

func TestGitignoreAppliesAcrossSearchEmbeddingAndTracking(t *testing.T) {
	root := t.TempDir()
	writeSearchFile(t, filepath.Join(root, ".gitignore"), strings.Join([]string{
		"dist/",
		"*.log",
		"!important.log",
		"/coverage",
		"",
	}, "\n"))
	writeSearchFile(t, filepath.Join(root, "docs", ".gitignore"), "/generated/\n")

	keptPath := filepath.Join(root, "app", "main.txt")
	importantPath := filepath.Join(root, "important.log")
	ignoredDistPath := filepath.Join(root, "dist", "bundle.txt")
	ignoredLogPath := filepath.Join(root, "debug.log")
	ignoredCoveragePath := filepath.Join(root, "coverage", "report.txt")
	ignoredNestedPath := filepath.Join(root, "docs", "generated", "api.md")
	for _, path := range []string{keptPath, importantPath, ignoredDistPath, ignoredLogPath, ignoredCoveragePath, ignoredNestedPath} {
		writeSearchFile(t, path, "permission check\n")
	}

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"permission":                          {1, 0},
			"app/main.txt:1\npermission check":    {1, 0},
			"important.log:1\npermission check":   {1, 0},
			".gitignore:1\ndist/":                 {0, 0},
			"docs/.gitignore:1\n/generated/":      {0, 0},
			"dist/bundle.txt:1\npermission check": {1, 0},
		},
	}
	service := NewService(
		embedder,
		provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"},
		0,
		t.TempDir(),
		nil,
	)

	lexical, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    10,
		Mode:          ModeLexical,
	})
	if err != nil {
		t.Fatalf("lexical Search() error = %v", err)
	}
	if got := searchResultPaths(lexical.Results); strings.Join(got, ",") != "app/main.txt,important.log" {
		t.Fatalf("lexical result paths = %#v, want kept files only", got)
	}

	hybrid, err := service.Search(context.Background(), Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    2,
		Mode:          ModeHybrid,
	})
	if err != nil {
		t.Fatalf("hybrid Search() error = %v", err)
	}
	if got := searchResultPaths(hybrid.Results); strings.Join(got, ",") != "app/main.txt,important.log" {
		t.Fatalf("hybrid result paths = %#v, want kept files only", got)
	}
	for _, input := range embedder.Inputs() {
		if strings.Contains(input, "dist/bundle.txt") ||
			strings.Contains(input, "debug.log") ||
			strings.Contains(input, "coverage/report.txt") ||
			strings.Contains(input, "docs/generated/api.md") {
			t.Fatalf("embedder input included gitignored file: %q", input)
		}
	}

	tracked := service.scanWorkspace(root)
	for _, path := range []string{keptPath, importantPath} {
		if _, ok := tracked[path]; !ok {
			t.Fatalf("tracked files missing kept path %q: %#v", path, tracked)
		}
	}
	for _, path := range []string{ignoredDistPath, ignoredLogPath, ignoredCoveragePath, ignoredNestedPath} {
		if _, ok := tracked[path]; ok {
			t.Fatalf("tracked files included gitignored path %q: %#v", path, tracked)
		}
	}
}

func searchResultPaths(results []Result) []string {
	paths := make([]string, 0, len(results))
	for _, result := range results {
		paths = append(paths, result.Path)
	}
	return paths
}
