package search

import (
	"os"
	"path/filepath"
	"testing"
)

const testZigFile = `pub const SessionService = struct {
    /// Builds the request payload.
    pub fn buildPayload(self: *SessionService, input: []const u8) bool {
        _ = self;
        _ = input;
        return true;
    }

    pub fn fromInput(input: []const u8) SessionService {
        _ = input;
        return .{};
    }
};

pub const BuildMode = enum {
    fast,
    safe,
};
`

func TestZigSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.zig")
	if err := os.WriteFile(path, []byte(testZigFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := zigSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "zig", Line: 1},
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "zig", Line: 3},
		{FilePath: path, Name: "fromInput", Kind: "function", Language: "zig", Line: 9},
		{FilePath: path, Name: "BuildMode", Kind: "type", Language: "zig", Line: 14},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "pub const SessionService = struct" {
		t.Fatalf("type signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[1].Parent)
	}
	if enriched[1].Signature != "pub fn buildPayload(self: *SessionService, input: []const u8) bool" {
		t.Fatalf("method signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("associated fn parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "pub fn fromInput(input: []const u8) SessionService" {
		t.Fatalf("associated fn signature = %q", enriched[2].Signature)
	}
	if enriched[3].Signature != "pub const BuildMode = enum" {
		t.Fatalf("enum signature = %q", enriched[3].Signature)
	}
}

func TestAnalyzerRegistryEnrichesZigSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.zig")
	if err := os.WriteFile(path, []byte(testZigFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "zig", []Symbol{
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "zig", Line: 3},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("zig signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("zig parent = %q, want SessionService", enriched[0].Parent)
	}
	if enriched[0].Doc != "Builds the request payload." {
		t.Fatalf("zig doc = %q", enriched[0].Doc)
	}
}
