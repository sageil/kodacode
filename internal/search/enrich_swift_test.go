package search

import (
	"os"
	"path/filepath"
	"testing"
)

const testSwiftFile = `public final class SessionService: Buildable {
    init(repo: Repo) {}

    func buildPayload(input: String) -> Bool {
        true
    }

    static func fromInput(_ input: String) -> SessionService {
        SessionService(repo: Repo())
    }
}

protocol Buildable {
    func buildPayload(input: String) -> Bool
}
`

func TestSwiftSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Example.swift")
	if err := os.WriteFile(path, []byte(testSwiftFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := swiftSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "swift", Line: 1},
		{FilePath: path, Name: "init", Kind: "function", Language: "swift", Line: 2},
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "swift", Line: 4},
		{FilePath: path, Name: "fromInput", Kind: "function", Language: "swift", Line: 8},
		{FilePath: path, Name: "Buildable", Kind: "interface", Language: "swift", Line: 12},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "public final class SessionService: Buildable" {
		t.Fatalf("class signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("init parent = %q, want SessionService", enriched[1].Parent)
	}
	if enriched[1].Signature != "init(repo: Repo)" {
		t.Fatalf("init signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "func buildPayload(input: String) -> Bool" {
		t.Fatalf("method signature = %q", enriched[2].Signature)
	}
	if enriched[3].Parent != "SessionService" {
		t.Fatalf("factory parent = %q, want SessionService", enriched[3].Parent)
	}
	if enriched[3].Signature != "static func fromInput(_ input: String) -> SessionService" {
		t.Fatalf("factory signature = %q", enriched[3].Signature)
	}
	if enriched[4].Signature != "protocol Buildable" {
		t.Fatalf("protocol signature = %q", enriched[4].Signature)
	}
}

func TestAnalyzerRegistryEnrichesSwiftSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Example.swift")
	if err := os.WriteFile(path, []byte(testSwiftFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "swift", []Symbol{
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "swift", Line: 4},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("swift signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("swift parent = %q, want SessionService", enriched[0].Parent)
	}
}
