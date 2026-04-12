package search

import (
	"context"
	"testing"
)

func TestHybridSearch(t *testing.T) {
	db, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	// Seed test data.
	for _, f := range []string{"/src/auth.go", "/src/session.go"} {
		if _, err := db.Exec("INSERT INTO files (path, hash, indexed_at) VALUES (?, 'h', 0)", f); err != nil {
			t.Fatal(err)
		}
	}
	symbols := []struct {
		path, name, kind, sig, doc, tokens string
		line                               int
	}{
		{"/src/auth.go", "CheckPermission", "function", "func CheckPermission()", "Checks permissions", SplitTokens("CheckPermission"), 10},
		{"/src/auth.go", "AuthService", "type", "", "Auth service", SplitTokens("AuthService"), 1},
		{"/src/session.go", "CreateSession", "function", "func CreateSession()", "Creates session", SplitTokens("CreateSession"), 25},
	}
	for _, s := range symbols {
		if _, err := db.Exec(
			"INSERT INTO symbols (file_path, name, kind, language, signature, doc, line, parent, tokens) VALUES (?, ?, ?, 'go', ?, ?, ?, '', ?)",
			s.path, s.name, s.kind, s.sig, s.doc, s.line, s.tokens,
		); err != nil {
			t.Fatal(err)
		}
	}

	emb := &mockEmbedder{dims: 4}
	ei := NewEmbeddingIndexer(EmbeddingIndexerConfig{
		DB:       db,
		Embedder: emb,
		Model:    "test-model",
	})
	if _, err := ei.Index(context.Background()); err != nil {
		t.Fatal(err)
	}

	searcher := NewSearcher(db)
	searcher.SetEmbeddingIndexer(ei)

	ctx := context.Background()

	t.Run("hybrid returns results", func(t *testing.T) {
		results, err := searcher.Search(ctx, "CheckPermission", 10)
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected results")
		}
	})

	t.Run("results have source field", func(t *testing.T) {
		results, err := searcher.Search(ctx, "permission", 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range results {
			if r.Source == "" {
				t.Errorf("result %q has empty Source", r.Name)
			}
			if r.Source != "fts" && r.Source != "vector" && r.Source != "both" {
				t.Errorf("result %q has invalid Source %q", r.Name, r.Source)
			}
		}
	})

	t.Run("fts-only fallback without embedder", func(t *testing.T) {
		ftsOnly := NewSearcher(db)
		results, err := ftsOnly.Search(ctx, "CheckPermission", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) == 0 {
			t.Fatal("FTS-only should still return results")
		}
		// Without embedder, Source should be empty (FTS-only mode).
		for _, r := range results {
			if r.Source != "" {
				t.Errorf("FTS-only result %q has Source %q, want empty", r.Name, r.Source)
			}
		}
	})
}
