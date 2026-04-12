package search

import (
	"context"
	"testing"
)

func TestSearchIntegration(t *testing.T) {
	db, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	symbols := []Symbol{
		{FilePath: "/src/auth.go", Name: "CheckPermission", Kind: "function", Language: "go",
			Signature: "func CheckPermission(ctx context.Context, perm string) error",
			Doc: "CheckPermission verifies that the user has the given permission.",
			Line: 10, Tokens: SplitTokens("CheckPermission")},
		{FilePath: "/src/session.go", Name: "CreateSession", Kind: "function", Language: "go",
			Signature: "func (s *SessionService) CreateSession(ctx context.Context) (*Session, error)",
			Doc: "CreateSession creates a new user session.",
			Line: 25, Tokens: SplitTokens("CreateSession")},
		{FilePath: "/src/session.go", Name: "SessionService", Kind: "type", Language: "go",
			Doc: "SessionService manages user sessions.",
			Line: 5, Tokens: SplitTokens("SessionService")},
		{FilePath: "/src/handler.go", Name: "HandleLogin", Kind: "function", Language: "go",
			Signature: "func HandleLogin(w http.ResponseWriter, r *http.Request)",
			Doc: "HandleLogin processes login requests and creates sessions.",
			Line: 15, Tokens: SplitTokens("HandleLogin")},
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"/src/auth.go", "/src/session.go", "/src/handler.go"} {
		if _, err := tx.Exec("INSERT INTO files (path, hash, indexed_at) VALUES (?, 'abc', 0)", f); err != nil {
			t.Fatal(err)
		}
	}
	stmt, err := tx.Prepare(`INSERT INTO symbols (file_path, name, kind, language, signature, doc, line, parent, tokens) VALUES (?, ?, ?, ?, ?, ?, ?, '', ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, sym := range symbols {
		if _, err := stmt.Exec(sym.FilePath, sym.Name, sym.Kind, sym.Language, sym.Signature, sym.Doc, sym.Line, sym.Tokens); err != nil {
			t.Fatal(err)
		}
	}
	stmt.Close() //nolint:errcheck
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	searcher := NewSearcher(db)
	ctx := context.Background()

	t.Run("exact name match", func(t *testing.T) {
		results, err := searcher.Search(ctx, "CheckPermission", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) == 0 {
			t.Fatal("expected results for CheckPermission")
		}
		if results[0].Name != "CheckPermission" {
			t.Errorf("first result = %q, want CheckPermission", results[0].Name)
		}
	})

	t.Run("natural language query", func(t *testing.T) {
		results, err := searcher.Search(ctx, "user permission check", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) == 0 {
			t.Fatal("expected results for permission check")
		}
		found := false
		for _, r := range results {
			if r.Name == "CheckPermission" {
				found = true
				break
			}
		}
		if !found {
			t.Error("CheckPermission not in results for 'user permission check'")
		}
	})

	t.Run("session related", func(t *testing.T) {
		results, err := searcher.Search(ctx, "session", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) < 2 {
			t.Fatalf("expected >= 2 results for session, got %d", len(results))
		}
	})

	t.Run("symbol count", func(t *testing.T) {
		count, err := searcher.SymbolCount(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if count != len(symbols) {
			t.Errorf("symbol count = %d, want %d", count, len(symbols))
		}
	})

	t.Run("empty query", func(t *testing.T) {
		results, err := searcher.Search(ctx, "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results for empty query, got %d", len(results))
		}
	})
}

func TestBuildFTSQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"CheckPermission", `"CheckPermission" OR "check" OR "permission" OR "checkpermission"`},
		{"simple", `"simple"`},
		{"", ""},
		{"a", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := buildFTSQuery(tt.input)
			if got != tt.want {
				t.Errorf("buildFTSQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
