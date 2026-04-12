package tool_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestGrepTool_basic(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("func main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewGrepTool()
	args := []byte(`{"pattern":"func main","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "main.go") {
		t.Fatalf("expected main.go in output, got: %s", res.Output)
	}
}

func TestGrepTool_noMatch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewGrepTool()
	args := []byte(`{"pattern":"XYZNOTFOUND","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "No matches") {
		t.Fatalf("expected no matches, got: %s", res.Output)
	}
}
