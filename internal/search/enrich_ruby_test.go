package search

import (
	"os"
	"path/filepath"
	"testing"
)

const testRubyFile = `module Permissions
  class SessionService < BaseService
    # Builds the request payload.
    def build_payload(input, strict: true)
      true
    end

    def self.from_input(input)
      new
    end
  end
end
`

func TestRubySymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.rb")
	if err := os.WriteFile(path, []byte(testRubyFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := rubySymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "Permissions", Kind: "package", Language: "ruby", Line: 1},
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "ruby", Line: 2},
		{FilePath: path, Name: "build_payload", Kind: "function", Language: "ruby", Line: 4},
		{FilePath: path, Name: "from_input", Kind: "function", Language: "ruby", Line: 8},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "module Permissions" {
		t.Fatalf("module signature = %q", enriched[0].Signature)
	}
	if enriched[1].Signature != "class SessionService < BaseService" {
		t.Fatalf("class signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "def build_payload(input, strict: true)" {
		t.Fatalf("method signature = %q", enriched[2].Signature)
	}
	if enriched[3].Parent != "SessionService" {
		t.Fatalf("class method parent = %q, want SessionService", enriched[3].Parent)
	}
	if enriched[3].Signature != "def self.from_input(input)" {
		t.Fatalf("class method signature = %q", enriched[3].Signature)
	}
}

func TestAnalyzerRegistryEnrichesRubySymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.rb")
	if err := os.WriteFile(path, []byte(testRubyFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "ruby", []Symbol{
		{FilePath: path, Name: "build_payload", Kind: "function", Language: "ruby", Line: 4},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("ruby signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("ruby parent = %q, want SessionService", enriched[0].Parent)
	}
	if enriched[0].Doc != "Builds the request payload." {
		t.Fatalf("ruby doc = %q", enriched[0].Doc)
	}
}
