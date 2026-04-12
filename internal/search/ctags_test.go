package search

import (
	"encoding/json"
	"testing"
)

func TestCtagsEntryParsing(t *testing.T) {
	lines := []string{
		`{"_type":"tag","name":"Open","path":"db.go","language":"Go","kind":"func","line":14,"signature":"(projectDir string)"}`,
		`{"_type":"tag","name":"Symbol","path":"types.go","language":"Go","kind":"struct","line":3,"scope":"","scopeKind":""}`,
		`{"_type":"tag","name":"Search","path":"search.go","language":"Go","kind":"method","line":10,"scope":"Searcher","scopeKind":"struct","signature":"(ctx context.Context, query string)"}`,
		`{"_type":"ptag","name":"JSON_OUTPUT_VERSION","path":"","language":"NONE"}`,
	}

	var symbols []Symbol
	for _, line := range lines {
		var entry ctagsEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		if entry.Type != "tag" {
			continue
		}
		symbols = append(symbols, Symbol{
			FilePath:  entry.Path,
			Name:      entry.Name,
			Kind:      normalizeKind(entry.Kind),
			Language:  entry.Language,
			Signature: entry.Signature,
			Line:      entry.Line,
			Parent:    entry.Scope,
			Tokens:    SplitTokens(entry.Name),
		})
	}

	if len(symbols) != 3 {
		t.Fatalf("got %d symbols, want 3", len(symbols))
	}

	if symbols[0].Kind != "function" {
		t.Errorf("Open kind = %q, want function", symbols[0].Kind)
	}
	if symbols[1].Kind != "type" {
		t.Errorf("Symbol kind = %q, want type", symbols[1].Kind)
	}
	if symbols[2].Parent != "Searcher" {
		t.Errorf("Search parent = %q, want Searcher", symbols[2].Parent)
	}
}

func TestNormalizeKind(t *testing.T) {
	tests := map[string]string{
		"func":       "function",
		"function":   "function",
		"method":     "function",
		"class":      "type",
		"struct":     "type",
		"interface":  "interface",
		"const":      "const",
		"enumerator": "const",
		"var":        "variable",
		"field":      "variable",
		"package":    "package",
		"unknown":    "unknown",
	}
	for input, want := range tests {
		if got := normalizeKind(input); got != want {
			t.Errorf("normalizeKind(%q) = %q, want %q", input, got, want)
		}
	}
}
