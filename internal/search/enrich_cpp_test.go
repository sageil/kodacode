package search

import (
	"os"
	"path/filepath"
	"testing"
)

const testCPPFile = `class SessionService : public Buildable {
public:
    bool buildPayload(const std::string& input) const {
        return true;
    }
};

inline SessionService makeService() {
    return SessionService{};
}

class Buildable {};
`

func TestCPPSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.cpp")
	if err := os.WriteFile(path, []byte(testCPPFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := cppSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "cpp", Line: 1},
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "cpp", Line: 3},
		{FilePath: path, Name: "makeService", Kind: "function", Language: "cpp", Line: 8},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "class SessionService : public Buildable" {
		t.Fatalf("class signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[1].Parent)
	}
	if enriched[1].Signature != "bool buildPayload(const std::string& input) const" {
		t.Fatalf("method signature = %q", enriched[1].Signature)
	}
	if enriched[2].Signature != "inline SessionService makeService()" {
		t.Fatalf("free function signature = %q", enriched[2].Signature)
	}
}

func TestAnalyzerRegistryEnrichesCPPSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.cpp")
	if err := os.WriteFile(path, []byte(testCPPFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "cpp", []Symbol{
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "cpp", Line: 3},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("cpp signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("cpp parent = %q, want SessionService", enriched[0].Parent)
	}
}
