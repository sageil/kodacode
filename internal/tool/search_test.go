package tool_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestSearchTool_fileNameMatch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewSearchTool(nil)
	args := []byte(`{"query":"handler","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "File Matches") {
		t.Fatalf("expected File Matches section, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "handler.go") {
		t.Fatalf("expected handler.go in output, got: %s", res.Output)
	}
}

func TestSearchTool_contentMatch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("func executeSearch() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewSearchTool(nil)
	args := []byte(`{"query":"executeSearch","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Content Matches") {
		t.Fatalf("expected Content Matches section, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "main.go") {
		t.Fatalf("expected main.go in output, got: %s", res.Output)
	}
}

func TestSearchTool_noMatch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewSearchTool(nil)
	args := []byte(`{"query":"XYZNOTFOUND","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "No results") {
		t.Fatalf("expected no results, got: %s", res.Output)
	}
}

func TestSearchTool_includeFilter(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "handler.ts"), []byte("export function handler() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewSearchTool(nil)
	args := []byte(`{"query":"handler","path":"` + dir + `","include":"*.go"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "handler.go") {
		t.Fatalf("expected handler.go in output, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "handler.ts") {
		t.Fatalf("expected handler.ts to be filtered out, got: %s", res.Output)
	}
}

func TestSearchTool_deduplication(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir := t.TempDir()
	// A file that matches by both name and content should appear only once.
	if err := os.WriteFile(filepath.Join(dir, "search.go"), []byte("func search() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewSearchTool(nil)
	args := []byte(`{"query":"search","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	// Should appear in file matches but NOT duplicated in content matches.
	count := strings.Count(res.Output, "search.go")
	if count != 1 {
		t.Fatalf("expected search.go to appear exactly once, appeared %d times in: %s", count, res.Output)
	}
}

func TestSearchTool_invalidPath(t *testing.T) {
	tl := tool.NewSearchTool(nil)
	args := []byte(`{"query":"handler","path":"/definitely/missing/kodacode-search-root"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != tool.ErrCodeNotFound {
		t.Fatalf("error_code = %q, want %q (output=%q)", res.ErrorCode, tool.ErrCodeNotFound, res.Output)
	}
	if !strings.Contains(res.Output, "not found") {
		t.Fatalf("expected not found output, got: %s", res.Output)
	}
}

func TestSearchTool_invalidRegexSurfacesError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc run() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewSearchTool(nil)
	args := []byte(`{"query":"[","path":"` + dir + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode == "" {
		t.Fatalf("expected structured error result, got output: %s", res.Output)
	}
	if !strings.Contains(res.Output, "scan failed") {
		t.Fatalf("expected scan failure output, got: %s", res.Output)
	}
}
