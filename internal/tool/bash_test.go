package tool_test

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestBashTool_echo(t *testing.T) {
	tl := tool.NewBashTool()
	args := []byte(`{"command":"echo hello","description":"echo test","purpose":"diagnostic"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Fatalf("expected 'hello' in output, got: %s", res.Output)
	}
	if res.Metadata["purpose"] != "diagnostic" {
		t.Fatalf("expected purpose diagnostic, got: %v", res.Metadata["purpose"])
	}
}

func TestBashTool_exitCode(t *testing.T) {
	tl := tool.NewBashTool()
	args := []byte(`{"command":"exit 42","description":"exit test","purpose":"verification"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Metadata["exit_code"] != 42 {
		t.Fatalf("expected exit code 42, got: %v", res.Metadata["exit_code"])
	}
}

func TestBashTool_missingCommand(t *testing.T) {
	tl := tool.NewBashTool()
	args := []byte(`{"description":"no command","purpose":"other"}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestBashTool_missingPurpose(t *testing.T) {
	tl := tool.NewBashTool()
	args := []byte(`{"command":"echo hello","description":"echo test"}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err == nil {
		t.Fatal("expected error for missing purpose")
	}
}

func TestIsReadOnlyBashCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"git status --porcelain", true},
		{"git log --oneline -5", true},
		{"git rev-parse --abbrev-ref HEAD", true},
		{"git status && git rev-parse --abbrev-ref HEAD", true},
		{"ls -la", true},
		{"echo hello", true},
		{"pwd", true},
		{"npm install", false},
		{"go build ./...", false},
		{"touch file.txt", false},
		{"git status && rm -rf .", false},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			if got := tool.IsReadOnlyBashCommand(tt.cmd); got != tt.want {
				t.Errorf("IsReadOnlyBashCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestBashHasMutatingGitCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"git status", false},
		{"git status && git diff --stat", false},
		{"FOO=1 git status", false},
		{"command git log --oneline", false},
		{"git commit -m test", true},
		{"git merge feature", true},
		{"git rebase main", true},
		{"env GIT_TRACE=1 git push --force", true},
		{"echo hello && git checkout -- file.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			if got := tool.BashHasMutatingGitCommand(tt.cmd); got != tt.want {
				t.Errorf("BashHasMutatingGitCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
