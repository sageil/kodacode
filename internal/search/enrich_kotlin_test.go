package search

import (
	"os"
	"path/filepath"
	"testing"
)

const testKotlinFile = `sealed class SessionService : Buildable {
    override suspend fun buildPayload(input: String): Boolean {
        return true
    }

    companion object {
        fun fromInput(input: String): SessionService = TODO()
    }
}

interface Buildable {
    suspend fun buildPayload(input: String): Boolean
}
`

func TestKotlinSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Example.kt")
	if err := os.WriteFile(path, []byte(testKotlinFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := kotlinSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "kotlin", Line: 1},
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "kotlin", Line: 2},
		{FilePath: path, Name: "fromInput", Kind: "function", Language: "kotlin", Line: 7},
		{FilePath: path, Name: "Buildable", Kind: "interface", Language: "kotlin", Line: 11},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "sealed class SessionService : Buildable" {
		t.Fatalf("class signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[1].Parent)
	}
	if enriched[1].Signature != "override suspend fun buildPayload(input: String): Boolean" {
		t.Fatalf("method signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("factory parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "fun fromInput(input: String): SessionService" {
		t.Fatalf("factory signature = %q", enriched[2].Signature)
	}
	if enriched[3].Signature != "interface Buildable" {
		t.Fatalf("interface signature = %q", enriched[3].Signature)
	}
}

func TestAnalyzerRegistryEnrichesKotlinSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Example.kt")
	if err := os.WriteFile(path, []byte(testKotlinFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "kotlin", []Symbol{
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "kotlin", Line: 2},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("kotlin signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("kotlin parent = %q, want SessionService", enriched[0].Parent)
	}
}
