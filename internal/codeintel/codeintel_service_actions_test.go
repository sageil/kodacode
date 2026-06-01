package codeintel

import (
	"testing"

	"github.com/sageil/kodacode/internal/lsp"
	"github.com/sageil/kodacode/internal/tool"
)

func TestCodeActionCandidateRangesQuickfixRetriesOverlappingDiagnostics(t *testing.T) {
	request := tool.CodeIntelCodeActionRequest{Kind: "quickfix"}
	initial := lsp.Range{
		Start: lsp.Position{Line: 144, Character: 15},
		End:   lsp.Position{Line: 145, Character: 31},
	}

	ranges := codeActionCandidateRanges(request, initial, []lsp.Diagnostic{
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 145, Character: 4},
				End:   lsp.Position{Line: 145, Character: 18},
			},
		},
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 148, Character: 0},
				End:   lsp.Position{Line: 148, Character: 12},
			},
		},
	}, []lsp.Range{
		{
			Start: lsp.Position{Line: 144, Character: 20},
			End:   lsp.Position{Line: 144, Character: 20},
		},
	})

	if len(ranges) != 2 {
		t.Fatalf("len(ranges) = %d, want 2", len(ranges))
	}
	if ranges[0] != initial {
		t.Fatalf("ranges[0] = %#v, want %#v", ranges[0], initial)
	}
	wantDiagnostic := (lsp.Range{
		Start: lsp.Position{Line: 145, Character: 4},
		End:   lsp.Position{Line: 145, Character: 18},
	})
	if ranges[1] != wantDiagnostic {
		t.Fatalf("ranges[1] = %#v, want %#v", ranges[1], wantDiagnostic)
	}
}

func TestCodeActionCandidateRangesQuickfixRetriesDiagnosticAtCursorBoundary(t *testing.T) {
	request := tool.CodeIntelCodeActionRequest{Kind: "quickfix"}
	initial := lsp.Range{
		Start: lsp.Position{Line: 10, Character: 5},
		End:   lsp.Position{Line: 10, Character: 5},
	}
	wantDiagnostic := lsp.Range{
		Start: lsp.Position{Line: 10, Character: 5},
		End:   lsp.Position{Line: 10, Character: 12},
	}

	ranges := codeActionCandidateRanges(request, initial, []lsp.Diagnostic{
		{Range: wantDiagnostic},
	}, nil)

	if len(ranges) != 2 {
		t.Fatalf("len(ranges) = %d, want 2", len(ranges))
	}
	if ranges[1] != wantDiagnostic {
		t.Fatalf("ranges[1] = %#v, want %#v", ranges[1], wantDiagnostic)
	}
}

func TestCodeActionCandidateRangesNonQuickfixKeepsFallbackRanges(t *testing.T) {
	request := tool.CodeIntelCodeActionRequest{Kind: "source.organizeImports"}
	initial := lsp.Range{
		Start: lsp.Position{Line: 10, Character: 0},
		End:   lsp.Position{Line: 10, Character: 20},
	}
	fallback := []lsp.Range{
		{
			Start: lsp.Position{Line: 10, Character: 8},
			End:   lsp.Position{Line: 10, Character: 8},
		},
	}

	ranges := codeActionCandidateRanges(request, initial, []lsp.Diagnostic{
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 10, Character: 1},
				End:   lsp.Position{Line: 10, Character: 5},
			},
		},
	}, fallback)

	if len(ranges) != 2 {
		t.Fatalf("len(ranges) = %d, want 2", len(ranges))
	}
	if ranges[1] != fallback[0] {
		t.Fatalf("ranges[1] = %#v, want %#v", ranges[1], fallback[0])
	}
}
