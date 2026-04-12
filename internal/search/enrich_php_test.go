package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripPHPLine_CommentInString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"url in double quotes", `$s = "http://example.com";`, `$s = ;`},
		{"url in single quotes", `$s = 'http://example.com';`, `$s = ;`},
		{"hash in string", `$s = "color #ff0";`, `$s = ;`},
		{"real slash comment", `$x = 1; // todo`, `$x = 1; `},
		{"real hash comment", `$x = 1; # todo`, `$x = 1; `},
		{"comment after string", `$s = "hello"; // note`, `$s = ; `},
		{"no comment", `$x = $a + $b;`, `$x = $a + $b;`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripPHPLine(tt.input)
			if got != tt.want {
				t.Errorf("stripPHPLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

const testPHPFile = `<?php

final class SessionService implements Buildable
{
    public function buildPayload(string $input): bool
    {
        return true;
    }

    public static function fromInput(string $input): self
    {
        return new self();
    }
}

interface Buildable
{
    public function buildPayload(string $input): bool;
}
`

func TestPHPSymbolEnricher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.php")
	if err := os.WriteFile(path, []byte(testPHPFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := phpSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "php", Line: 3},
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "php", Line: 5},
		{FilePath: path, Name: "fromInput", Kind: "function", Language: "php", Line: 10},
		{FilePath: path, Name: "Buildable", Kind: "interface", Language: "php", Line: 15},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "final class SessionService implements Buildable" {
		t.Fatalf("class signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[1].Parent)
	}
	if enriched[1].Signature != "public function buildPayload(string $input): bool" {
		t.Fatalf("method signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("factory parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "public static function fromInput(string $input): self" {
		t.Fatalf("factory signature = %q", enriched[2].Signature)
	}
	if enriched[3].Signature != "interface Buildable" {
		t.Fatalf("interface signature = %q", enriched[3].Signature)
	}
}

func TestAnalyzerRegistryEnrichesPHPSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.php")
	if err := os.WriteFile(path, []byte(testPHPFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "php", []Symbol{
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "php", Line: 5},
	})
	if len(enriched) != 1 {
		t.Fatalf("symbols len = %d, want 1", len(enriched))
	}
	if enriched[0].Signature == "" {
		t.Fatal("php signature missing after registry enrichment")
	}
	if enriched[0].Parent != "SessionService" {
		t.Fatalf("php parent = %q, want SessionService", enriched[0].Parent)
	}
}
