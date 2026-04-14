package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/lsp"
)

type fakePositionLSPServer struct {
	name             string
	definitionCalls  []int
	referenceCalls   []int
	hoverCalls       []int
	definitionResult func(line, character int) ([]lsp.Location, error)
	referenceResult  func(line, character int) ([]lsp.Location, error)
	hoverResult      func(line, character int) (*lsp.HoverResult, error)
}

func (f *fakePositionLSPServer) Name() string {
	if f.name != "" {
		return f.name
	}
	return "fake-lsp"
}

func (f *fakePositionLSPServer) Definition(_ context.Context, _ string, line, character int) ([]lsp.Location, error) {
	f.definitionCalls = append(f.definitionCalls, character)
	if f.definitionResult != nil {
		return f.definitionResult(line, character)
	}
	return nil, nil
}

func (f *fakePositionLSPServer) References(_ context.Context, _ string, line, character int) ([]lsp.Location, error) {
	f.referenceCalls = append(f.referenceCalls, character)
	if f.referenceResult != nil {
		return f.referenceResult(line, character)
	}
	return nil, nil
}

func (f *fakePositionLSPServer) Hover(_ context.Context, _ string, line, character int) (*lsp.HoverResult, error) {
	f.hoverCalls = append(f.hoverCalls, character)
	if f.hoverResult != nil {
		return f.hoverResult(line, character)
	}
	return nil, nil
}

func TestExecutePositionLSPAction_FallsBackToNearestSymbolPosition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("callTarget()\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	definitionServer := &fakePositionLSPServer{
		definitionResult: func(line, character int) ([]lsp.Location, error) {
			if line == 0 && character == 9 {
				return []lsp.Location{{
					URI:   lsp.FileURI(path),
					Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 10}},
				}}, nil
			}
			return nil, nil
		},
	}
	res, err := executePositionLSPAction(t.Context(), "definition", definitionServer, path, path, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, path+":1:1") {
		t.Fatalf("unexpected definition output: %q", res.Output)
	}
	if len(definitionServer.definitionCalls) < 2 || !slices.Equal(definitionServer.definitionCalls[:2], []int{10, 9}) {
		t.Fatalf("unexpected definition calls: %v", definitionServer.definitionCalls)
	}

	referenceServer := &fakePositionLSPServer{
		referenceResult: func(line, character int) ([]lsp.Location, error) {
			if line == 0 && character == 9 {
				return []lsp.Location{{
					URI:   lsp.FileURI(path),
					Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 10}},
				}}, nil
			}
			return nil, nil
		},
	}
	res, err = executePositionLSPAction(t.Context(), "references", referenceServer, path, path, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, path+":1:1") {
		t.Fatalf("unexpected references output: %q", res.Output)
	}
	if len(referenceServer.referenceCalls) < 2 || !slices.Equal(referenceServer.referenceCalls[:2], []int{10, 9}) {
		t.Fatalf("unexpected reference calls: %v", referenceServer.referenceCalls)
	}

	hoverRaw, err := json.Marshal("hover info")
	if err != nil {
		t.Fatal(err)
	}
	inspectServer := &fakePositionLSPServer{
		hoverResult: func(line, character int) (*lsp.HoverResult, error) {
			if line == 0 && character == 9 {
				return &lsp.HoverResult{Contents: hoverRaw}, nil
			}
			return &lsp.HoverResult{}, nil
		},
	}
	res, err = executePositionLSPAction(t.Context(), "inspect", inspectServer, path, path, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "hover info" {
		t.Fatalf("unexpected inspect output: %q", res.Output)
	}
	if len(inspectServer.hoverCalls) < 2 || !slices.Equal(inspectServer.hoverCalls[:2], []int{10, 9}) {
		t.Fatalf("unexpected hover calls: %v", inspectServer.hoverCalls)
	}
}
