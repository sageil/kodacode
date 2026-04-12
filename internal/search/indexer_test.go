package search

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldIndex(t *testing.T) {
	dir := t.TempDir()

	makeFile := func(rel string, size int) string {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
		return abs
	}

	mainGo := makeFile("main.go", 100)
	nestedJS := makeFile("node_modules/pkg/index.js", 50)
	deepGit := makeFile(".git/objects/abc", 30)
	cacheFile := makeFile(".cache/data.bin", 40)
	srcFile := makeFile("src/app.go", 80)
	vendorFile := makeFile("vendor/lib/lib.go", 60)
	bigFile := makeFile("huge.bin", 2_000_000)
	pyCache := makeFile("__pycache__/mod.pyc", 20)

	tests := []struct {
		name    string
		ignore  []string
		exclude []string
		maxSize int64
		absPath string
		relPath string
		want    bool
	}{
		{
			name:    "plain file with no patterns",
			maxSize: 1_000_000,
			absPath: mainGo,
			relPath: "main.go",
			want:    true,
		},
		{
			name:    "doublestar ignore matches nested path",
			ignore:  []string{"node_modules/**"},
			maxSize: 1_000_000,
			absPath: nestedJS,
			relPath: "node_modules/pkg/index.js",
			want:    false,
		},
		{
			name:    "doublestar ignore matches .git subtree",
			ignore:  []string{".git/**"},
			maxSize: 1_000_000,
			absPath: deepGit,
			relPath: ".git/objects/abc",
			want:    false,
		},
		{
			name:    "doublestar ignore matches .cache subtree",
			ignore:  []string{".cache/**"},
			maxSize: 1_000_000,
			absPath: cacheFile,
			relPath: ".cache/data.bin",
			want:    false,
		},
		{
			name:    "doublestar ignore matches vendor subtree",
			ignore:  []string{"vendor/**"},
			maxSize: 1_000_000,
			absPath: vendorFile,
			relPath: "vendor/lib/lib.go",
			want:    false,
		},
		{
			name:    "doublestar ignore matches __pycache__ subtree",
			ignore:  []string{"__pycache__/**"},
			maxSize: 1_000_000,
			absPath: pyCache,
			relPath: "__pycache__/mod.pyc",
			want:    false,
		},
		{
			name:    "non-matching ignore allows file",
			ignore:  []string{"node_modules/**"},
			maxSize: 1_000_000,
			absPath: srcFile,
			relPath: "src/app.go",
			want:    true,
		},
		{
			name:    "exclude patterns use doublestar",
			exclude: []string{"vendor/**"},
			maxSize: 1_000_000,
			absPath: vendorFile,
			relPath: "vendor/lib/lib.go",
			want:    false,
		},
		{
			name:    "file exceeding max size rejected",
			maxSize: 1_000_000,
			absPath: bigFile,
			relPath: "huge.bin",
			want:    false,
		},
		{
			name:    "nonexistent file rejected",
			maxSize: 1_000_000,
			absPath: filepath.Join(dir, "missing.txt"),
			relPath: "missing.txt",
			want:    false,
		},
		{
			name:    "multiple ignore patterns applied together",
			ignore:  []string{"node_modules/**", ".git/**", "__pycache__/**"},
			maxSize: 1_000_000,
			absPath: nestedJS,
			relPath: "node_modules/pkg/index.js",
			want:    false,
		},
		{
			name:    "multiple ignore patterns allow clean file",
			ignore:  []string{"node_modules/**", ".git/**", "__pycache__/**"},
			maxSize: 1_000_000,
			absPath: srcFile,
			relPath: "src/app.go",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ix := &Indexer{
				cfg: IndexerConfig{
					IgnorePatterns:  tt.ignore,
					ExcludePatterns: tt.exclude,
					MaxFileSize:     tt.maxSize,
				},
			}
			got := ix.shouldIndex(tt.absPath, tt.relPath)
			if got != tt.want {
				t.Errorf("shouldIndex(%q, %q) = %v, want %v", tt.absPath, tt.relPath, got, tt.want)
			}
		})
	}
}

