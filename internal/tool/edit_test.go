package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func containsAll(text string, want ...string) bool {
	for _, w := range want {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

func TestEditTool_simpleReplace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"func foo() {}","newString":"func bar() {}"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Output, "Edited file successfully.\nVersion: ") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
	if got, _ := res.Metadata["version"].(string); got != versionToken("package main\n\nfunc bar() {}\n") {
		t.Fatalf("unexpected version metadata: %#v", res.Metadata)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "package main\n\nfunc bar() {}\n" {
		t.Fatalf("unexpected content: %s", data)
	}
}

func TestEditTool_notFound(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	if err := os.WriteFile(p, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"NOTPRESENT","newString":"x"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeNotFound {
		t.Fatalf("expected not_found result, got %#v", res)
	}
	if res.Output == "" {
		t.Fatal("expected diagnostic output")
	}
}

func TestEditTool_emptyOldStringCreatesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "new.txt")
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"","newString":"created"}`)
	if _, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "created" {
		t.Fatalf("unexpected content: %s", data)
	}
}

func TestEditTool_lineScopedReplace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "package main\n\nfunc foo() {}\n\nfunc foo() {}\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"func foo() {}","newString":"func bar() {}","startLine":5,"endLine":5}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != "" {
		t.Fatalf("unexpected error result: %#v", res)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	want := "package main\n\nfunc foo() {}\n\nfunc bar() {}\n"
	if string(data) != want {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestEditTool_conflictOnStaleExpectedVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	initial := "package main\n\nfunc foo() {}\n"
	if err := os.WriteFile(p, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("package main\n\nfunc baz() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"func baz() {}","newString":"func qux() {}","expectedVersion":"` + versionToken(initial) + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeConflict {
		t.Fatalf("expected conflict result, got %#v", res)
	}
	if !strings.Contains(res.Output, "file version mismatch") {
		t.Fatalf("unexpected conflict output: %q", res.Output)
	}
}

func TestEditTool_ambiguousMatchSuggestsLineScoping(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "package main\n\nfunc foo() {}\n\nfunc foo() {}\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"func foo() {}","newString":"func bar() {}"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeInvalidArgs {
		t.Fatalf("expected invalid_args result, got %#v", res)
	}
	if got := res.Output; got == "" || !containsAll(got, "Candidate lines:", "startLine/endLine") {
		t.Fatalf("unexpected diagnostic: %q", got)
	}
}

func TestEditTool_exactRangeReplace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "package main\n\nfunc alpha() {}\nfunc beta() {}\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"func beta() {}","newString":"func gamma() {}","range":{"startLine":4,"startCharacter":0,"endLine":4,"endCharacter":14}}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != "" {
		t.Fatalf("unexpected error result: %#v", res)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	want := "package main\n\nfunc alpha() {}\nfunc gamma() {}\n"
	if string(data) != want {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestEditTool_doesNotFuzzyMatchWhitespaceVariants(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	if err := os.WriteFile(p, []byte("func foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"func  foo() {}","newString":"func bar() {}"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeNotFound {
		t.Fatalf("expected not_found result, got %#v", res)
	}
	if !strings.Contains(res.Output, "current exact text") {
		t.Fatalf("unexpected diagnostic: %q", res.Output)
	}
}

func TestEditTool_preservesCRLFLineEndings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "alpha\r\nbeta\r\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"beta","newString":"gamma"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != "" {
		t.Fatalf("unexpected error result: %#v", res)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	want := "alpha\r\ngamma\r\n"
	if string(data) != want {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestEditTool_exactRangeReplaceUTF16Emoji(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "x=🚀\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"🚀","newString":"🔥","range":{"startLine":1,"startCharacter":2,"endLine":1,"endCharacter":4}}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != "" {
		t.Fatalf("unexpected error result: %#v", res)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "x=🔥\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestEditTool_overwriteBranchConflictsWhenExpectedVersionFileMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	initial := "before\n"
	if err := os.WriteFile(p, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"","newString":"after\n","expectedVersion":"` + versionToken(initial) + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeConflict {
		t.Fatalf("expected conflict result, got %#v", res)
	}
	if !strings.Contains(res.Output, "file no longer exists") {
		t.Fatalf("unexpected conflict output: %q", res.Output)
	}
}

func TestEditTool_RejectsSymlinkPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	tl := tool.NewEditTool()
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"filePath":"`+link+`","oldString":"before","newString":"after"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodePermission {
		t.Fatalf("expected permission result, got %#v", res)
	}
	if info, err := os.Lstat(link); err != nil {
		t.Fatal(err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink to remain intact, mode=%v", info.Mode())
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "before\n" {
		t.Fatalf("unexpected target content %q (err=%v)", got, err)
	}
}

func TestEditTool_overwriteBranchReturnsVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	tl := tool.NewEditTool()
	args := []byte(`{"filePath":"` + p + `","oldString":"","newString":"created\n"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Output, "Created/overwrote file successfully.\nVersion: ") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
	if got, _ := res.Metadata["version"].(string); got != versionToken("created\n") {
		t.Fatalf("unexpected version metadata: %#v", res.Metadata)
	}
}
