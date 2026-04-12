package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestWriteTool_create(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	tl := tool.NewWriteTool()
	args := []byte(`{"filePath":"` + p + `","content":"hello world"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Output, "Created out.txt (11 B)\nVersion: ") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
	if got, _ := res.Metadata["version"].(string); got == "" {
		t.Fatalf("expected version metadata, got %#v", res.Metadata)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("unexpected file content: %s", data)
	}
}

func TestWriteTool_createsParentDirs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "deep", "file.txt")
	tl := tool.NewWriteTool()
	args := []byte(`{"filePath":"` + p + `","content":"deep"}`)
	if _, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("file not created:", err)
	}
}

func TestWriteTool_noOpWhenContentUnchanged(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewWriteTool()
	args := []byte(`{"filePath":"` + p + `","content":"hello world"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "Unchanged out.txt (11 B)" {
		t.Fatalf("unexpected output: %s", res.Output)
	}
	if changed, _ := res.Metadata["changed"].(bool); changed {
		t.Fatalf("expected changed=false metadata, got %#v", res.Metadata)
	}
	if got, _ := res.Metadata["version"].(string); got != versionToken("hello world") {
		t.Fatalf("unexpected version metadata: %#v", res.Metadata)
	}
}

func TestWriteTool_conflictOnStaleExpectedVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewWriteTool()
	args := []byte(`{"filePath":"` + p + `","content":"after","expectedVersion":"deadbeef"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeConflict {
		t.Fatalf("expected conflict result, got %#v", res)
	}
	if !strings.Contains(res.Output, `Expected version "deadbeef"`) {
		t.Fatalf("unexpected conflict output: %q", res.Output)
	}
}

func TestWriteTool_conflictWhenExpectedVersionFileMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	tl := tool.NewWriteTool()
	args := []byte(`{"filePath":"` + p + `","content":"after","expectedVersion":"deadbeef"}`)
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

func TestWriteTool_conflictOnLargeFileMiddleChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "large.txt")
	original := strings.Repeat("a", 5000) + "MIDDLE" + strings.Repeat("b", 5000)
	if err := os.WriteFile(p, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	staleVersion := versionToken(original)
	changed := strings.Repeat("a", 5000) + "CHANGD" + strings.Repeat("b", 5000)
	if err := os.WriteFile(p, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewWriteTool()
	args := []byte(`{"filePath":"` + p + `","content":"replacement","expectedVersion":"` + staleVersion + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodeConflict {
		t.Fatalf("expected conflict result, got %#v", res)
	}
}

func TestWriteTool_RejectsSymlinkPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	tl := tool.NewWriteTool()
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"filePath":"`+link+`","content":"after"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.ErrorCode != tool.ErrCodePermission {
		t.Fatalf("expected permission result, got %#v", res)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal(err)
	} else if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink to remain intact, mode=%v", info.Mode())
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "before" {
		t.Fatalf("unexpected target content %q (err=%v)", got, err)
	}
}
