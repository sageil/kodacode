package codeintel

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/lsp"
	"github.com/sageil/kodacode/internal/tool"
)

type fakeDiagnosticsServer struct {
	refresh func(context.Context, string) ([]lsp.Diagnostic, error)
}

func (f fakeDiagnosticsServer) RefreshDiagnostics(ctx context.Context, filePath string) ([]lsp.Diagnostic, error) {
	return f.refresh(ctx, filePath)
}

func TestCollectWorkspaceDiagnosticsPreservesPartialFailures(t *testing.T) {
	grouped := map[string][]string{
		"/repo": []string{"src/a.ts", "src/b.ts"},
	}

	results, err := collectWorkspaceDiagnostics(context.Background(), grouped, 25*time.Millisecond, func(_ context.Context, _ string, path string) (diagnosticsServer, error) {
		switch path {
		case "src/a.ts":
			return fakeDiagnosticsServer{
				refresh: func(context.Context, string) ([]lsp.Diagnostic, error) {
					return []lsp.Diagnostic{{
						Range: lsp.Range{
							Start: lsp.Position{Line: 2, Character: 4},
							End:   lsp.Position{Line: 2, Character: 8},
						},
						Severity: 1,
						Message:  "broken",
						Source:   "vtsls",
					}}, nil
				},
			}, nil
		case "src/b.ts":
			return fakeDiagnosticsServer{
				refresh: func(context.Context, string) ([]lsp.Diagnostic, error) {
					return nil, context.DeadlineExceeded
				},
			}, nil
		default:
			t.Fatalf("unexpected path %q", path)
			return nil, nil
		}
	})
	if err != nil {
		t.Fatalf("collectWorkspaceDiagnostics() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Path != "src/a.ts" || len(results[0].Diagnostics) != 1 || results[0].Error != "" {
		t.Fatalf("results[0] = %#v", results[0])
	}
	if results[1].Path != "src/b.ts" {
		t.Fatalf("results[1] = %#v", results[1])
	}
	if !strings.Contains(results[1].Error, "timed out waiting for diagnostics") {
		t.Fatalf("results[1].Error = %q", results[1].Error)
	}
}

func TestCollectWorkspaceDiagnosticsPropagatesParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := collectWorkspaceDiagnostics(ctx, map[string][]string{
		"/repo": []string{"src/a.ts"},
	}, 25*time.Millisecond, func(context.Context, string, string) (diagnosticsServer, error) {
		return nil, errors.New("start failed")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("collectWorkspaceDiagnostics() error = %v, want context.Canceled", err)
	}
}

func TestCodeIntelServiceWorkspaceServerStatusListsConfiguredServers(t *testing.T) {
	service := NewCodeIntelService(Config{
		AutoDiscover: false,
		Servers: []lsp.ServerConfig{
			{Name: "vtsls", Command: "vtsls", Extensions: []string{".ts"}},
			{Name: "gopls", Command: "gopls", Extensions: []string{".go"}},
		},
	})

	status := service.WorkspaceServerStatus("/repo", []string{"/repo/subproject", "/repo"})
	if status != nil {
		t.Fatalf("WorkspaceServerStatus() = %#v, want nil without active servers", status)
	}
}

func TestWorkspaceCodeIntelSymbolsReturnsUnavailableNoticeWhenServersFailToStart(t *testing.T) {
	root := t.TempDir()
	service := NewCodeIntelService(Config{
		AutoDiscover: false,
		Servers: []lsp.ServerConfig{{
			Name:       "missing",
			Command:    "definitely-not-a-real-lsp-binary",
			Extensions: []string{".go"},
		}},
	})

	result, err := service.Navigator(root, nil).Symbols(context.Background(), "SessionService")
	if err != nil {
		t.Fatalf("Symbols() error = %v", err)
	}
	if result.Notice == nil || result.Notice.Kind != tool.CodeIntelNoticeKindUnavailable {
		t.Fatalf("result = %#v, want unavailable notice", result)
	}
	if !strings.Contains(result.Notice.Message, "definitely-not-a-real-lsp-binary not found") {
		t.Fatalf("notice = %#v", result.Notice)
	}
}

func TestWorkspaceCodeIntelTraceReturnsUnavailableNoticeWhenFileTypeHasNoConfiguredServer(t *testing.T) {
	root := t.TempDir()
	service := NewCodeIntelService(Config{AutoDiscover: false})

	result, err := service.Navigator(root, nil).Trace(context.Background(), tool.CodeIntelTraceRequest{
		Path:      filepath.Join(root, "notes.txt"),
		Line:      1,
		Character: 0,
		Mode:      tool.CodeIntelTraceModeCallers,
	})
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if result.Notice == nil || result.Notice.Kind != tool.CodeIntelNoticeKindUnavailable {
		t.Fatalf("result = %#v, want unavailable notice", result)
	}
	if !strings.Contains(result.Notice.Message, `no LSP server configured for file type ".txt"`) {
		t.Fatalf("notice = %#v", result.Notice)
	}
}

type fakeTraceServer struct {
	prepare  func(context.Context, string, int, int) ([]lsp.CallHierarchyItem, error)
	incoming func(context.Context, lsp.CallHierarchyItem) ([]lsp.CallHierarchyIncomingCall, error)
	outgoing func(context.Context, lsp.CallHierarchyItem) ([]lsp.CallHierarchyOutgoingCall, error)
}

func (f fakeTraceServer) PrepareCallHierarchy(ctx context.Context, filePath string, line, character int) ([]lsp.CallHierarchyItem, error) {
	return f.prepare(ctx, filePath, line, character)
}

func (f fakeTraceServer) IncomingCalls(ctx context.Context, item lsp.CallHierarchyItem) ([]lsp.CallHierarchyIncomingCall, error) {
	return f.incoming(ctx, item)
}

func (f fakeTraceServer) OutgoingCalls(ctx context.Context, item lsp.CallHierarchyItem) ([]lsp.CallHierarchyOutgoingCall, error) {
	return f.outgoing(ctx, item)
}

func TestTraceFromServerReturnsUnsupportedWhenCallHierarchyIsUnavailable(t *testing.T) {
	result, err := traceFromServer(context.Background(), fakeTraceServer{
		prepare: func(context.Context, string, int, int) ([]lsp.CallHierarchyItem, error) {
			return nil, lsp.ErrCallHierarchyUnsupported
		},
		incoming: func(context.Context, lsp.CallHierarchyItem) ([]lsp.CallHierarchyIncomingCall, error) {
			return nil, nil
		},
		outgoing: func(context.Context, lsp.CallHierarchyItem) ([]lsp.CallHierarchyOutgoingCall, error) {
			return nil, nil
		},
	}, tool.CodeIntelTraceRequest{
		Path:      "/repo/service.go",
		Line:      12,
		Character: 4,
		Mode:      tool.CodeIntelTraceModeCallers,
		Depth:     2,
		MaxNodes:  10,
	})
	if err != nil {
		t.Fatalf("traceFromServer() error = %v", err)
	}
	if result.Notice == nil || result.Notice.Kind != tool.CodeIntelNoticeKindUnsupported {
		t.Fatalf("result = %#v, want unsupported notice", result)
	}
}

func TestTraceFromServerReturnsNoCallableSymbolWhenPrepareIsEmpty(t *testing.T) {
	result, err := traceFromServer(context.Background(), fakeTraceServer{
		prepare: func(context.Context, string, int, int) ([]lsp.CallHierarchyItem, error) {
			return nil, nil
		},
		incoming: func(context.Context, lsp.CallHierarchyItem) ([]lsp.CallHierarchyIncomingCall, error) {
			return nil, nil
		},
		outgoing: func(context.Context, lsp.CallHierarchyItem) ([]lsp.CallHierarchyOutgoingCall, error) {
			return nil, nil
		},
	}, tool.CodeIntelTraceRequest{
		Path:      "/repo/service.go",
		Line:      12,
		Character: 4,
		Mode:      tool.CodeIntelTraceModeCallers,
		Depth:     2,
		MaxNodes:  10,
	})
	if err != nil {
		t.Fatalf("traceFromServer() error = %v", err)
	}
	if !result.Supported || result.Found {
		t.Fatalf("result = %#v, want supported no-result state", result)
	}
}

func TestTraceFromServerCallersReturnsOneHopOnly(t *testing.T) {
	root := lsp.CallHierarchyItem{
		Name:           "Search",
		Kind:           12,
		URI:            "file:///repo/service.go",
		SelectionRange: lsp.Range{Start: lsp.Position{Line: 10, Character: 2}},
	}
	caller := lsp.CallHierarchyItem{
		Name:           "Run",
		Kind:           12,
		URI:            "file:///repo/runner.go",
		SelectionRange: lsp.Range{Start: lsp.Position{Line: 4, Character: 1}},
	}
	incomingCalls := 0
	outgoingCalls := 0
	result, err := traceFromServer(context.Background(), fakeTraceServer{
		prepare: func(context.Context, string, int, int) ([]lsp.CallHierarchyItem, error) {
			return []lsp.CallHierarchyItem{root}, nil
		},
		incoming: func(_ context.Context, item lsp.CallHierarchyItem) ([]lsp.CallHierarchyIncomingCall, error) {
			incomingCalls++
			if item.Name != "Search" {
				t.Fatalf("unexpected incoming item: %#v", item)
			}
			return []lsp.CallHierarchyIncomingCall{{From: caller}}, nil
		},
		outgoing: func(context.Context, lsp.CallHierarchyItem) ([]lsp.CallHierarchyOutgoingCall, error) {
			outgoingCalls++
			return nil, nil
		},
	}, tool.CodeIntelTraceRequest{
		Path:      "/repo/service.go",
		Line:      11,
		Character: 2,
		Mode:      tool.CodeIntelTraceModeCallers,
		Depth:     4,
		MaxNodes:  10,
	})
	if err != nil {
		t.Fatalf("traceFromServer() error = %v", err)
	}
	if incomingCalls != 1 || outgoingCalls != 0 {
		t.Fatalf("incoming/outgoing calls = %d/%d, want 1/0", incomingCalls, outgoingCalls)
	}
	if !result.Supported || !result.Found {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Nodes) != 2 || len(result.Edges) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestTraceFromServerGraphDedupesNodesAndRespectsDepth(t *testing.T) {
	root := lsp.CallHierarchyItem{Name: "A", Kind: 12, URI: "file:///repo/a.go", SelectionRange: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}}
	b := lsp.CallHierarchyItem{Name: "B", Kind: 12, URI: "file:///repo/b.go", SelectionRange: lsp.Range{Start: lsp.Position{Line: 1, Character: 0}}}
	c := lsp.CallHierarchyItem{Name: "C", Kind: 12, URI: "file:///repo/c.go", SelectionRange: lsp.Range{Start: lsp.Position{Line: 2, Character: 0}}}
	d := lsp.CallHierarchyItem{Name: "D", Kind: 12, URI: "file:///repo/d.go", SelectionRange: lsp.Range{Start: lsp.Position{Line: 3, Character: 0}}}
	e := lsp.CallHierarchyItem{Name: "E", Kind: 12, URI: "file:///repo/e.go", SelectionRange: lsp.Range{Start: lsp.Position{Line: 4, Character: 0}}}

	result, err := traceFromServer(context.Background(), fakeTraceServer{
		prepare: func(context.Context, string, int, int) ([]lsp.CallHierarchyItem, error) {
			return []lsp.CallHierarchyItem{root}, nil
		},
		incoming: func(_ context.Context, item lsp.CallHierarchyItem) ([]lsp.CallHierarchyIncomingCall, error) {
			switch item.Name {
			case "A":
				return []lsp.CallHierarchyIncomingCall{{From: b}}, nil
			case "B":
				return []lsp.CallHierarchyIncomingCall{{From: d}}, nil
			case "D":
				return []lsp.CallHierarchyIncomingCall{{From: e}}, nil
			default:
				return nil, nil
			}
		},
		outgoing: func(_ context.Context, item lsp.CallHierarchyItem) ([]lsp.CallHierarchyOutgoingCall, error) {
			switch item.Name {
			case "A":
				return []lsp.CallHierarchyOutgoingCall{{To: c}}, nil
			case "C":
				return []lsp.CallHierarchyOutgoingCall{{To: b}}, nil
			default:
				return nil, nil
			}
		},
	}, tool.CodeIntelTraceRequest{
		Path:      "/repo/a.go",
		Line:      1,
		Character: 0,
		Mode:      tool.CodeIntelTraceModeGraph,
		Depth:     2,
		MaxNodes:  10,
	})
	if err != nil {
		t.Fatalf("traceFromServer() error = %v", err)
	}
	if len(result.Nodes) != 4 {
		t.Fatalf("nodes = %#v, want 4 unique nodes", result.Nodes)
	}
	if len(result.Edges) != 4 {
		t.Fatalf("edges = %#v, want 4 edges", result.Edges)
	}
	for _, node := range result.Nodes {
		if node.Name == "E" {
			t.Fatalf("nodes = %#v, want depth-3 node excluded", result.Nodes)
		}
	}
}

func TestTraceFromServerGraphTruncatesAtMaxNodes(t *testing.T) {
	root := lsp.CallHierarchyItem{Name: "A", Kind: 12, URI: "file:///repo/a.go", SelectionRange: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}}
	b := lsp.CallHierarchyItem{Name: "B", Kind: 12, URI: "file:///repo/b.go", SelectionRange: lsp.Range{Start: lsp.Position{Line: 1, Character: 0}}}
	c := lsp.CallHierarchyItem{Name: "C", Kind: 12, URI: "file:///repo/c.go", SelectionRange: lsp.Range{Start: lsp.Position{Line: 2, Character: 0}}}

	result, err := traceFromServer(context.Background(), fakeTraceServer{
		prepare: func(context.Context, string, int, int) ([]lsp.CallHierarchyItem, error) {
			return []lsp.CallHierarchyItem{root}, nil
		},
		incoming: func(context.Context, lsp.CallHierarchyItem) ([]lsp.CallHierarchyIncomingCall, error) {
			return nil, nil
		},
		outgoing: func(_ context.Context, item lsp.CallHierarchyItem) ([]lsp.CallHierarchyOutgoingCall, error) {
			if item.Name == "A" {
				return []lsp.CallHierarchyOutgoingCall{{To: b}, {To: c}}, nil
			}
			return nil, nil
		},
	}, tool.CodeIntelTraceRequest{
		Path:      "/repo/a.go",
		Line:      1,
		Character: 0,
		Mode:      tool.CodeIntelTraceModeGraph,
		Depth:     1,
		MaxNodes:  2,
	})
	if err != nil {
		t.Fatalf("traceFromServer() error = %v", err)
	}
	if !result.Truncated {
		t.Fatalf("result = %#v, want truncated=true", result)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("nodes = %#v, want max 2 nodes", result.Nodes)
	}
}
