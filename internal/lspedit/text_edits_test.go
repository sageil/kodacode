package lspedit

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/lsp"
)

func TestApplyTextEditsUsesUTF16Positions(t *testing.T) {
	updated, applied, err := ApplyTextEdits("a😀b\n", []lsp.TextEdit{{
		Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 3}, End: lsp.Position{Line: 0, Character: 4}},
		NewText: "c",
	}})
	if err != nil {
		t.Fatalf("ApplyTextEdits() error = %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	if updated != "a😀c\n" {
		t.Fatalf("updated = %q", updated)
	}
}

func TestApplyTextEditsRejectsOverlappingRanges(t *testing.T) {
	_, _, err := ApplyTextEdits("abcdef", []lsp.TextEdit{
		{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 1}, End: lsp.Position{Line: 0, Character: 4}},
			NewText: "x",
		},
		{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 3}, End: lsp.Position{Line: 0, Character: 5}},
			NewText: "y",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "overlapping text edits") {
		t.Fatalf("error = %v", err)
	}
}

func TestApplyTextEditsAppliesMultipleEditsFromOriginalContent(t *testing.T) {
	updated, applied, err := ApplyTextEdits("one two three", []lsp.TextEdit{
		{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 3}},
			NewText: "1",
		},
		{
			Range:   lsp.Range{Start: lsp.Position{Line: 0, Character: 8}, End: lsp.Position{Line: 0, Character: 13}},
			NewText: "3",
		},
	})
	if err != nil {
		t.Fatalf("ApplyTextEdits() error = %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied = %d, want 2", applied)
	}
	if updated != "1 two 3" {
		t.Fatalf("updated = %q", updated)
	}
}

func TestApplyTextEditsRejectsPositionBeyondLine(t *testing.T) {
	_, _, err := ApplyTextEdits("short\n", []lsp.TextEdit{{
		Range:   lsp.Range{Start: lsp.Position{Line: 1, Character: 1}, End: lsp.Position{Line: 1, Character: 1}},
		NewText: "x",
	}})
	if err == nil || !strings.Contains(err.Error(), "beyond the end of line") {
		t.Fatalf("error = %v", err)
	}
}
