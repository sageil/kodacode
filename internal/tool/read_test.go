package tool_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestReadTool_file(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(p, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewReadTool()
	args := []byte(`{"filePath":"` + p + `"}`)
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "line1") {
		t.Fatalf("expected line1 in output, got: %s", res.Output)
	}
}

func TestReadTool_directory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewReadTool()
	args := []byte(`{"filePath":"` + dir + `"}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected directory error, got: %s", err)
	}
}

func TestReadTool_notFound(t *testing.T) {
	tl := tool.NewReadTool()
	args := []byte(`{"filePath":"/no/such/file/here.txt"}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{}, args)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadTool_MediaDoesNotInlineBase64(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pixel.png")
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO2Zz5kAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}

	tl := tool.NewReadTool()
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"filePath":"`+p+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Output, "data:") || strings.Contains(res.Output, "base64") {
		t.Fatalf("media output should omit inline binary payloads, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Binary content omitted") {
		t.Fatalf("expected media budget warning, got: %s", res.Output)
	}
}

func TestReadTool_OffsetReadsWindowWithoutLoadingWholeFileIntoOutput(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "large.txt")

	var sb strings.Builder
	for i := 1; i <= 500; i++ {
		sb.WriteString("line")
		sb.WriteString(strings.Repeat("x", 20))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	tl := tool.NewReadTool()
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"filePath":"`+p+`","offset":200,"limit":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "200:") {
		t.Fatalf("expected output to include requested offset, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "204:") {
		t.Fatalf("expected output to include requested window end, got: %s", res.Output)
	}
	if strings.Contains(res.Output, "205:") {
		t.Fatalf("unexpected line past requested window, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Use offset=205 to continue") {
		t.Fatalf("expected continuation hint, got: %s", res.Output)
	}
}

func TestReadTool_LongLineOverScannerLimitStillReads(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "long.txt")
	content := strings.Repeat("x", 1024*1024+128) + "\nsecond line\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tl := tool.NewReadTool()
	res, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"filePath":"`+p+`","limit":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "1: ") {
		t.Fatalf("expected first line in output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "2: second line") {
		t.Fatalf("expected second line in output, got: %s", res.Output)
	}
}

func TestReadTool_CachedSliceInvalidatesOnSameSizeRewriteWithRestoredMtime(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cached.txt")
	original := "aa\nbb\n"
	if err := os.WriteFile(p, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	mtime := info.ModTime()

	tl := tool.NewReadTool()
	first, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"filePath":"`+p+`","offset":1,"limit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.Output, "<version>"+versionToken(original)+"</version>") {
		t.Fatalf("unexpected initial version token: %s", first.Output)
	}

	updated := "cc\ndd\n"
	if len(updated) != len(original) {
		t.Fatal("test setup requires same-size rewrite")
	}
	if err := os.WriteFile(p, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	second, err := tl.Execute(t.Context(), tool.ExecutionContext{}, []byte(`{"filePath":"`+p+`","offset":1,"limit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(second.Output, "<version>"+versionToken(updated)+"</version>") {
		t.Fatalf("expected updated version token, got: %s", second.Output)
	}
}
