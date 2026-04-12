package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestBulkReadTool_matchesGoFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "package main\nfunc A() {}\n")
	writeFile(t, filepath.Join(dir, "b.go"), "package main\nfunc B() {}\n")
	writeFile(t, filepath.Join(dir, "c.txt"), "not a go file\n")

	tl := tool.NewBulkReadTool()
	args := []byte(`{"pattern":"*.go","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: dir}, args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(res.Output, "a.go") {
		t.Errorf("expected a.go in output")
	}
	if !strings.Contains(res.Output, "b.go") {
		t.Errorf("expected b.go in output")
	}
	if strings.Contains(res.Output, "c.txt") {
		t.Errorf("did not expect c.txt in output")
	}
	if res.Metadata["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", res.Metadata["count"])
	}
}

func TestBulkReadTool_noMatch(t *testing.T) {
	dir := t.TempDir()
	tl := tool.NewBulkReadTool()
	args := []byte(`{"pattern":"*.xyz","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: dir}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "No readable files") {
		t.Errorf("expected no-match message, got: %s", res.Output)
	}
}

func TestBulkReadTool_maxFilesLimit(t *testing.T) {
	dir := t.TempDir()
	for i := range 10 {
		writeFile(t, filepath.Join(dir, strings.Repeat("x", i+1)+".go"), "package main\n")
	}

	tl := tool.NewBulkReadTool()
	args := []byte(`{"pattern":"*.go","path":"` + dir + `","maxFiles":3}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: dir}, args)
	if err != nil {
		t.Fatal(err)
	}

	if res.Metadata["count"].(int) > 3 {
		t.Errorf("expected at most 3 files, got %v", res.Metadata["count"])
	}
	if !res.Metadata["truncated"].(bool) {
		t.Errorf("expected truncated=true")
	}
}

func TestBulkReadTool_skipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "data.zip"), "PK\x03\x04fake zip\n")

	tl := tool.NewBulkReadTool()
	args := []byte(`{"pattern":"*","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: dir}, args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Output, "data.zip") {
		t.Errorf("should skip binary files")
	}
}

func TestBulkReadTool_respectsIgnorePatterns(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "dep.go"), "package vendor\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n")

	tl := tool.NewBulkReadTool()
	args := []byte(`{"pattern":"**/*.go","path":"` + dir + `"}`)
	ectx := tool.ExecutionContext{
		WorkDir:        dir,
		IgnorePatterns: []string{"vendor/**"},
	}
	res, err := tl.Execute(t.Context(), ectx, args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Output, "dep.go") {
		t.Errorf("should respect ignore patterns")
	}
	if !strings.Contains(res.Output, "main.go") {
		t.Errorf("should include non-ignored files")
	}
}

func TestBulkReadTool_lineLimit(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := range 50 {
		lines = append(lines, strings.Repeat("x", i+1))
	}
	writeFile(t, filepath.Join(dir, "big.go"), strings.Join(lines, "\n"))

	tl := tool.NewBulkReadTool()
	args := []byte(`{"pattern":"*.go","path":"` + dir + `","limit":5}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: dir}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "5 of 50 lines") {
		t.Errorf("expected truncation indicator, got: %s", res.Output)
	}
}

func TestBulkReadTool_emptyPattern(t *testing.T) {
	tl := tool.NewBulkReadTool()
	args := []byte(`{"pattern":""}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != "invalid_args" {
		t.Errorf("expected invalid_args error, got: %s", res.ErrorCode)
	}
}

func TestBulkReadTool_subdirectoryPattern(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "pkg", "util")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "helpers.go"), "package util\nfunc Help() {}\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n")

	tl := tool.NewBulkReadTool()
	args := []byte(`{"pattern":"pkg/**/*.go","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: dir}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "helpers.go") {
		t.Errorf("expected helpers.go in output")
	}
	if strings.Contains(res.Output, "main.go") {
		t.Errorf("should not include files outside pattern")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