// setupIndexerTest creates a temp project directory with Go files and a fresh DB.
func setupIndexerTest(t *testing.T) (*sql.DB, string) {
	t.Helper()
	projectDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectDir, "auth.go"), []byte(`package example

// CheckPermission verifies the user has the given permission.
func CheckPermission(ctx context.Context, perm string) error {
	return nil
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(projectDir, "session.go"), []byte(`package example

// SessionService manages user sessions.
type SessionService struct {
	repo Repo
}

// Create creates a new session.
func (s *SessionService) Create(ctx context.Context) (*Session, error) {
	return nil, nil
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := Open(t.TempDir(), projectDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck
	return db, projectDir
}

func querySymbols(t *testing.T, db *sql.DB) []Symbol {
	t.Helper()
	rows, err := db.Query("SELECT file_path, name, kind, language, signature, doc, line, parent, tokens FROM symbols ORDER BY file_path, line")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("rows.Close() error = %v", err)
		}
	}()
	var out []Symbol
	for rows.Next() {
		var s Symbol
		if err := rows.Scan(&s.FilePath, &s.Name, &s.Kind, &s.Language, &s.Signature, &s.Doc, &s.Line, &s.Parent, &s.Tokens); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func countSymbolsForFile(t *testing.T, db *sql.DB, path string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM symbols WHERE file_path = ?", path).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestIndexer_IndexGoFiles(t *testing.T) {
	db, projectDir := setupIndexerTest(t)
	ix := NewIndexer(db, projectDir, IndexerConfig{})
	ctx := context.Background()

	n, err := ix.Index(ctx)
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	if n == 0 {
		t.Fatal("Index() indexed 0 files, want > 0")
	}

	symbols := querySymbols(t, db)
	if len(symbols) == 0 {
		t.Fatal("no symbols in DB after indexing")
	}

	byName := make(map[string]Symbol, len(symbols))
	for _, s := range symbols {
		byName[s.Name] = s
	}

	if sym, ok := byName["CheckPermission"]; !ok {
		t.Fatal("CheckPermission not found in index")
	} else {
		if sym.Kind != "function" {
			t.Errorf("CheckPermission kind = %q, want function", sym.Kind)
		}
		if sym.Language != "go" {
			t.Errorf("CheckPermission language = %q, want go", sym.Language)
		}
		if sym.Signature == "" {
			t.Error("CheckPermission signature is empty")
		}
		if sym.Doc == "" {
			t.Error("CheckPermission doc is empty")
		}
	}

	if sym, ok := byName["SessionService"]; !ok {
		t.Fatal("SessionService not found in index")
	} else if sym.Doc == "" {
		t.Error("SessionService doc is empty")
	}

	if sym, ok := byName["Create"]; !ok {
		t.Fatal("Create not found in index")
	} else {
		if sym.Parent == "" {
			t.Errorf("Create parent is empty, want non-empty")
		}
		if sym.Signature == "" {
			t.Error("Create signature is empty")
		}
	}
}

func TestIndexer_SyncPathsUpdatesChangedFile(t *testing.T) {
	db, projectDir := setupIndexerTest(t)
	ix := NewIndexer(db, projectDir, IndexerConfig{})
	ctx := context.Background()

	if _, err := ix.Index(ctx); err != nil {
		t.Fatalf("Index() error = %v", err)
	}

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

	n, err := ix.SyncPaths(ctx, []string{authPath})
	if err != nil {
		t.Fatalf("SyncPaths() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("SyncPaths() indexed %d files, want 1", n)
	}

	var sawAuthorize, sawCheckPermission bool
	for _, sym := range querySymbols(t, db) {
		switch sym.Name {
		case "Authorize":
			sawAuthorize = true
		case "CheckPermission":
			sawCheckPermission = true
		}
	}
	if !sawAuthorize {
		t.Fatal("updated symbol Authorize not found after SyncPaths")
	}
	if sawCheckPermission {
		t.Fatal("stale symbol CheckPermission should have been removed after SyncPaths")
	}
}

func TestIndexer_SyncPathsRemovesDeletedFile(t *testing.T) {
	db, projectDir := setupIndexerTest(t)
	ix := NewIndexer(db, projectDir, IndexerConfig{})
	ctx := context.Background()

	if _, err := ix.Index(ctx); err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	sessionPath := filepath.Join(projectDir, "session.go")
	if count := countSymbolsForFile(t, db, sessionPath); count == 0 {
		t.Fatal("expected indexed symbols for session.go before deletion")
	}
	if err := os.Remove(sessionPath); err != nil {
		t.Fatal(err)
	}

	n, err := ix.SyncPaths(ctx, []string{sessionPath})
	if err != nil {
		t.Fatalf("SyncPaths() error = %v", err)
	}
	if n != 0 {
		t.Fatalf("SyncPaths() indexed %d files for deleted path, want 0", n)
	}
	if count := countSymbolsForFile(t, db, sessionPath); count != 0 {
		t.Fatalf("deleted file still has %d indexed symbols, want 0", count)
	}
}

func TestIndexer_IndexIdempotent(t *testing.T) {
	db, projectDir := setupIndexerTest(t)
	ix := NewIndexer(db, projectDir, IndexerConfig{})
	ctx := context.Background()

	if _, err := ix.Index(ctx); err != nil {
		t.Fatalf("first Index() error = %v", err)
	}

	n, err := ix.Index(ctx)
	if err != nil {
		t.Fatalf("second Index() error = %v", err)
	}
	if n != 0 {
		t.Errorf("second Index() = %d, want 0 (no changes)", n)
	}
}

func TestIndexer_IndexDetectsChanges(t *testing.T) {
	db, projectDir := setupIndexerTest(t)
	ix := NewIndexer(db, projectDir, IndexerConfig{})
	ctx := context.Background()

	if _, err := ix.Index(ctx); err != nil {
		t.Fatal(err)
	}

	// Wait briefly so mtime differs.
	time.Sleep(10 * time.Millisecond)

	// Modify auth.go by adding a new function.
	if err := os.WriteFile(filepath.Join(projectDir, "auth.go"), []byte(`package example

// CheckPermission verifies the user has the given permission.
func CheckPermission(ctx context.Context, perm string) error {
	return nil
}

// ValidateToken validates an auth token.
func ValidateToken(token string) bool {
	return true
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := ix.Index(ctx)
	if err != nil {
		t.Fatalf("second Index() error = %v", err)
	}
	if n == 0 {
		t.Fatal("second Index() = 0, want > 0 after modification")
	}

	symbols := querySymbols(t, db)
	found := false
	for _, s := range symbols {
		if s.Name == "ValidateToken" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("ValidateToken not found after re-index")
	}
}

func TestIndexer_RemoveDeleted(t *testing.T) {
	db, projectDir := setupIndexerTest(t)
	ix := NewIndexer(db, projectDir, IndexerConfig{})
	ctx := context.Background()

	if _, err := ix.Index(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify auth.go symbols exist.
	symbols := querySymbols(t, db)
	hasAuth := false
	for _, s := range symbols {
		if s.Name == "CheckPermission" {
			hasAuth = true
			break
		}
	}
	if !hasAuth {
		t.Fatal("CheckPermission not found before deletion")
	}

	// Delete auth.go from disk.
	if err := os.Remove(filepath.Join(projectDir, "auth.go")); err != nil {
		t.Fatal(err)
	}

	// Re-index should remove auth.go symbols.
	if _, err := ix.Index(ctx); err != nil {
		t.Fatal(err)
	}

	symbols = querySymbols(t, db)
	for _, s := range symbols {
		if s.Name == "CheckPermission" {
			t.Fatal("CheckPermission still in DB after file deletion")
		}
	}
}

func TestIndexer_EnrichSymbols(t *testing.T) {
	db, projectDir := setupIndexerTest(t)
	ix := NewIndexer(db, projectDir, IndexerConfig{})
	ctx := context.Background()

	if _, err := ix.Index(ctx); err != nil {
		t.Fatal(err)
	}

	symbols := querySymbols(t, db)
	for _, s := range symbols {
		if s.Kind == "function" && s.Signature == "" {
			t.Errorf("function %q has empty signature", s.Name)
		}
		if s.Tokens == "" {
			t.Errorf("symbol %q has empty tokens", s.Name)
		}
	}

	// Verify method has parent set.
	for _, s := range symbols {
		if s.Name == "Create" {
			if s.Parent == "" {
				t.Error("Create parent is empty, want non-empty")
			}
			return
		}
	}
	t.Fatal("Create method not found in enriched symbols")
}
