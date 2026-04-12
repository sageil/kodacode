package tool_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestTruncate_short(t *testing.T) {
	r := tool.Truncate("hello", "head")
	if r.Truncated {
		t.Fatal("expected not truncated")
	}
	if r.Content != "hello" {
		t.Fatalf("Truncate(%q, %q).Content = %q, want %q", "hello", "head", r.Content, "hello")
	}
}

func TestTruncate_longLines(t *testing.T) {
	var lines []string
	for range 2001 {
		lines = append(lines, "x")
	}
	text := strings.Join(lines, "\n")
	r := tool.Truncate(text, "head")
	if !r.Truncated {
		t.Fatal("expected truncated for 2001 lines")
	}
	// Content must be bounded.
	got := strings.Split(r.Content, "\n")
	// Allow for the appended hint line(s).
	// The first MaxLines lines should be present.
	if len(got) < tool.MaxLines {
		t.Fatalf("expected at least %d lines in truncated output, got %d", tool.MaxLines, len(got))
	}
}

func TestTruncate_tail(t *testing.T) {
	var lines []string
	for i := range 2001 {
		lines = append(lines, strings.Repeat("a", i%10+1))
	}
	text := strings.Join(lines, "\n")
	r := tool.Truncate(text, "tail")
	if !r.Truncated {
		t.Fatal("expected truncated for 2001 lines with tail direction")
	}
}

func TestTruncate_byteLimit(t *testing.T) {
	// Build a string > MaxBytes but <= MaxLines lines.
	line := strings.Repeat("a", 100)
	var sb strings.Builder
	for i := 0; i < 600; i++ {
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	text := sb.String()
	r := tool.Truncate(text, "head")
	if !r.Truncated {
		t.Fatal("expected truncated by byte limit")
	}
	if len(r.Content) > tool.MaxBytes+200 { // allow for hint line
		t.Fatalf("content too long after truncation: %d bytes", len(r.Content))
	}
}

func TestTruncate_utf8Safe(t *testing.T) {
	// Build content that exceeds MaxBytes with multi-byte runes.
	var sb strings.Builder
	rune_ := '日' // 3-byte UTF-8
	for sb.Len() < tool.MaxBytes+100 {
		sb.WriteRune(rune_)
	}
	text := sb.String()
	r := tool.Truncate(text, "head")
	if !r.Truncated {
		t.Fatal("expected truncation")
	}
	// The truncated content (before hint) must be valid UTF-8.
	// Strip the hint suffix first.
	parts := strings.SplitN(r.Content, "\n\n[output truncated", 2)
	if !utf8.ValidString(parts[0]) {
		t.Fatal("truncated content is not valid UTF-8")
	}
}

func TestExecutionContext_zero(t *testing.T) {
	// Zero-value ExecutionContext must be usable without panicking.
	ectx := tool.ExecutionContext{}
	if ectx.Ask != nil {
		t.Fatal("Ask should be nil by default")
	}
	if ectx.WriteOutput != nil {
		t.Fatal("WriteOutput should be nil by default")
	}
	if ectx.Abort != nil {
		t.Fatal("Abort should be nil by default")
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := tool.NewRegistry()
	bt := &tool.Tool{Name: "bash", Description: "run bash"}
	r.Register(bt)
	got, ok := r.Get("bash")
	if !ok {
		t.Fatal("expected to find bash")
	}
	if got.Name != "bash" {
		t.Fatalf("got %q, want %q", got.Name, "bash")
	}
	// Register replaces an existing tool with the same name.
	replacement := &tool.Tool{Name: "bash", Description: "replaced"}
	r.Register(replacement)
	got2, ok2 := r.Get("bash")
	if !ok2 {
		t.Fatal("expected to find bash after replacement")
	}
	if got2.Description != "replaced" {
		t.Fatalf("Replace: got description %q, want %q", got2.Description, "replaced")
	}
}

func TestRegistry_ForAgent(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(&tool.Tool{Name: "bash"})
	r.Register(&tool.Tool{Name: "read"})
	r.Register(&tool.Tool{Name: "write"})
	got := r.ForAgent([]string{"bash", "read"})
	if len(got) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(got))
	}
	names := map[string]bool{}
	for _, tl := range got {
		names[tl.Name] = true
	}
	if !names["bash"] || !names["read"] {
		t.Fatalf("ForAgent returned wrong tools: %v", got)
	}
}

func TestRegistry_All(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(&tool.Tool{Name: "bash"})
	r.Register(&tool.Tool{Name: "read"})
	all := r.All()
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	r := tool.NewRegistry()
	_, ok := r.Get("notexist")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestRegistry_ForAgent_empty(t *testing.T) {
	r := tool.NewRegistry()
	r.Register(&tool.Tool{Name: "bash"})
	got := r.ForAgent([]string{})
	if len(got) != 0 {
		t.Fatalf("expected 0 tools for empty allow list, got %d", len(got))
	}
}
