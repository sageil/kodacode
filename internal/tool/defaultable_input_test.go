package tool

import (
	"encoding/json"
	"testing"
)

func TestParseSearchInputAcceptsOmittedDefaultableFields(t *testing.T) {
	input, err := parseSearchInput(json.RawMessage(`{"query":"cache","path":"."}`))
	if err != nil {
		t.Fatalf("parseSearchInput() error = %v", err)
	}
	if input.Mode != "" {
		t.Fatalf("Mode = %q, want empty auto mode", input.Mode)
	}
	if input.Glob != "" {
		t.Fatalf("Glob = %q, want empty", input.Glob)
	}
	if input.Regex {
		t.Fatal("Regex = true, want false")
	}
	if input.CaseSensitive {
		t.Fatal("CaseSensitive = true, want false")
	}
	if input.MaxMatches != searchDefaultMaxMatches {
		t.Fatalf("MaxMatches = %d, want %d", input.MaxMatches, searchDefaultMaxMatches)
	}
}

func TestParseWebFetchInputAcceptsOmittedDefaultableFields(t *testing.T) {
	input, parsed, err := parseWebFetchInput(json.RawMessage(`{"url":"https://example.com/docs"}`))
	if err != nil {
		t.Fatalf("parseWebFetchInput() error = %v", err)
	}
	if parsed == nil || parsed.String() != "https://example.com/docs" {
		t.Fatalf("parsed URL = %#v", parsed)
	}
	if input.Method != "GET" {
		t.Fatalf("Method = %q, want GET", input.Method)
	}
	if len(input.Headers) != 0 {
		t.Fatalf("Headers = %#v, want empty", input.Headers)
	}
	if input.Body != "" {
		t.Fatalf("Body = %q, want empty", input.Body)
	}
	if input.Format != "auto" {
		t.Fatalf("Format = %q, want auto", input.Format)
	}
	if input.Selector != "" {
		t.Fatalf("Selector = %q, want empty", input.Selector)
	}
}
