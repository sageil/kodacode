package tool_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	// Create an initial commit so log/show work.
	cmd := exec.Command("touch", "README")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "initial commit")
	return dir
}

func TestGitTool_status(t *testing.T) {
	dir := initGitRepo(t)
	tl := tool.NewGitTool()
	ectx := tool.ExecutionContext{WorkDir: dir}
	res, err := tl.Execute(t.Context(), ectx, []byte(`{"action":"status"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Metadata["exit_code"] != 0 {
		t.Fatalf("expected exit code 0, got %v: %s", res.Metadata["exit_code"], res.Output)
	}
}

func TestGitTool_log(t *testing.T) {
	dir := initGitRepo(t)
	tl := tool.NewGitTool()
	ectx := tool.ExecutionContext{WorkDir: dir}
	res, err := tl.Execute(t.Context(), ectx, []byte(`{"action":"log","limit":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "initial commit") {
		t.Fatalf("expected 'initial commit' in log output, got: %s", res.Output)
	}
}

func TestGitTool_invalidAction(t *testing.T) {
	tl := tool.NewGitTool()
	ectx := tool.ExecutionContext{}
	_, err := tl.Execute(t.Context(), ectx, []byte(`{"action":"rebase"}`))
	if err == nil {
		t.Fatal("expected error for unsupported action")
	}
	if !strings.Contains(err.Error(), "unsupported git action") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitTool_missingAction(t *testing.T) {
	tl := tool.NewGitTool()
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for missing action")
	}
}

func TestGitTool_diff(t *testing.T) {
	dir := initGitRepo(t)
	tl := tool.NewGitTool()
	ectx := tool.ExecutionContext{WorkDir: dir}
	res, err := tl.Execute(t.Context(), ectx, []byte(`{"action":"diff"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Metadata["exit_code"] != 0 {
		t.Fatalf("expected exit code 0, got %v", res.Metadata["exit_code"])
	}
}

func TestGitTool_branch(t *testing.T) {
	dir := initGitRepo(t)
	tl := tool.NewGitTool()
	ectx := tool.ExecutionContext{WorkDir: dir}
	res, err := tl.Execute(t.Context(), ectx, []byte(`{"action":"branch"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "main") && !strings.Contains(res.Output, "master") {
		t.Fatalf("expected branch name in output, got: %s", res.Output)
	}
}

func TestGitTool_blameRequiresArgs(t *testing.T) {
	tl := tool.NewGitTool()
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"action":"blame"}`))
	if err == nil {
		t.Fatal("expected error for blame without args")
	}
}

func TestGitTool_commitRequiresArgs(t *testing.T) {
	tl := tool.NewGitTool()
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"action":"commit"}`))
	if err == nil {
		t.Fatal("expected error for commit without args")
	}
}

func TestGitTool_ReadOnlyIsFalse(t *testing.T) {
	tl := tool.NewGitTool()
	if tl.ReadOnly {
		t.Error("git tool should not be marked ReadOnly — it has mutating actions")
	}
}

func TestIsDestructiveGitCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"git push --force origin main", true},
		{"git push -f origin main", true},
		{"git reset --hard HEAD~1", true},
		{"git clean -fd", true},
		{"git clean -xfd", true},
		{"git checkout -- .", true},
		{"git checkout -- src/file.go", true},
		{"git checkout .", true},
		{"git restore .", true},
		{"git restore -- .", true},
		{"git stash drop", true},
		{"git stash drop stash@{0}", true},
		{"git branch -D feature-branch", true},
		{"git push origin main", false},
		{"git reset HEAD file.go", false},
		{"git checkout feature-branch", false},
		{"git stash", false},
		{"git stash pop", false},
		{"git stash list", false},
		{"git branch -d feature-branch", false},
		{"git restore --staged file.go", false},
		{"git status", false},
		{"git commit -m 'test'", false},
		{"git log --oneline", false},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			if got := tool.IsDestructiveGitCommand(tt.cmd); got != tt.want {
				t.Errorf("IsDestructiveGitCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

