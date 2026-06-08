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

func TestGitDiffToolExecuteShowsWorkingTreeDiff(t *testing.T) {
	ensureGitAvailable(t)

	root := t.TempDir()
	initGitRepo(t, root)
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "one\n")
	gitCommitAll(t, root, "init")
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "one\ntwo\n")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewGitDiffTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"staged":false}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, needle := range []string{
		"command: git diff --no-color --no-ext-diff --unified=3 -- .",
		"scope: working",
		"diff --git a/tracked.txt b/tracked.txt",
		"+two",
	} {
		if !strings.Contains(result.Output, needle) {
			t.Fatalf("output missing %q\n%s", needle, result.Output)
		}
	}
}

func TestGitDiffToolExecuteShowsStagedDiff(t *testing.T) {
	ensureGitAvailable(t)

	root := t.TempDir()
	initGitRepo(t, root)
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "one\n")
	gitCommitAll(t, root, "init")
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "one\ntwo\n")
	runGitTest(t, root, "add", "tracked.txt")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewGitDiffTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"staged":true}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, needle := range []string{
		"command: git diff --no-color --no-ext-diff --unified=3 --cached -- .",
		"scope: staged",
		"diff --git a/tracked.txt b/tracked.txt",
		"+two",
	} {
		if !strings.Contains(result.Output, needle) {
			t.Fatalf("output missing %q\n%s", needle, result.Output)
		}
	}
}

func TestGitDiffToolExecuteAcceptsCaseInsensitiveStringStaged(t *testing.T) {
	ensureGitAvailable(t)

	root := t.TempDir()
	initGitRepo(t, root)
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "one\n")
	gitCommitAll(t, root, "init")
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "one\ntwo\n")
	runGitTest(t, root, "add", "tracked.txt")

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewGitDiffTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"staged":"TRUE"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "scope: staged") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestGitDiffToolExecuteScopesToNestedWorkspaceRoot(t *testing.T) {
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

	scope, err := workspace.New(workspaceRoot)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewGitDiffTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"staged":false}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "inside.txt") {
		t.Fatalf("output = %q", result.Output)
	}
	if strings.Contains(result.Output, "outside.txt") {
		t.Fatalf("output leaked outside diff:\n%s", result.Output)
	}
}

func TestGitDiffToolExecuteCapsLargeDiffOutput(t *testing.T) {
	ensureGitAvailable(t)

	root := t.TempDir()
	initGitRepo(t, root)
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "one\n")
	gitCommitAll(t, root, "init")
	var builder strings.Builder
	builder.WriteString("one\n")
	for i := 0; i < gitDiffRenderedOutputLimit; i++ {
		builder.WriteString("changed line that should make the diff large\n")
	}
	builder.WriteString("tail marker should not be rendered\n")
	writeTestFile(t, filepath.Join(root, "tracked.txt"), builder.String())

	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewGitDiffTool().Execute(context.Background(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"staged":false}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "[diff output capped; use git_status and targeted file reads for large changes]") {
		t.Fatalf("output missing cap guidance:\n%s", result.Output)
	}
	if strings.Contains(result.Output, "tail marker should not be rendered") {
		t.Fatalf("output included uncapped tail:\n%s", result.Output)
	}
	if len(result.Output) > gitDiffRenderedOutputLimit+512 {
		t.Fatalf("output length = %d, want capped near %d", len(result.Output), gitDiffRenderedOutputLimit)
	}
}
