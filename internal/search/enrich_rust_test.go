package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRustFile = `pub struct SessionService<T> {
    repo: T,
}

pub trait Buildable {
    fn build_payload(&self) -> bool;
}

impl<T> SessionService<T> {
    pub async fn build_payload(&self, input: &str) -> bool {
        true
    }

    #[instrument(skip(self))]
    pub fn from_input(input: T) -> Self {
        Self { repo: input }
    }
}
`

func TestRustSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.rs")
	if err := os.WriteFile(path, []byte(testRustFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := rustSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "rust", Line: 1},
		{FilePath: path, Name: "Buildable", Kind: "trait", Language: "rust", Line: 5},
		{FilePath: path, Name: "build_payload", Kind: "function", Language: "rust", Line: 10},
		{FilePath: path, Name: "from_input", Kind: "function", Language: "rust", Line: 15},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "pub struct SessionService<T>" {
		t.Fatalf("struct signature = %q", enriched[0].Signature)
	}
	if enriched[1].Signature != "pub trait Buildable" {
		t.Fatalf("trait signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "pub async fn build_payload(&self, input: &str) -> bool" {
		t.Fatalf("method signature = %q", enriched[2].Signature)
	}
	if enriched[3].Parent != "SessionService" {
		t.Fatalf("associated fn parent = %q, want SessionService", enriched[3].Parent)
	}
	if enriched[3].Signature != "pub fn from_input(input: T) -> Self" {
		t.Fatalf("associated fn signature = %q", enriched[3].Signature)
	}
}

func TestStripBraceCountingLine_SlashInString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"url in string", `let s = "http://example.com";`, `let s = ;`},
		{"real comment", `let x = 1; // todo`, `let x = 1; `},
		{"comment after string", `let s = "hello"; // note`, `let s = ; `},
		{"no comment", `let x = a + b;`, `let x = a + b;`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripBraceCountingLine(tt.input)
			if got != tt.want {
				t.Errorf("stripBraceCountingLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRustSnippetIgnoresStringBraces(t *testing.T) {
	dir := t.TempDir()
	src := `pub fn format_value(input: &str) -> String {
    format!("value={{{}}}", input)
}
`
	path := filepath.Join(dir, "braces.rs")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := rustSymbolEnricher{}
	enriched := enricher.Enrich(path, []Symbol{
		{FilePath: path, Name: "format_value", Kind: "function", Language: "rust", Line: 1},
	})
	if !strings.Contains(enriched[0].Signature, "format_value") {
		t.Fatalf("signature truncated by string brace: %q", enriched[0].Signature)
	}
}

func TestAnalyzerRegistryEnrichesRustSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.rs")
	if err := os.WriteFile(path, []byte(testRustFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "rust", []Symbol{
		{FilePath: path, Name: "build_payload", Kind: "function", Language: "rust", Line: 10},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("rust signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("rust parent = %q, want SessionService", enriched[0].Parent)
	}
}
