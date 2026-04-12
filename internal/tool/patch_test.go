package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestPatchTool_multipleEdits(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc foo() {}\nfunc bar() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","edits":[` +
		`{"oldString":"func foo() {}","newString":"func alpha() {}"},` +
		`{"oldString":"func bar() {}","newString":"func beta() {}"}` +
		`]}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "2 edits") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
	data, _ := os.ReadFile(p)
	expected := "package main\n\nfunc alpha() {}\nfunc beta() {}\n"
	if string(data) != expected {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestPatchTool_failedEditNoPartialWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	original := "package main\n\nfunc foo() {}\n"
	if err := os.WriteFile(p, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	// First edit succeeds, second edit fails — file should remain unchanged.
	args := []byte(`{"filePath":"` + p + `","edits":[` +
		`{"oldString":"func foo() {}","newString":"func bar() {}"},` +
		`{"oldString":"NOTPRESENT","newString":"x"}` +
		`]}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeNotFound {
		t.Fatalf("expected not_found result, got %#v", res)
	}
	if !strings.Contains(res.Output, "edit 1") {
		t.Fatalf("expected error to reference edit index 1, got: %s", res.Output)
	}
	// Verify file was not modified.
	data, _ := os.ReadFile(p)
	if string(data) != original {
		t.Fatalf("file should not have been modified, got: %q", data)
	}
}

func TestPatchTool_emptyEdits(t *testing.T) {
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"/tmp/x","edits":[]}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeInvalidArgs {
		t.Fatalf("expected invalid_args result, got %#v", res)
	}
}

func TestPatchTool_editsAsString(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	if err := os.WriteFile(p, []byte("func foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	// Model sends edits as a JSON string containing an array.
	args := []byte(`{"filePath":"` + p + `","edits":"[{\"oldString\":\"func foo() {}\",\"newString\":\"func bar() {}\"}]"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatalf("expected string-wrapped edits to parse, got: %v", err)
	}
	if !strings.Contains(res.Output, "1 edits") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "func bar() {}\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestPatchTool_singleEditObject(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	if err := os.WriteFile(p, []byte("func foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	// Model sends edits as a single object instead of an array.
	args := []byte(`{"filePath":"` + p + `","edits":{"oldString":"func foo() {}","newString":"func bar() {}"}}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatalf("expected single-object edits to parse, got: %v", err)
	}
	if !strings.Contains(res.Output, "1 edits") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "func bar() {}\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestPatchTool_lineScopedEdit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "package main\n\nfunc foo() {}\n\nfunc foo() {}\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","edits":[{"oldString":"func foo() {}","newString":"func bar() {}","startLine":5,"endLine":5}]}`)
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

func TestPatchTool_exactRangeEdit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "package main\n\nfunc alpha() {}\nfunc beta() {}\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","edits":[{"oldString":"func beta() {}","newString":"func gamma() {}","range":{"startLine":4,"startCharacter":0,"endLine":4,"endCharacter":14}}]}`)
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

func TestPatchTool_conflictOnStaleExpectedVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	initial := "package main\n\nfunc foo() {}\n"
	if err := os.WriteFile(p, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("package main\n\nfunc bar() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","expectedVersion":"` + versionToken(initial) + `","edits":[{"oldString":"func bar() {}","newString":"func baz() {}"}]}`)
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

func TestPatchTool_doesNotFuzzyMatchWhitespaceVariants(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	if err := os.WriteFile(p, []byte("func foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","edits":[{"oldString":"func  foo() {}","newString":"func bar() {}"}]}`)
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

func TestPatchTool_preservesCRLFLineEndings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "alpha\r\nbeta\r\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","edits":[{"oldString":"beta","newString":"gamma"}]}`)
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

func TestPatchTool_exactRangeReplaceUTF16Emoji(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "x=🚀\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","edits":[{"oldString":"🚀","newString":"🔥","range":{"startLine":1,"startCharacter":2,"endLine":1,"endCharacter":4}}]}`)
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

func TestPatchTool_scopedEditsUseOriginalFileCoordinates(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","edits":[` +
		`{"oldString":"one","newString":"ONE\nINSERTED","startLine":1,"endLine":1},` +
		`{"oldString":"four","newString":"FOUR","startLine":4,"endLine":4}` +
		`]}`)
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
	want := "ONE\nINSERTED\ntwo\nthree\nFOUR\n"
	if string(data) != want {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestPatchTool_rejectsOverlappingOriginalHunks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "one\ntwo\nthree\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","edits":[` +
		`{"oldString":"one\ntwo","newString":"ONE\nTWO","startLine":1,"endLine":2},` +
		`{"oldString":"two","newString":"TWO","startLine":2,"endLine":2}` +
		`]}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeInvalidArgs {
		t.Fatalf("expected invalid_args result, got %#v", res)
	}
	if !strings.Contains(res.Output, "overlap") {
		t.Fatalf("unexpected overlap output: %q", res.Output)
	}
}

func TestPatchTool_rejectsSameStartInsertions(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src.go")
	content := "alpha\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewPatchTool()
	args := []byte(`{"filePath":"` + p + `","edits":[` +
		`{"oldString":"","newString":"X","range":{"startLine":1,"startCharacter":0,"endLine":1,"endCharacter":0}},` +
		`{"oldString":"","newString":"Y","range":{"startLine":1,"startCharacter":0,"endLine":1,"endCharacter":0}}` +
		`]}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeInvalidArgs {
		t.Fatalf("expected invalid_args result, got %#v", res)
	}
	if !strings.Contains(res.Output, "begin at the same position") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestPatchTool_RejectsSymlinkPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	tl := tool.NewPatchTool()
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"filePath":"`+link+`","edits":[{"oldString":"before","newString":"after"}]}`))
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
