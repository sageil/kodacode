package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

type fakeCodeIntel struct {
	definition func(string, int, int) ([]CodeIntelLocation, error)
	diagnostic func([]string) ([]CodeIntelFileDiagnostics, error)
	symbols    func(string) (CodeIntelSymbolsResult, error)
	trace      func(CodeIntelTraceRequest) (CodeIntelTraceResult, error)
	refs       func(CodeIntelRefsRequest) (CodeIntelRefsResult, error)
	rename     func(CodeIntelRenameRequest) (CodeIntelMutationSummary, error)
	codeAction func(CodeIntelCodeActionRequest) (CodeIntelCodeActionResult, error)
}

func (f fakeCodeIntel) Definition(_ context.Context, filePath string, line, character int) ([]CodeIntelLocation, error) {
	return f.definition(filePath, line, character)
}

func (f fakeCodeIntel) Diagnostics(_ context.Context, filePaths []string) ([]CodeIntelFileDiagnostics, error) {
	return f.diagnostic(filePaths)
}

func (f fakeCodeIntel) Symbols(_ context.Context, query string) (CodeIntelSymbolsResult, error) {
	return f.symbols(query)
}

func (f fakeCodeIntel) Trace(_ context.Context, request CodeIntelTraceRequest) (CodeIntelTraceResult, error) {
	return f.trace(request)
}

func (f fakeCodeIntel) Refs(_ context.Context, request CodeIntelRefsRequest) (CodeIntelRefsResult, error) {
	return f.refs(request)
}

func (f fakeCodeIntel) RenameSymbol(_ context.Context, request CodeIntelRenameRequest) (CodeIntelMutationSummary, error) {
	return f.rename(request)
}

func (f fakeCodeIntel) ApplyCodeAction(_ context.Context, request CodeIntelCodeActionRequest) (CodeIntelCodeActionResult, error) {
	return f.codeAction(request)
}

