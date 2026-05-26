package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestRenameSymbolToolDelegatesToCodeIntel(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	var captured CodeIntelRenameRequest
	result, err := NewRenameSymbolTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			rename: func(request CodeIntelRenameRequest) (CodeIntelMutationSummary, error) {
				captured = request
				return CodeIntelMutationSummary{
					Paths:     []string{filepath.Join(root, "service.go"), filepath.Join(root, "main.go")},
					TextEdits: 3,
				}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","line":12,"character":4,"new_name":"SessionStore"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.Line != 12 || captured.Character != 4 || captured.NewName != "SessionStore" || filepath.Base(captured.Path) != "main.go" {
		t.Fatalf("captured request = %#v", captured)
	}
	if !strings.Contains(result.Output, `Renamed symbol to "SessionStore" across 2 file(s) with 3 text edit(s).`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestRenameSymbolToolAcceptsStringPosition(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	var captured CodeIntelRenameRequest
	_, err = NewRenameSymbolTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			rename: func(request CodeIntelRenameRequest) (CodeIntelMutationSummary, error) {
				captured = request
				return CodeIntelMutationSummary{}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","line":"12","character":"4","new_name":"SessionStore"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.Line != 12 || captured.Character != 4 {
		t.Fatalf("captured request = %#v", captured)
	}
}

func TestRenameSymbolToolResolvesSymbolToCharacter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("const sessionStore = createStore()\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	var captured CodeIntelRenameRequest
	_, err = NewRenameSymbolTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			rename: func(request CodeIntelRenameRequest) (CodeIntelMutationSummary, error) {
				captured = request
				return CodeIntelMutationSummary{}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","line":1,"symbol":"sessionStore","new_name":"projectStore"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.Character != 6 || captured.NewName != "projectStore" {
		t.Fatalf("captured request = %#v", captured)
	}
}

func TestCodeActionToolDelegatesToCodeIntel(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	var captured CodeIntelCodeActionRequest
	result, err := NewCodeActionTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			codeAction: func(request CodeIntelCodeActionRequest) (CodeIntelCodeActionResult, error) {
				captured = request
				return CodeIntelCodeActionResult{
					Title: "Organize Imports",
					Summary: CodeIntelMutationSummary{
						Paths:     []string{filepath.Join(root, "main.go")},
						TextEdits: 1,
					},
				}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","start_line":1,"start_character":0,"end_line":1,"end_character":20,"title":null,"kind":"source.organizeImports","only_preferred":true}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.Kind != "source.organizeImports" || !captured.OnlyPreferred || captured.StartLine != 1 || captured.EndCharacter != 20 {
		t.Fatalf("captured request = %#v", captured)
	}
	if !strings.Contains(result.Output, `Applied code action "Organize Imports" across 1 file(s) with 1 text edit(s).`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestCodeActionToolAcceptsStringNumericAndBooleanArgs(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	var captured CodeIntelCodeActionRequest
	_, err = NewCodeActionTool().Execute(t.Context(), ExecutionContext{
		Workspace: scope,
		CodeIntelAPI: fakeCodeIntel{
			codeAction: func(request CodeIntelCodeActionRequest) (CodeIntelCodeActionResult, error) {
				captured = request
				return CodeIntelCodeActionResult{}, nil
			},
		},
	}, json.RawMessage(`{"path":"main.go","start_line":"1","start_character":"0","end_line":"1","end_character":"20","title":null,"kind":"source.organizeImports","only_preferred":"TRUE"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.StartLine != 1 || captured.EndCharacter != 20 || !captured.OnlyPreferred {
		t.Fatalf("captured request = %#v", captured)
	}
}
