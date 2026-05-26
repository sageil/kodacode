package tui

import "testing"

func TestParseSearchToolViewInputDefaultsModeToAuto(t *testing.T) {
	input, ok := parseSearchToolViewInput(`{"query":"TODO","path":"."}`)
	if !ok {
		t.Fatal("parseSearchToolViewInput() = false, want true")
	}
	if input.Mode != "auto" {
		t.Fatalf("Mode = %q, want auto", input.Mode)
	}
}
