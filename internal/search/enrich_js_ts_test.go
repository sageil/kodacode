package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testTypeScriptFile = `export class SessionService extends BaseService implements Buildable {
  /**
   * Build request payload.
   */
  buildPayload<T extends Input>(input: T): boolean {
    return true
  }

  constructor(private readonly repo: Repo) {}

  get ready(): boolean {
    return true
  }

  static fromInput = function(input: string): SessionService {
    return new SessionService(input as unknown as Repo)
  }
}

export const checkPermission = async <T extends Context>(ctx: T, perm: string): Promise<boolean> => {
  return true
}

export const buildRunner = function(input: string): boolean {
  return true
}
`

const testJavaScriptFile = `/**
 * Build request payload.
 */
export async function buildPayload(input) {
  return input
}

export const runCheck = async value => {
  return value
}
`

func TestJSTSSymbolEnricherTypeScript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.ts")
	if err := os.WriteFile(path, []byte(testTypeScriptFile), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := jsTSSymbolEnricher{}
	symbols := []Symbol{
		{FilePath: path, Name: "SessionService", Kind: "type", Language: "typescript", Line: 1},
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "typescript", Line: 5},
		{FilePath: path, Name: "constructor", Kind: "function", Language: "typescript", Line: 9},
		{FilePath: path, Name: "ready", Kind: "function", Language: "typescript", Line: 13},
		{FilePath: path, Name: "fromInput", Kind: "function", Language: "typescript", Line: 16},
		{FilePath: path, Name: "checkPermission", Kind: "function", Language: "typescript", Line: 21},
		{FilePath: path, Name: "buildRunner", Kind: "function", Language: "typescript", Line: 25},
	}

	enriched := enricher.Enrich(path, symbols)
	if enriched[0].Signature != "export class SessionService extends BaseService implements Buildable" {
		t.Fatalf("type signature = %q", enriched[0].Signature)
	}
	if enriched[1].Parent != "SessionService" {
		t.Fatalf("method parent = %q, want SessionService", enriched[1].Parent)
	}
	if enriched[1].Signature != "buildPayload<T extends Input>(input: T): boolean" {
		t.Fatalf("method signature = %q", enriched[1].Signature)
	}
	if enriched[2].Parent != "SessionService" {
		t.Fatalf("constructor parent = %q, want SessionService", enriched[2].Parent)
	}
	if enriched[2].Signature != "constructor(private readonly repo: Repo)" {
		t.Fatalf("constructor signature = %q", enriched[2].Signature)
	}
	if enriched[3].Parent != "SessionService" {
		t.Fatalf("getter parent = %q, want SessionService", enriched[3].Parent)
	}
	if enriched[3].Signature != "get ready(): boolean" {
		t.Fatalf("getter signature = %q", enriched[3].Signature)
	}
	if enriched[4].Parent != "SessionService" {
		t.Fatalf("function-expression parent = %q, want SessionService", enriched[4].Parent)
	}
	if enriched[4].Signature != "static fromInput = function(input: string): SessionService" {
		t.Fatalf("function-expression signature = %q", enriched[4].Signature)
	}
	if enriched[5].Signature != "export const checkPermission = async <T extends Context>(ctx: T, perm: string): Promise<boolean> =>" {
		t.Fatalf("generic arrow signature = %q", enriched[5].Signature)
	}
	if enriched[6].Signature != "export const buildRunner = function(input: string): boolean" {
		t.Fatalf("function expression signature = %q", enriched[6].Signature)
	}
}

func TestStripStringsAndLineComments_SlashInString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"url in double quotes", `x = "http://example.com"`, `x = `},
		{"url in single quotes", `x = 'http://example.com'`, `x = `},
		{"url in backticks", "x = `http://example.com`", "x = "},
		{"real comment after code", `x = 1 // comment`, `x = 1 `},
		{"comment after string", `x = "hello" // comment`, `x =  `},
		{"no comment", `x = a + b`, `x = a + b`},
		{"slash not doubled", `x = a / b`, `x = a / b`},
		{"empty string", `x = ""`, `x = `},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripStringsAndLineComments(tt.input)
			if got != tt.want {
				t.Errorf("stripStringsAndLineComments(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestJSTSSnippetIgnoresStringBraces(t *testing.T) {
	dir := t.TempDir()
	src := `export function buildPayload(input: string = "a { b }") {
  return input
}
`
	path := filepath.Join(dir, "braces.ts")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	enricher := jsTSSymbolEnricher{}
	enriched := enricher.Enrich(path, []Symbol{
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "typescript", Line: 1},
	})
	if !strings.Contains(enriched[0].Signature, "buildPayload") {
		t.Fatalf("signature truncated by string brace: %q", enriched[0].Signature)
	}
}

func TestAnalyzerRegistryEnrichesJSTSSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.js")
	if err := os.WriteFile(path, []byte(testJavaScriptFile), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewAnalyzerRegistry()
	enriched := reg.Enrich(path, "javascript", []Symbol{
		{FilePath: path, Name: "buildPayload", Kind: "function", Language: "javascript", Line: 4},
		{FilePath: path, Name: "runCheck", Kind: "function", Language: "javascript", Line: 8},
	})
	if len(enriched) != 2 {
		t.Fatalf("symbols len = %d, want 2", len(enriched))
	}
	if enriched[0].Signature != "export async function buildPayload(input)" {
		t.Fatalf("function signature = %q", enriched[0].Signature)
	}
	if enriched[0].Doc != "Build request payload." {
		t.Fatalf("function doc = %q", enriched[0].Doc)
	}
	if enriched[1].Signature != "export const runCheck = async value =>" {
		t.Fatalf("bare arrow signature = %q", enriched[1].Signature)
	}
}
