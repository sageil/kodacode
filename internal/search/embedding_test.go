package search

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"testing"
)

// mockEmbedder returns deterministic vectors based on input index.
// Safe for concurrent use.
type mockEmbedder struct {
	dims  int
	calls atomic.Int32
}

func (m *mockEmbedder) Embed(_ context.Context, _ string, texts []string) ([][]float32, error) {
	m.calls.Add(1)
	vecs := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, m.dims)
		// Each text gets a distinct direction vector.
		v[i%m.dims] = 1.0
		vecs[i] = v
	}
	return vecs, nil
}

func setupTestDB(t *testing.T) (*EmbeddingIndexer, *mockEmbedder) {
	t.Helper()
	db, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck

	// Insert test symbols.
	files := []string{"/src/auth.go", "/src/session.go"}
	for _, f := range files {
		if _, err := db.Exec("INSERT INTO files (path, hash, indexed_at) VALUES (?, 'h', 0)", f); err != nil {
			t.Fatal(err)
		}
	}
	symbols := []struct {
		path, name, kind, sig, doc string
		line                       int
	}{
		{"/src/auth.go", "CheckPermission", "function", "func CheckPermission()", "Checks permissions", 10},
		{"/src/auth.go", "AuthService", "type", "", "Auth service struct", 1},
		{"/src/session.go", "CreateSession", "function", "func CreateSession()", "Creates a session", 25},
	}
	for _, s := range symbols {
		if _, err := db.Exec(
			"INSERT INTO symbols (file_path, name, kind, language, signature, doc, line, parent, tokens) VALUES (?, ?, ?, 'go', ?, ?, ?, '', '')",
			s.path, s.name, s.kind, s.sig, s.doc, s.line,
		); err != nil {
			t.Fatal(err)
		}
	}

	emb := &mockEmbedder{dims: 4}
	indexer := NewEmbeddingIndexer(EmbeddingIndexerConfig{
		DB:         db,
		Embedder:   emb,
		Model:      "test-model",
		BatchSize:  2,
		ProjectDir: "/src",
	})
	return indexer, emb
}