func TestDefinitionToolFormatsDefinitionResult(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("targetValue()\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	tl := NewDefinitionTool()
	res, err := tl.Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			definition: func(filePath string, line, character int) ([]CodeIntelLocation, error) {
				if filepath.Base(filePath) != "main.go" || line != 0 || character < 0 {
					t.Fatalf("unexpected definition request: %s %d %d", filePath, line, character)
				}
				return []CodeIntelLocation{{
					Path:      filepath.Join(root, "pkg", "service.go"),
					Line:      42,
					Character: 7,
				}}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","line":1,"character":0}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(res.Output, "Definition for main.go:1:0") {
		t.Fatalf("output = %q", res.Output)
	}
	if !strings.Contains(res.Output, "service.go:42:7") {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestDefinitionToolAcceptsStringPosition(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("targetValue()\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	tl := NewDefinitionTool()
	_, err = tl.Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			definition: func(filePath string, line, character int) ([]CodeIntelLocation, error) {
				if line != 0 || character < 0 {
					t.Fatalf("unexpected definition request: %s %d %d", filePath, line, character)
				}
				return nil, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","line":"1","character":"0"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestDefinitionToolResolvesSymbolToCharacter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("const targetValue = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	var capturedCharacter int
	_, err = NewDefinitionTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			definition: func(filePath string, line, character int) ([]CodeIntelLocation, error) {
				capturedCharacter = character
				return []CodeIntelLocation{{Path: filePath, Line: line + 1, Character: character}}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","line":1,"symbol":"targetValue"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if capturedCharacter != 6 {
		t.Fatalf("capturedCharacter = %d, want 6", capturedCharacter)
	}
}

func TestParseDiagnosticsInputAcceptsStringifiedPathsArray(t *testing.T) {
	input, err := parseDiagnosticsInput(json.RawMessage(`{"paths":"[\"main.go\",\"pkg/service.go\"]"}`))
	if err != nil {
		t.Fatalf("parseDiagnosticsInput() error = %v", err)
	}
	if got, want := len(input.Paths), 2; got != want {
		t.Fatalf("len(paths) = %d, want %d", got, want)
	}
	if input.Paths[0] != "main.go" || input.Paths[1] != "pkg/service.go" {
		t.Fatalf("paths = %#v", input.Paths)
	}
}

func TestParseDiagnosticsInputAcceptsSinglePathAlias(t *testing.T) {
	input, err := parseDiagnosticsInput(json.RawMessage(`{"path":"main.go"}`))
	if err != nil {
		t.Fatalf("parseDiagnosticsInput() error = %v", err)
	}
	if len(input.Paths) != 1 || input.Paths[0] != "main.go" {
		t.Fatalf("paths = %#v, want single path alias", input.Paths)
	}
}

func TestDiagnosticsToolFiltersDuplicateAndOutOfBoundsDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	tl := NewDiagnosticsTool()
	res, err := tl.Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			diagnostic: func(filePaths []string) ([]CodeIntelFileDiagnostics, error) {
				return []CodeIntelFileDiagnostics{{
					Path: filePaths[0],
					Diagnostics: []CodeIntelDiagnostic{
						{Line: 1, Character: 0, Severity: "error", Message: "missing import", Source: "gopls"},
						{Line: 1, Character: 0, Severity: "error", Message: "missing import", Source: "gopls"},
						{Line: 20, Character: 0, Severity: "error", Message: "stale phantom", Source: "gopls"},
					},
				}}, nil
			},
		},
	}, json.RawMessage(`{"paths":["main.go"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(res.Output, "stale phantom") {
		t.Fatalf("output should filter out-of-bounds diagnostics:\n%s", res.Output)
	}
	if strings.Count(res.Output, "missing import") != 1 {
		t.Fatalf("output should dedupe repeated diagnostics:\n%s", res.Output)
	}
}

func TestDiagnosticsToolFormatsPerFileFailuresInline(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "util.go"), []byte("package main\nconst value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	tl := NewDiagnosticsTool()
	res, err := tl.Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			diagnostic: func(filePaths []string) ([]CodeIntelFileDiagnostics, error) {
				return []CodeIntelFileDiagnostics{
					{
						Path:  filePaths[0],
						Error: "timed out waiting for diagnostics after 15s",
					},
					{
						Path: filePaths[1],
						Diagnostics: []CodeIntelDiagnostic{
							{Line: 2, Character: 6, Severity: "warning", Message: "unused value", Source: "gopls"},
						},
					},
				}, nil
			},
		},
	}, json.RawMessage(`{"paths":["main.go","util.go"]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(res.Output, "main.go") || !strings.Contains(res.Output, "diagnostics unavailable: timed out waiting for diagnostics after 15s") {
		t.Fatalf("output should include inline failure details:\n%s", res.Output)
	}
	if !strings.Contains(res.Output, "util.go") || !strings.Contains(res.Output, "unused value") {
		t.Fatalf("output should preserve successful diagnostics:\n%s", res.Output)
	}
}

func TestDiagnosticsToolRejectsDirectoryInput(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	tl := NewDiagnosticsTool()
	_, err = tl.Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			diagnostic: func([]string) ([]CodeIntelFileDiagnostics, error) {
				t.Fatal("Diagnostics() should not be called for directory input")
				return nil, nil
			},
		},
	}, json.RawMessage(`{"paths":["src"]}`))
	if err == nil {
		t.Fatal("Execute() error = nil, want invalid arguments")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	if !strings.Contains(err.Error(), `path "src" is a directory; pass concrete source file paths, not directories`) {
		t.Fatalf("err.Error() = %q", err.Error())
	}
}

func TestDiagnosticsToolRejectsMissingFileInput(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	tl := NewDiagnosticsTool()
	_, err = tl.Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			diagnostic: func([]string) ([]CodeIntelFileDiagnostics, error) {
				t.Fatal("Diagnostics() should not be called for missing file input")
				return nil, nil
			},
		},
	}, json.RawMessage(`{"paths":["missing.go"]}`))
	if err == nil {
		t.Fatal("Execute() error = nil, want invalid arguments")
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	if !strings.Contains(err.Error(), `path "missing.go" does not exist; pass a concrete source file path`) {
		t.Fatalf("err.Error() = %q", err.Error())
	}
}

