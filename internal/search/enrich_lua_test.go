package search

import (
	"os"
	"path/filepath"
	"testing"
)

const testLuaFile = `local SessionService = {}

--- Builds the request payload.
function SessionService:build_payload(input)
  return true
end

SessionService.from_input = function(input)
  return SessionService
end

local function check_permission(ctx, perm)
  return true
end
`

func TestLuaSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.lua")
	if err := os.WriteFile(path, []byte(testLuaFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := luaSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "package", Language: "lua", Line: 1},
		{FilePath: path, Name: "build_payload", Kind: "function", Language: "lua", Line: 4},
		{FilePath: path, Name: "from_input", Kind: "function", Language: "lua", Line: 8},
		{FilePath: path, Name: "check_permission", Kind: "function", Language: "lua", Line: 12},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "local SessionService = {" {
		t.Fatalf("table signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[1].Parent)
	}
	if enriched[1].Signature != "function SessionService:build_payload(input)" {
		t.Fatalf("method signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("assigned method parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "SessionService.from_input = function(input)" {
		t.Fatalf("assigned method signature = %q", enriched[2].Signature)
	}
	if enriched[3].Signature != "local function check_permission(ctx, perm)" {
		t.Fatalf("local function signature = %q", enriched[3].Signature)
	}
}

func TestAnalyzerRegistryEnrichesLuaSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.lua")
	if err := os.WriteFile(path, []byte(testLuaFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "lua", []Symbol{
		{FilePath: path, Name: "build_payload", Kind: "function", Language: "lua", Line: 4},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("lua signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("lua parent = %q, want SessionService", enriched[0].Parent)
	}
	if enriched[0].Doc != "Builds the request payload." {
		t.Fatalf("lua doc = %q", enriched[0].Doc)
	}
}
