package search

import (
	"os"
	"path/filepath"
	"testing"
)

const testPythonFile = `class SessionService(BaseService):
    """SessionService manages user sessions."""

    @classmethod
    def create(cls, user_id: str) -> bool:
        """Create builds a new session."""
        return True


@audit("perm")
async def check_permission(ctx, perm: str) -> bool:
    """CheckPermission verifies access."""
    return True
`

func TestPythonSymbolEnricher(t *testing.T) {
	if pythonRuntimePath() == "" {
		t.Skip("python runtime not available")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "example.py")
	if err := os.WriteFile(path, []byte(testPythonFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := pythonSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "python", Line: 1},
		{FilePath: path, Name: "create", Kind: "function", Language: "python", Line: 4},
		{FilePath: path, Name: "check_permission", Kind: "function", Language: "python", Line: 10},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Doc != "SessionService manages user sessions." {
		t.Fatalf("class doc = %q, want %q", enriched[0].Doc, "SessionService manages user sessions.")
	}
	if enriched[0].Signature != "class SessionService(BaseService)" {
		t.Fatalf("class signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want %q", enriched[1].Parent, "SessionService")
	}
	if enriched[1].Doc != "Create builds a new session." {
		t.Fatalf("method doc = %q, want %q", enriched[1].Doc, "Create builds a new session.")
	}
	if enriched[1].Signature != "def create(cls, user_id: str) -> bool" {
		t.Fatalf("method signature = %q", enriched[1].Signature)
	}
	if enriched[2].Doc != "CheckPermission verifies access." {
		t.Fatalf("function doc = %q, want %q", enriched[2].Doc, "CheckPermission verifies access.")
	}
	if enriched[2].Signature != "async def check_permission(ctx, perm: str) -> bool" {
		t.Fatalf("function signature = %q", enriched[2].Signature)
	}
}

func TestAnalyzerRegistryEnrichesPythonSymbols(t *testing.T) {
	if pythonRuntimePath() == "" {
		t.Skip("python runtime not available")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "example.py")
	if err := os.WriteFile(path, []byte(testPythonFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "python", []Symbol{
		{FilePath: path, Name: "create", Kind: "function", Language: "python", Line: 4},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Doc == "" {
		t.Fatal("python doc missing after registry enrichment")
	}
	if enriched[0].Signature == "" {
		t.Fatal("python signature missing after registry enrichment")
	}
}
