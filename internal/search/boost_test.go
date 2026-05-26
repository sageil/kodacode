package search

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestPathBoostMultiplierMatchesDirectorySuffixAndSubstring(t *testing.T) {
	config := pathBoostConfig{
		enabled: true,
		penalties: []pathBoostRule{
			{pattern: "/tests/", factor: 0.5},
			{pattern: ".md", factor: 0.6},
			{pattern: "_test.", factor: 0.4},
		},
		bonuses: []pathBoostRule{
			{pattern: "/src/", factor: 1.1},
		},
	}

	if got := pathBoostMultiplier("src/auth.go", config); got != 1.1 {
		t.Fatalf("src multiplier = %v, want 1.1", got)
	}
	if got := pathBoostMultiplier("pkg/tests/auth.go", config); got != 0.5 {
		t.Fatalf("tests multiplier = %v, want 0.5", got)
	}
	if got := pathBoostMultiplier("docs/guide.md", config); got != 0.6 {
		t.Fatalf("suffix multiplier = %v, want 0.6", got)
	}
	if got := pathBoostMultiplier("src/auth_test.go", config); got != 0.44000000000000006 {
		t.Fatalf("combined multiplier = %v, want 0.44", got)
	}
}

func TestPathBoostMultiplierDisabledReturnsOne(t *testing.T) {
	if got := pathBoostMultiplier("src/auth.go", pathBoostConfig{
		enabled: false,
		bonuses: []pathBoostRule{{pattern: "/src/", factor: 1.1}},
	}); got != 1 {
		t.Fatalf("disabled multiplier = %v, want 1", got)
	}
}

func TestHybridSearchAppliesPathBoostsToChunkRanking(t *testing.T) {
	root := t.TempDir()
	srcPath := filepath.Join(root, "src", "auth.go")
	docsPath := filepath.Join(root, "docs", "auth.md")
	srcContent := "auth gate\n"
	docsContent := "auth gate\n"
	writeSearchFile(t, srcPath, srcContent)
	writeSearchFile(t, docsPath, docsContent)

	embedder := &fakeEmbedder{
		vectors: map[string][]float32{
			"auth":                      {1, 0},
			"src/auth.go:1\nauth gate":  {1, 0},
			"docs/auth.md:1\nauth gate": {1, 0},
		},
	}
	service := NewService(embedder, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, t.TempDir(), nil)

	response, err := service.Search(context.Background(), Request{
		Query:         "auth",
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
	if response.Results[0].Path != "src/auth.go" {
		t.Fatalf("first result path = %q, want src/auth.go", response.Results[0].Path)
	}
}
