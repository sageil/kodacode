package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestGitShowToolExecuteShowsCommitContext(t *testing.T) {
	ensureGitAvailable(t)

	root := t.TempDir()
	initGitRepo(t, root)
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "one\n")
	gitCommitAll(t, root, "init")
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "one\ntwo\n")
	gitCommitAll(t, root, "second")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewGitShowTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"rev":"HEAD"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, needle := range []string{
		"command: git show --no-color --no-ext-diff --format=fuller --unified=3 HEAD -- .",
		"revision: HEAD",
		"Commit:",
		"Test User <test@example.com>",
		"second",
		"diff --git a/tracked.txt b/tracked.txt",
		"+two",
	} {
		if !strings.Contains(result.Output, needle) {
			t.Fatalf("output missing %q\n%s", needle, result.Output)
		}
	}
}

func TestGitShowToolAcceptsRevisionAliases(t *testing.T) {
	for _, input := range []string{
		`{"revision":"HEAD"}`,
		`{"commit":"HEAD"}`,
		`{"ref":"HEAD"}`,
	} {
		parsed, err := parseGitShowInput(json.RawMessage(input))
		if err != nil {
			t.Fatalf("parseGitShowInput(%s) error = %v", input, err)
		}
		if parsed.Rev != "HEAD" {
			t.Fatalf("parseGitShowInput(%s).Rev = %q, want HEAD", input, parsed.Rev)
		}
	}
}

func TestGitShowToolExecuteScopesToNestedWorkspaceRoot(t *testing.T) {
	ensureGitAvailable(t)

	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)
	workspaceRoot := filepath.Join(repoRoot, "app")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeTestFile(t, filepath.Join(repoRoot, "outside.txt"), "one\n")
	writeTestFile(t, filepath.Join(workspaceRoot, "inside.txt"), "one\n")
	gitCommitAll(t, repoRoot, "init")
	writeTestFile(t, filepath.Join(repoRoot, "outside.txt"), "two\n")
	writeTestFile(t, filepath.Join(workspaceRoot, "inside.txt"), "two\n")
	gitCommitAll(t, repoRoot, "second")

	scope, err := workspace.New(workspaceRoot)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewGitShowTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"rev":"HEAD"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "inside.txt") {
		t.Fatalf("output = %q", result.Output)
	}
	if strings.Contains(result.Output, "outside.txt") {
		t.Fatalf("output leaked outside path:\n%s", result.Output)
	}
}

func TestGitShowToolExecuteReturnsStructuredErrorOutsideRepository(t *testing.T) {
	ensureGitAvailable(t)

	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewGitShowTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"rev":"HEAD"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Error, "workspace directory is not inside a git repository") {
		t.Fatalf("error = %q", result.Error)
	}
}
