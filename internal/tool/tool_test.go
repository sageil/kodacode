package tool

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestExecutionContextResolvePathRequiresWorkspace(t *testing.T) {
	_, err := (ExecutionContext{}).ResolvePath(workspace.AccessRead, "app.go")
	if !errors.Is(err, ErrWorkspaceRequired) {
		t.Fatalf("ResolvePath() error = %v, want ErrWorkspaceRequired", err)
	}
}

func TestExecutionContextResolvePathUsesWorkspaceScope(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	got, err := (ExecutionContext{Workspace: scope}).ResolvePath(workspace.AccessRead, "app.go")
	if err != nil {
		t.Fatalf("ResolvePath() error = %v", err)
	}
	if got.ResolvedPath != filepath.Join(scope.Root(), "app.go") {
		t.Fatalf("resolved path = %q", got.ResolvedPath)
	}
}

func TestExecutionContextResolvePathResolvesExternalPaths(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	external := filepath.Join(t.TempDir(), "notes.txt")
	got, err := (ExecutionContext{Workspace: scope}).ResolvePath(workspace.AccessRead, external)
	if err != nil {
		t.Fatalf("ResolvePath() error = %v", err)
	}
	resolvedDir, err := filepath.EvalSymlinks(filepath.Dir(external))
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	want := filepath.Join(resolvedDir, filepath.Base(external))
	if got.ResolvedPath != want {
		t.Fatalf("resolved path = %q, want %q", got.ResolvedPath, want)
	}
}