func TestSymbolsToolFormatsSymbolMatches(t *testing.T) {
	tl := NewSymbolsTool()
	res, err := tl.Execute(t.Context(), ExecutionContext{
		CodeIntelAPI: fakeCodeIntel{
			symbols: func(query string) (CodeIntelSymbolsResult, error) {
				if query != "SessionService" {
					t.Fatalf("query = %q", query)
				}
				return CodeIntelSymbolsResult{
					Symbols: []CodeIntelSymbol{{
						Name: "SessionService",
						Kind: "Struct",
						Path: "internal/app/session.go",
						Line: 18,
					}},
				}, nil
			},
		},
	}, json.RawMessage(`{"query":"SessionService"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(res.Output, `Symbols matching "SessionService"`) {
		t.Fatalf("output = %q", res.Output)
	}
	if !strings.Contains(res.Output, "SessionService [Struct] internal/app/session.go:18") {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestSymbolsToolDefinitionRequiresWorkspace(t *testing.T) {
	definition := NewSymbolsTool().Definition()
	if !definition.RequiresWorkspace {
		t.Fatal("RequiresWorkspace = false, want true")
	}
}

func TestSymbolsToolFormatsUnavailableNotice(t *testing.T) {
	tl := NewSymbolsTool()
	res, err := tl.Execute(t.Context(), ExecutionContext{
		CodeIntelAPI: fakeCodeIntel{
			symbols: func(query string) (CodeIntelSymbolsResult, error) {
				if query != "SessionService" {
					t.Fatalf("query = %q", query)
				}
				return CodeIntelSymbolsResult{
					Notice: &CodeIntelNotice{
						Kind:    CodeIntelNoticeKindUnavailable,
						Message: "gopls not found. Install: go install golang.org/x/tools/gopls@latest",
					},
				}, nil
			},
		},
	}, json.RawMessage(`{"query":"SessionService"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(res.Output, "symbols unavailable: gopls not found") {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestTraceToolFormatsCallerResults(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc run() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	tl := NewTraceTool()
	res, err := tl.Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			trace: func(request CodeIntelTraceRequest) (CodeIntelTraceResult, error) {
				if filepath.Base(request.Path) != "main.go" || request.Mode != CodeIntelTraceModeCallers {
					t.Fatalf("unexpected trace request: %#v", request)
				}
				return CodeIntelTraceResult{
					Supported: true,
					Found:     true,
					RootID:    "root",
					Nodes: []CodeIntelTraceNode{
						{ID: "root", Name: "run", Kind: "Function", Path: request.Path, Line: 2, Character: 5},
						{ID: "caller", Name: "main", Kind: "Function", Path: filepath.Join(root, "entry.go"), Line: 8, Character: 1},
					},
					Edges: []CodeIntelTraceEdge{{FromID: "caller", ToID: "root"}},
				}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","line":2,"character":5,"mode":"callers"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(res.Output, "Callers of run [Function]") {
		t.Fatalf("output = %q", res.Output)
	}
	if !strings.Contains(res.Output, "main [Function]") {
		t.Fatalf("output = %q", res.Output)
	}
	var structured CodeIntelTraceResult
	if err := json.Unmarshal(res.StructuredResult, &structured); err != nil {
		t.Fatalf("Unmarshal(StructuredResult) error = %v", err)
	}
	if !structured.Supported || !structured.Found || structured.RootID != "root" || len(structured.Nodes) != 2 || len(structured.Edges) != 1 {
		t.Fatalf("structured trace result = %#v", structured)
	}
	if structured.Nodes[0].Name != "run" || filepath.Base(structured.Nodes[0].Path) != "main.go" {
		t.Fatalf("structured trace result = %#v", structured)
	}
	if structured.Nodes[1].Name != "main" || filepath.Base(structured.Nodes[1].Path) != "entry.go" {
		t.Fatalf("structured trace result = %#v", structured)
	}
	if structured.Edges[0].FromID != "caller" || structured.Edges[0].ToID != "root" {
		t.Fatalf("structured trace result = %#v", structured)
	}
}

func TestTraceToolResolvesSymbolToCharacter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("function runTask() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	var captured CodeIntelTraceRequest
	_, err = NewTraceTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			trace: func(request CodeIntelTraceRequest) (CodeIntelTraceResult, error) {
				captured = request
				return CodeIntelTraceResult{}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","line":1,"symbol":"runTask","mode":"callers"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.Character != 9 || captured.Mode != CodeIntelTraceModeCallers {
		t.Fatalf("captured request = %#v", captured)
	}
}

func TestTraceToolFormatsUnsupportedResult(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc run() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	res, err := NewTraceTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			trace: func(CodeIntelTraceRequest) (CodeIntelTraceResult, error) {
				return CodeIntelTraceResult{}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","line":2,"character":5,"mode":"graph"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(res.Output, "trace unsupported: this file type or language server does not support call hierarchy") {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestRefsToolFormatsWriterResults(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "store.ts"), []byte("const value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	res, err := NewRefsTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			refs: func(request CodeIntelRefsRequest) (CodeIntelRefsResult, error) {
				if request.Mode != CodeIntelRefsModeWriters {
					t.Fatalf("unexpected refs request: %#v", request)
				}
				return CodeIntelRefsResult{
					Supported: true,
					Found:     true,
					Target: CodeIntelTraceNode{
						Name:      "value",
						Kind:      "Variable",
						Path:      request.Path,
						Line:      1,
						Character: 6,
					},
					References: []CodeIntelReference{{
						Kind:      CodeIntelReferenceKindWrite,
						Path:      filepath.Join(root, "store.ts"),
						Line:      8,
						Character: 2,
						Snippet:   "state.value = nextValue",
					}},
					ClassificationSupported: true,
				}, nil
			},
		},
	}, json.RawMessage(`{"path":"store.ts","line":1,"character":6,"mode":"writers"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(res.Output, "Writers of value [Variable]") {
		t.Fatalf("output = %q", res.Output)
	}
	if !strings.Contains(res.Output, "state.value = nextValue") {
		t.Fatalf("output = %q", res.Output)
	}
	var structured CodeIntelRefsResult
	if err := json.Unmarshal(res.StructuredResult, &structured); err != nil {
		t.Fatalf("Unmarshal(StructuredResult) error = %v", err)
	}
	if !structured.Supported || !structured.Found || !structured.ClassificationSupported || len(structured.References) != 1 {
		t.Fatalf("structured refs result = %#v", structured)
	}
	if structured.Target.Name != "value" || filepath.Base(structured.Target.Path) != "store.ts" {
		t.Fatalf("structured refs result = %#v", structured)
	}
	if structured.References[0].Kind != CodeIntelReferenceKindWrite || filepath.Base(structured.References[0].Path) != "store.ts" || structured.References[0].Snippet != "state.value = nextValue" {
		t.Fatalf("structured refs result = %#v", structured)
	}
}

func TestRefsToolFormatsUnsupportedResult(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "store.ts"), []byte("const value = 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	res, err := NewRefsTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			refs: func(CodeIntelRefsRequest) (CodeIntelRefsResult, error) {
				return CodeIntelRefsResult{}, nil
			},
		},
	}, json.RawMessage(`{"path":"store.ts","line":1,"character":6}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(res.Output, "refs unsupported: this file type or language server does not support reference lookup") {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestRefsToolResolvesSymbolToCharacter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "store.ts"), []byte("state.value = nextValue\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	var captured CodeIntelRefsRequest
	_, err = NewRefsTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			refs: func(request CodeIntelRefsRequest) (CodeIntelRefsResult, error) {
				captured = request
				return CodeIntelRefsResult{}, nil
			},
		},
	}, json.RawMessage(`{"path":"store.ts","line":1,"symbol":"value","mode":"writers"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.Character != 6 || captured.Mode != CodeIntelRefsModeWriters {
		t.Fatalf("captured request = %#v", captured)
	}
}

func TestDefinitionToolRequiresCodeIntel(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewDefinitionTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
	}, json.RawMessage(`{"path":"main.go","line":1,"character":0}`))
	if err != ErrCodeIntelRequired {
		t.Fatalf("error = %v, want %v", err, ErrCodeIntelRequired)
	}
}
