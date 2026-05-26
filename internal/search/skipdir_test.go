package search

import (
	"context"
	"path/filepath"
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
