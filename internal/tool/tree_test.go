package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestTreeTool_basic(t *testing.T) {
	dir := t.TempDir()
	// Create structure:
	// src/
	//   auth/
	//     middleware.go
	//   main.go
	_ = os.MkdirAll(filepath.Join(dir, "src", "auth"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "src", "auth", "middleware.go"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte(""), 0o644)

	tl := tool.NewTreeTool()
	args := []byte(`{"path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "src/") {
		t.Fatalf("expected src/ in output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "auth/") {
		t.Fatalf("expected auth/ in output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "middleware.go") {
		t.Fatalf("expected middleware.go in output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "main.go") {
		t.Fatalf("expected main.go in output, got: %s", res.Output)
	}
	// Check tree connectors.
	if !strings.Contains(res.Output, "├── ") && !strings.Contains(res.Output, "└── ") {
		t.Fatalf("expected tree connectors in output, got: %s", res.Output)
	}
	// Check summary line.
	if !strings.Contains(res.Output, "directories") || !strings.Contains(res.Output, "files") {
		t.Fatalf("expected summary line, got: %s", res.Output)
	}
}

func TestTreeTool_depthLimit(t *testing.T) {
	dir := t.TempDir()
	// Create a deep structure: a/b/c/d/deep.txt
	_ = os.MkdirAll(filepath.Join(dir, "a", "b", "c", "d"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "a", "b", "c", "d", "deep.txt"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "a", "shallow.txt"), []byte(""), 0o644)

	tl := tool.NewTreeTool()
	args := []byte(`{"path":"` + dir + `","depth":2}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	// a/shallow.txt is at depth 2 (a=1, shallow.txt=2), should be included.
	if !strings.Contains(res.Output, "shallow.txt") {
		t.Fatalf("expected shallow.txt at depth 2, got: %s", res.Output)
	}
	// a/b/c/d/deep.txt is at depth 5, should be excluded.
	if strings.Contains(res.Output, "deep.txt") {
		t.Fatalf("did not expect deep.txt beyond depth 2, got: %s", res.Output)
	}
}

func TestTreeTool_hiddenFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "visible.txt"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".hidden"), []byte(""), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, ".secret"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".secret", "data.txt"), []byte(""), 0o644)

	tl := tool.NewTreeTool()

	// Without showHidden: hidden files/dirs should be excluded.
	args := []byte(`{"path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Output, ".hidden") {
		t.Fatalf("did not expect .hidden, got: %s", res.Output)
	}
	if strings.Contains(res.Output, ".secret") {
		t.Fatalf("did not expect .secret, got: %s", res.Output)
	}

	// With showHidden: hidden files/dirs should be included.
	args = []byte(`{"path":"` + dir + `","showHidden":true}`)
	res, err = tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, ".hidden") {
		t.Fatalf("expected .hidden with showHidden, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, ".secret") {
		t.Fatalf("expected .secret with showHidden, got: %s", res.Output)
	}
}

func TestTreeTool_includeFilter(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "src", "style.css"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte(""), 0o644)

	tl := tool.NewTreeTool()
	args := []byte(`{"path":"` + dir + `","include":"*.go"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "main.go") {
		t.Fatalf("expected main.go with include filter, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "style.css") {
		t.Fatalf("did not expect style.css with include filter, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "readme.txt") {
		t.Fatalf("did not expect readme.txt with include filter, got: %s", res.Output)
	}
	// src/ directory should still be shown since it contains a match.
	if !strings.Contains(res.Output, "src/") {
		t.Fatalf("expected src/ directory to be shown, got: %s", res.Output)
	}
}

func TestTreeTool_emptyDir(t *testing.T) {
	dir := t.TempDir()
	tl := tool.NewTreeTool()
	args := []byte(`{"path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "0 directories, 0 files") {
		t.Fatalf("expected empty summary, got: %s", res.Output)
	}
}
