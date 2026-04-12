package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenTool_FileNotFound(t *testing.T) {
	tool := NewOpenTool()
	args, _ := json.Marshal(openArgs{FilePath: "/nonexistent/path/to/file.go"})
	_, err := tool.Execute(context.Background(), ExecutionContext{}, args)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestOpenTool_MissingFilePath(t *testing.T) {
	tool := NewOpenTool()
	args, _ := json.Marshal(openArgs{})
	_, err := tool.Execute(context.Background(), ExecutionContext{}, args)
	if err == nil {
		t.Fatal("expected error for missing filePath")
	}
}

func TestOpenTool_InvalidJSON(t *testing.T) {
	tool := NewOpenTool()
	_, err := tool.Execute(context.Background(), ExecutionContext{}, []byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestResolveEditor_Requested(t *testing.T) {
	got := resolveEditor("subl")
	if got != "subl" {
		t.Fatalf("resolveEditor(%q) = %q, want %q", "subl", got, "subl")
	}
}

func TestResolveEditor_EnvVar(t *testing.T) {
	t.Setenv("EDITOR", "myeditor")
	got := resolveEditor("")
	if got != "myeditor" {
		t.Fatalf("resolveEditor(\"\") with EDITOR=myeditor = %q, want %q", got, "myeditor")
	}
}

func TestResolveEditor_NoEditorAvailable(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", "/nonexistent")
	got := resolveEditor("")
	if got != "" {
		t.Fatalf("resolveEditor(\"\") with no editors = %q, want empty", got)
	}
}

func TestBuildEditorArgs_Code(t *testing.T) {
	args := buildEditorArgs("code", "/tmp/file.go", 42)
	if len(args) != 2 || args[0] != "--goto" || args[1] != "/tmp/file.go:42" {
		t.Fatalf("code args = %v, want [--goto /tmp/file.go:42]", args)
	}
}

func TestBuildEditorArgs_CodeInsiders(t *testing.T) {
	args := buildEditorArgs("code-insiders", "/tmp/file.go", 10)
	if len(args) != 2 || args[0] != "--goto" || args[1] != "/tmp/file.go:10" {
		t.Fatalf("code-insiders args = %v, want [--goto /tmp/file.go:10]", args)
	}
}

func TestBuildEditorArgs_Vim(t *testing.T) {
	args := buildEditorArgs("vim", "/tmp/file.go", 42)
	if len(args) != 2 || args[0] != "+42" || args[1] != "/tmp/file.go" {
		t.Fatalf("vim args = %v, want [+42 /tmp/file.go]", args)
	}
}

func TestBuildEditorArgs_Nvim(t *testing.T) {
	args := buildEditorArgs("nvim", "/tmp/file.go", 5)
	if len(args) != 2 || args[0] != "+5" || args[1] != "/tmp/file.go" {
		t.Fatalf("nvim args = %v, want [+5 /tmp/file.go]", args)
	}
}

func TestBuildEditorArgs_Nano(t *testing.T) {
	args := buildEditorArgs("nano", "/tmp/file.go", 10)
	if len(args) != 2 || args[0] != "+10" || args[1] != "/tmp/file.go" {
		t.Fatalf("nano args = %v, want [+10 /tmp/file.go]", args)
	}
}

func TestBuildEditorArgs_Emacs(t *testing.T) {
	args := buildEditorArgs("emacs", "/tmp/file.go", 99)
	if len(args) != 2 || args[0] != "+99" || args[1] != "/tmp/file.go" {
		t.Fatalf("emacs args = %v, want [+99 /tmp/file.go]", args)
	}
}

func TestBuildEditorArgs_Subl(t *testing.T) {
	args := buildEditorArgs("subl", "/tmp/file.go", 7)
	if len(args) != 1 || args[0] != "/tmp/file.go:7" {
		t.Fatalf("subl args = %v, want [/tmp/file.go:7]", args)
	}
}

func TestBuildEditorArgs_Unknown(t *testing.T) {
	args := buildEditorArgs("myeditor", "/tmp/file.go", 42)
	if len(args) != 1 || args[0] != "/tmp/file.go" {
		t.Fatalf("unknown editor args = %v, want [/tmp/file.go]", args)
	}
}

func TestBuildEditorArgs_ZeroLine(t *testing.T) {
	args := buildEditorArgs("vim", "/tmp/file.go", 0)
	if args[0] != "+1" {
		t.Fatalf("zero line should default to +1, got %q", args[0])
	}
}

func TestOpenTool_TerminalEditor(t *testing.T) {
	// Create a temp file so the file-exists check passes.
	tmp := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmp, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewOpenTool()
	args, _ := json.Marshal(openArgs{FilePath: tmp, Line: 42, Editor: "vim"})
	result, err := tool.Execute(context.Background(), ExecutionContext{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Run: vim +42 "+tmp {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestOpenTool_UnknownEditorReturnsManualCommand(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmp, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewOpenTool()
	args, _ := json.Marshal(openArgs{FilePath: tmp, Line: 7, Editor: "bash"})
	result, err := tool.Execute(context.Background(), ExecutionContext{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Run: bash "+tmp {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if got := result.Metadata["editor"]; got != "bash" {
		t.Fatalf("editor metadata = %v, want bash", got)
	}
}

func TestOpenTool_NoEditorFound(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmp, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("EDITOR", "")
	t.Setenv("PATH", "/nonexistent")

	tool := NewOpenTool()
	args, _ := json.Marshal(openArgs{FilePath: tmp})
	_, err := tool.Execute(context.Background(), ExecutionContext{}, args)
	if err == nil {
		t.Fatal("expected error when no editor is available")
	}
}