func embeddingCountForFile(t *testing.T, db *sql.DB, model, path string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*)
		 FROM embeddings e
		 JOIN symbols s ON s.id = e.symbol_id
		 WHERE e.model = ? AND s.file_path = ?`,
		model, path,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestEmbeddingIndexer_Index(t *testing.T) {
	indexer, emb := setupTestDB(t)

	n, err := indexer.Index(context.Background())
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	if n != 3 {
		t.Errorf("Index() = %d, want 3", n)
	}

	// Should have made 2 batch calls (batch_size=2, 3 symbols).
	if got := emb.calls.Load(); got != 2 {
		t.Errorf("Embed() calls = %d, want 2", got)
	}

	// Running again should find nothing pending.
	n, err = indexer.Index(context.Background())
	if err != nil {
		t.Fatalf("second Index() error = %v", err)
	}
	if n != 0 {
		t.Errorf("second Index() = %d, want 0", n)
	}
}

func TestEmbeddingIndexer_SyncPathsIndexesOnlyRequestedFiles(t *testing.T) {
	indexer, emb := setupTestDB(t)
	ctx := context.Background()

	n, err := indexer.SyncPaths(ctx, []string{"/src/auth.go"})
	if err != nil {
		t.Fatalf("SyncPaths() error = %v", err)
	}
	if n != 2 {
		t.Fatalf("SyncPaths() = %d, want 2 auth.go symbols", n)
	}
	if got := emb.calls.Load(); got != 1 {
		t.Fatalf("Embed() calls after first SyncPaths = %d, want 1", got)
	}

	count, err := indexer.EmbeddingCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("EmbeddingCount() after auth.go sync = %d, want 2", count)
	}
	if got := embeddingCountForFile(t, indexer.db, "test-model", "/src/session.go"); got != 0 {
		t.Fatalf("session.go embeddings after auth.go sync = %d, want 0", got)
	}

	n, err = indexer.SyncPaths(ctx, []string{"/src/auth.go"})
	if err != nil {
		t.Fatalf("second SyncPaths() error = %v", err)
	}
	if n != 0 {
		t.Fatalf("second SyncPaths() = %d, want 0", n)
	}

	n, err = indexer.SyncPaths(ctx, []string{"/src/session.go"})
	if err != nil {
		t.Fatalf("session SyncPaths() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("session SyncPaths() = %d, want 1", n)
	}
	if got := emb.calls.Load(); got != 2 {
		t.Fatalf("Embed() calls after session sync = %d, want 2", got)
	}
}

func TestEmbeddingIndexer_EmbeddingCount(t *testing.T) {
	indexer, _ := setupTestDB(t)

	count, err := indexer.EmbeddingCount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("initial count = %d, want 0", count)
	}

	if _, err := indexer.Index(context.Background()); err != nil {
		t.Fatal(err)
	}

	count, err = indexer.EmbeddingCount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("after index count = %d, want 3", count)
	}
}

func TestEmbeddingIndexer_VectorSearch(t *testing.T) {
	indexer, _ := setupTestDB(t)
	ctx := context.Background()

	if _, err := indexer.Index(ctx); err != nil {
		t.Fatal(err)
	}

	results, err := indexer.VectorSearch(ctx, "check permission", 10)
	if err != nil {
		t.Fatalf("VectorSearch() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("VectorSearch() returned 0 results")
	}
	if len(results) > 3 {
		t.Errorf("VectorSearch() returned %d results, want <= 3", len(results))
	}
	// Results should have scores between -1 and 1.
	for _, r := range results {
		if r.Score < -1 || r.Score > 1 {
			t.Errorf("result %q score = %f, want [-1, 1]", r.Name, r.Score)
		}
	}
}

func TestEmbeddingIndexer_VectorSearchSkipsEmbeddingWhenNoCandidates(t *testing.T) {
	indexer, emb := setupTestDB(t)
	ctx := context.Background()

	if _, err := indexer.Index(ctx); err != nil {
		t.Fatal(err)
	}
	emb.calls.Store(0)

	results, err := indexer.VectorSearch(ctx, "zzzzzzzzzz", 10)
	if err != nil {
		t.Fatalf("VectorSearch() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("VectorSearch() returned %d results, want 0", len(results))
	}
	if got := emb.calls.Load(); got != 0 {
		t.Fatalf("Embed() calls = %d, want 0 when no candidates are found", got)
	}
}

func TestEmbeddingIndexer_PrefilterCandidateIDsSubstringFallback(t *testing.T) {
	indexer, _ := setupTestDB(t)

	ids, err := indexer.prefilterCandidateIDs(context.Background(), "ermission", 10)
	if err != nil {
		t.Fatalf("prefilterCandidateIDs() error = %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("prefilterCandidateIDs() returned 0 ids, want substring fallback match")
	}
}

func TestBuildCandidateLikeQuery_EscapesWildcards(t *testing.T) {
	tests := []struct {
		name    string
		term    string
		wantEsc string
	}{
		{"underscore", "check_permission", `check\_permission`},
		{"percent", "100%done", `100\%done`},
		{"backslash", `path\to`, `path\\to`},
		{"plain", "checkPermission", "checkPermission"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, args := buildCandidateLikeQuery("contains", []string{tt.term}, 10)
			if !strings.Contains(query, `ESCAPE`) {
				t.Fatalf("query missing ESCAPE clause: %s", query)
			}
			if len(args) < 3 {
				t.Fatalf("args len = %d, want >= 3", len(args))
			}
			got := args[0].(string)
			if !strings.Contains(got, tt.wantEsc) {
				t.Errorf("args[0] = %q, want to contain %q", got, tt.wantEsc)
			}
		})
	}
}

func TestBuildCandidateLikeQuery_UnderscoreExactMatch(t *testing.T) {
	db, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	for _, name := range []string{"check_permission", "checkApermission", "checkBpermission"} {
		if _, err := db.Exec(
			"INSERT INTO files (path, hash, indexed_at) VALUES ('a.go', 'h', 0) ON CONFLICT DO NOTHING"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(
			"INSERT INTO symbols (file_path, name, kind, language, line, parent, tokens) VALUES ('a.go', ?, 'function', 'go', 1, '', '')",
			name); err != nil {
			t.Fatal(err)
		}
	}

	query, args := buildCandidateLikeQuery("contains", []string{"check_permission"}, 100)
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close() error = %v", err)
		}
	}()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 1 {
		t.Fatalf("got %d matches, want 1 (only exact underscore match)", len(ids))
	}
}

func TestChunkText(t *testing.T) {
	ei := &EmbeddingIndexer{projectDir: "/proj"}
	s := pendingSymbol{
		filePath:  "/proj/src/auth.go",
		line:      10,
		kind:      "function",
		name:      "CheckPerm",
		signature: "func CheckPerm(ctx context.Context) error",
		doc:       "Checks permissions.",
	}
	got := ei.chunkText(s)
	want := "src/auth.go:10 function CheckPerm\nfunc CheckPerm(ctx context.Context) error\nChecks permissions."
	if got != want {
		t.Errorf("chunkText() =\n%s\nwant:\n%s", got, want)
	}
}

func TestChunkText_NoDocNoSig(t *testing.T) {
	ei := &EmbeddingIndexer{} // no projectDir — uses absolute path
	s := pendingSymbol{
		filePath: "/src/main.go",
		line:     1,
		kind:     "variable",
		name:     "version",
	}
	got := ei.chunkText(s)
	want := "/src/main.go:1 variable version"
	if got != want {
		t.Errorf("chunkText() = %q, want %q", got, want)
	}
}

func TestEmbeddingIndexer_DimensionValidation(t *testing.T) {
	db, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	if _, err := db.Exec("INSERT INTO files (path, hash, indexed_at) VALUES ('a.go', 'h', 0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO symbols (file_path, name, kind, language, line, parent, tokens) VALUES ('a.go', 'Foo', 'function', 'go', 1, '', '')"); err != nil {
		t.Fatal(err)
	}

	emb := &mockEmbedder{dims: 4}
	// Config expects 8 dimensions but mock returns 4.
	indexer := NewEmbeddingIndexer(EmbeddingIndexerConfig{
		DB:         db,
		Embedder:   emb,
		Model:      "test-model",
		BatchSize:  100,
		Dimensions: 8,
	})

	_, err = indexer.Index(context.Background())
	if err == nil {
		t.Fatal("expected dimension mismatch error")
	}
	if got := err.Error(); !strings.Contains(got, "dimension mismatch") {
		t.Errorf("error = %q, want to contain 'dimension mismatch'", got)
	}
}
