package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const testGoFile = `package example

// CheckPermission verifies that the user has the given permission.
func CheckPermission(ctx context.Context, perm string) error {
	return nil
}

// SessionService manages user sessions.
type SessionService struct {
	store Store
}

// Create creates a new session for the user.
func (s *SessionService) Create(ctx context.Context, userID string) (*Session, error) {
	return nil, nil
}

var defaultTimeout = 30
`

func TestEnrichGoSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.go")
	if err := os.WriteFile(path, []byte(testGoFile), 0o644); err != nil {
		t.Fatal(err)
	}

	symbols := []Symbol{
		{FilePath: path, Name: "CheckPermission", Kind: "function", Line: 4},
		{FilePath: path, Name: "SessionService", Kind: "type", Line: 9},
		{FilePath: path, Name: "Create", Kind: "function", Line: 14, Parent: "SessionService"},
		{FilePath: path, Name: "defaultTimeout", Kind: "variable", Line: 18},
	}

	enriched := EnrichGoSymbols(path, symbols)

	if enriched[0].Doc != "CheckPermission verifies that the user has the given permission." {
		t.Errorf("CheckPermission doc = %q", enriched[0].Doc)
	}
	if enriched[0].Signature != "func CheckPermission(ctx context.Context, perm string) error" {
		t.Errorf("CheckPermission sig = %q", enriched[0].Signature)
	}

	if enriched[1].Doc != "SessionService manages user sessions." {
		t.Errorf("SessionService doc = %q", enriched[1].Doc)
	}

	if enriched[2].Doc != "Create creates a new session for the user." {
		t.Errorf("Create doc = %q", enriched[2].Doc)
	}
	if enriched[2].Signature != "func (s *SessionService) Create(ctx context.Context, userID string) (*Session, error)" {
		t.Errorf("Create sig = %q", enriched[2].Signature)
	}
}

func TestExtractGoSymbolsWithoutCtags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.go")
	if err := os.WriteFile(path, []byte(testGoFile), 0o644); err != nil {
		t.Fatal(err)
	}

	symbols := ExtractGoSymbols(path)
	if len(symbols) != 4 {
		t.Fatalf("symbols len = %d, want 4", len(symbols))
	}

	byName := make(map[string]Symbol, len(symbols))
	for _, sym := range symbols {
		byName[sym.Name] = sym
	}

	if got := byName["CheckPermission"]; got.Kind != "function" || got.Signature == "" {
		t.Fatalf("CheckPermission = %+v, want function with signature", got)
	}
	if got := byName["SessionService"]; got.Kind != "type" {
		t.Fatalf("SessionService kind = %q, want type", got.Kind)
	}
	if got := byName["Create"]; got.Parent != "SessionService" || got.Kind != "function" {
		t.Fatalf("Create = %+v, want method on SessionService", got)
	}
	if got := byName["defaultTimeout"]; got.Kind != "variable" {
		t.Fatalf("defaultTimeout kind = %q, want variable", got.Kind)
	}
}

func TestExtractGoSymbols_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.go")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	symbols := ExtractGoSymbols(path)
	if len(symbols) != 0 {
		t.Errorf("empty file produced %d symbols, want 0", len(symbols))
	}
}

func TestEnrichGoSymbols_EmptySymbolSlice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.go")
	if err := os.WriteFile(path, []byte(testGoFile), 0o644); err != nil {
		t.Fatal(err)
	}
	enriched := EnrichGoSymbols(path, nil)
	if len(enriched) != 0 {
		t.Errorf("nil input produced %d symbols, want 0", len(enriched))
	}
}

func TestEnrichGoSymbols_MalformedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.go")
	if err := os.WriteFile(path, []byte(`package main
func broken( {
	unclosed
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Should not panic on malformed input.
	symbols := ExtractGoSymbols(path)
	_ = symbols

	enriched := EnrichGoSymbols(path, []Symbol{
		{FilePath: path, Name: "broken", Kind: "function", Line: 2},
	})
	_ = enriched
}

func TestExtractGoSymbols_TokensPopulated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.go")
	if err := os.WriteFile(path, []byte(testGoFile), 0o644); err != nil {
		t.Fatal(err)
	}
	symbols := ExtractGoSymbols(path)
	for _, sym := range symbols {
		if sym.Tokens == "" {
			t.Errorf("symbol %q has empty Tokens", sym.Name)
		}
	}
}

func TestFallbackExtract_UnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(`{"key": "value"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewAnalyzerRegistry()
	symbols := reg.FallbackExtract(context.Background(), path)
	if len(symbols) != 0 {
		t.Errorf("unsupported file produced %d symbols, want 0", len(symbols))
	}
}

func TestEnrich_EmptySymbolSlice(t *testing.T) {
	reg := NewAnalyzerRegistry()
	result := reg.Enrich("/fake/file.go", "go", nil)
	if result != nil {
		t.Errorf("nil input returned %v, want nil", result)
	}
	result = reg.Enrich("/fake/file.go", "go", []Symbol{})
	if len(result) != 0 {
		t.Errorf("empty input returned %d symbols, want 0", len(result))
	}
}

func TestAnalyzerRegistryEnrichesGoSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.go")
	if err := os.WriteFile(path, []byte(testGoFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	symbols := []Symbol{
		{FilePath: path, Name: "CheckPermission", Kind: "function", Language: "go", Line: 4},
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "go", Line: 9},
	}

	enriched := reg.Enrich(path, "go", symbols)
	if enriched[0].Signature == "" {
		t.Fatal("CheckPermission signature missing after enrichment")
	}
	if enriched[0].Doc == "" {
		t.Fatal("CheckPermission doc missing after enrichment")
	}
	if enriched[1].Doc == "" {
		t.Fatal("SessionService doc missing after enrichment")
	}
}
