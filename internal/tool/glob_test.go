package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestGlobTool_basic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewGlobTool()
	args := []byte(`{"pattern":"**/*.go","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "foo.go") {
		t.Fatalf("expected foo.go, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "bar.txt") {
		t.Fatalf("did not expect bar.txt, got: %s", res.Output)
	}
}

func TestGlobTool_noMatch(t *testing.T) {
	dir := t.TempDir()
	tl := tool.NewGlobTool()
	args := []byte(`{"pattern":"**/*.xyz","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "No files matched") {
		t.Fatalf("expected no match message, got: %s", res.Output)
	}
}
