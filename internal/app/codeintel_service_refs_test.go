package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/lsp"
	"github.com/sageil/kodacode/internal/tool"
)

type fakeRefsServer struct {
	references func(context.Context, string, int, int, bool) ([]lsp.Location, error)
	highlights func(context.Context, string, int, int) ([]lsp.DocumentHighlight, error)
}

func (f fakeRefsServer) References(ctx context.Context, path string, line, character int, includeDeclaration bool) ([]lsp.Location, error) {
	return f.references(ctx, path, line, character, includeDeclaration)
}

func (f fakeRefsServer) DocumentHighlights(ctx context.Context, path string, line, character int) ([]lsp.DocumentHighlight, error) {
	return f.highlights(ctx, path, line, character)
}

func TestRefsFromServerReturnsUnsupportedWhenReferencesAreUnavailable(t *testing.T) {
	result, err := refsFromServer(context.Background(), fakeRefsServer{
		references: func(context.Context, string, int, int, bool) ([]lsp.Location, error) {
			return nil, lsp.ErrReferencesUnsupported
		},
		highlights: func(context.Context, string, int, int) ([]lsp.DocumentHighlight, error) {
			return nil, nil
		},
	}, tool.CodeIntelRefsRequest{
		Path:       "/repo/store.ts",
		Line:       12,
		Character:  4,
		Mode:       tool.CodeIntelRefsModeAll,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("refsFromServer() error = %v", err)
	}
	if result.Notice == nil || result.Notice.Kind != tool.CodeIntelNoticeKindUnsupported {
		t.Fatalf("result = %#v, want unsupported notice", result)
	}
}

func TestWorkspaceCodeIntelRefsReturnsUnavailableNoticeWhenFileTypeHasNoConfiguredServer(t *testing.T) {
	root := t.TempDir()
	service := NewCodeIntelService(LSPConfig{AutoDiscover: false})

	result, err := service.Navigator(root, nil).Refs(context.Background(), tool.CodeIntelRefsRequest{
		Path:       filepath.Join(root, "notes.txt"),
		Line:       1,
		Character:  0,
		Mode:       tool.CodeIntelRefsModeAll,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("Refs() error = %v", err)
	}
	if result.Notice == nil || result.Notice.Kind != tool.CodeIntelNoticeKindUnavailable {
		t.Fatalf("result = %#v, want unavailable notice", result)
	}
	if !strings.Contains(result.Notice.Message, `no LSP server configured for file type ".txt"`) {
		t.Fatalf("notice = %#v", result.Notice)
	}
}

func TestRefsFromServerClassifiesReadersAndWriters(t *testing.T) {
	result, err := refsFromServer(context.Background(), fakeRefsServer{
		references: func(context.Context, string, int, int, bool) ([]lsp.Location, error) {
			return []lsp.Location{
				{URI: "file:///repo/store.ts", Range: lsp.Range{Start: lsp.Position{Line: 9, Character: 2}}},
				{URI: "file:///repo/view.ts", Range: lsp.Range{Start: lsp.Position{Line: 4, Character: 8}}},
			}, nil
		},
		highlights: func(_ context.Context, path string, line, character int) ([]lsp.DocumentHighlight, error) {
			switch path {
			case "/repo/store.ts":
				return []lsp.DocumentHighlight{{Range: lsp.Range{Start: lsp.Position{Line: line, Character: character}, End: lsp.Position{Line: line, Character: character + 5}}, Kind: 3}}, nil
			case "/repo/view.ts":
				return []lsp.DocumentHighlight{{Range: lsp.Range{Start: lsp.Position{Line: line, Character: character}, End: lsp.Position{Line: line, Character: character + 5}}, Kind: 2}}, nil
			default:
				return nil, nil
			}
		},
	}, tool.CodeIntelRefsRequest{
		Path:       "/repo/store.ts",
		Line:       1,
		Character:  1,
		Mode:       tool.CodeIntelRefsModeAll,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("refsFromServer() error = %v", err)
	}
	if !result.Supported || !result.Found {
		t.Fatalf("result = %#v", result)
	}
	if len(result.References) != 2 {
		t.Fatalf("references = %#v", result.References)
	}
	if result.References[0].Kind != tool.CodeIntelReferenceKindWrite || result.References[1].Kind != tool.CodeIntelReferenceKindRead {
		t.Fatalf("references = %#v", result.References)
	}
}

func TestRefsFromServerReaderModeOmitsUnclassifiedResults(t *testing.T) {
	result, err := refsFromServer(context.Background(), fakeRefsServer{
		references: func(context.Context, string, int, int, bool) ([]lsp.Location, error) {
			return []lsp.Location{
				{URI: "file:///repo/store.ts", Range: lsp.Range{Start: lsp.Position{Line: 9, Character: 2}}},
				{URI: "file:///repo/view.ts", Range: lsp.Range{Start: lsp.Position{Line: 4, Character: 8}}},
			}, nil
		},
		highlights: func(_ context.Context, path string, line, character int) ([]lsp.DocumentHighlight, error) {
			if path == "/repo/store.ts" {
				return nil, lsp.ErrDocumentHighlightUnsupported
			}
			return []lsp.DocumentHighlight{{Range: lsp.Range{Start: lsp.Position{Line: line, Character: character}, End: lsp.Position{Line: line, Character: character + 5}}, Kind: 2}}, nil
		},
	}, tool.CodeIntelRefsRequest{
		Path:       "/repo/store.ts",
		Line:       1,
		Character:  1,
		Mode:       tool.CodeIntelRefsModeReaders,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("refsFromServer() error = %v", err)
	}
	if len(result.References) != 1 || result.References[0].Kind != tool.CodeIntelReferenceKindRead {
		t.Fatalf("references = %#v", result.References)
	}
	if result.ClassificationSupported {
		t.Fatalf("result = %#v, want classificationSupported=false", result)
	}
	if !result.ClassificationIncomplete {
		t.Fatalf("result = %#v, want classificationIncomplete=true", result)
	}
}

func TestRefsFromServerPropagatesHighlightErrors(t *testing.T) {
	_, err := refsFromServer(context.Background(), fakeRefsServer{
		references: func(context.Context, string, int, int, bool) ([]lsp.Location, error) {
			return []lsp.Location{{URI: "file:///repo/store.ts", Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}}}, nil
		},
		highlights: func(context.Context, string, int, int) ([]lsp.DocumentHighlight, error) {
			return nil, errors.New("server exploded")
		},
	}, tool.CodeIntelRefsRequest{
		Path:       "/repo/store.ts",
		Line:       1,
		Character:  0,
		Mode:       tool.CodeIntelRefsModeAll,
		MaxResults: 10,
	})
	if err == nil || err.Error() != "server exploded" {
		t.Fatalf("refsFromServer() error = %v", err)
	}
}
