package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommentDocEnricherAcrossLanguages(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		source   string
		symbol   Symbol
		wantDoc  string
		language string
	}{
		{
			name:    "javascript block comment",
			relPath: "example.js",
			source: `/**
 * Builds the request payload.
 */
function buildPayload() {}
`,
			symbol:   Symbol{Name: "buildPayload", Kind: "function", Line: 4, Language: "javascript"},
			wantDoc:  "Builds the request payload.",
			language: "javascript",
		},
		{
			name:    "typescript line comments",
			relPath: "example.ts",
			source: `// Builds the request payload.
// Keeps field ordering stable.
export function buildPayload(): void {}
`,
			symbol:   Symbol{Name: "buildPayload", Kind: "function", Line: 3, Language: "typescript"},
			wantDoc:  "Builds the request payload.\nKeeps field ordering stable.",
			language: "typescript",
		},
		{
			name:    "python hash comments",
			relPath: "example.py",
			source: `# Builds the request payload.
def build_payload():
    return None
`,
			symbol:   Symbol{Name: "build_payload", Kind: "function", Line: 2, Language: "python"},
			wantDoc:  "Builds the request payload.",
			language: "python",
		},
		{
			name:    "ruby hash comments",
			relPath: "example.rb",
			source: `# Builds the request payload.
def build_payload
end
`,
			symbol:   Symbol{Name: "build_payload", Kind: "function", Line: 2, Language: "ruby"},
			wantDoc:  "Builds the request payload.",
			language: "ruby",
		},
		{
			name:    "lua dash comments",
			relPath: "example.lua",
			source: `--- Builds the request payload.
function build_payload() end
`,
			symbol:   Symbol{Name: "build_payload", Kind: "function", Line: 2, Language: "lua"},
			wantDoc:  "Builds the request payload.",
			language: "lua",
		},
		{
			name:    "rust doc comments",
			relPath: "example.rs",
			source: `/// Builds the request payload.
pub fn build_payload() {}
`,
			symbol:   Symbol{Name: "build_payload", Kind: "function", Line: 2, Language: "rust"},
			wantDoc:  "Builds the request payload.",
			language: "rust",
		},
		{
			name:    "zig doc comments",
			relPath: "example.zig",
			source: `/// Builds the request payload.
pub fn buildPayload() void {}
`,
			symbol:   Symbol{Name: "buildPayload", Kind: "function", Line: 2, Language: "zig"},
			wantDoc:  "Builds the request payload.",
			language: "zig",
		},
	}

	enricher := commentDocEnricher{}

	t.Run("empty file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.js")
		if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		got := enricher.Enrich(path, []Symbol{{Name: "x", Kind: "function", Line: 1, Language: "javascript"}})
		if len(got) != 1 {
			t.Fatalf("symbols len = %d, want 1", len(got))
		}
		if got[0].Doc != "" {
			t.Errorf("doc = %q, want empty for empty file", got[0].Doc)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		got := enricher.Enrich("/nonexistent/file.js", []Symbol{{Name: "x", Kind: "function", Line: 1, Language: "javascript"}})
		if len(got) != 1 {
			t.Fatalf("symbols len = %d, want 1", len(got))
		}
	})

	t.Run("symbol on line 1 skipped", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "first.js")
		if err := os.WriteFile(path, []byte("function x() {}"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := enricher.Enrich(path, []Symbol{{Name: "x", Kind: "function", Line: 1, Language: "javascript"}})
		if got[0].Doc != "" {
			t.Errorf("line-1 symbol should have no doc, got %q", got[0].Doc)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tt.relPath)
			if err := os.WriteFile(path, []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}
			if !enricher.Supports(tt.language, path) {
				t.Fatalf("enricher does not support %s", tt.language)
			}
			got := enricher.Enrich(path, []Symbol{tt.symbol})
			if len(got) != 1 {
				t.Fatalf("symbols len = %d, want 1", len(got))
			}
			if got[0].Doc != tt.wantDoc {
				t.Fatalf("doc = %q, want %q", got[0].Doc, tt.wantDoc)
			}
		})
	}
}
