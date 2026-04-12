package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/lsp"
)

func TestFormatGroupedDiagnostics_DropsOutOfBoundsLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.ts")
	if err := os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diags := []lsp.Diagnostic{
		{Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}, Severity: 1, Message: "real error on line 1"},
		{Range: lsp.Range{Start: lsp.Position{Line: 2, Character: 0}}, Severity: 2, Message: "real warning on line 3"},
		{Range: lsp.Range{Start: lsp.Position{Line: 10, Character: 0}}, Severity: 1, Message: "phantom error on line 11"},
		{Range: lsp.Range{Start: lsp.Position{Line: 99, Character: 0}}, Severity: 1, Message: "phantom error on line 100"},
	}

	got, errors, warnings := formatGroupedDiagnostics(f, "test.ts", diags)

	if !strings.Contains(got, "real error on line 1") {
		t.Errorf("should include valid diagnostic on line 1, got:\n%s", got)
	}
	if !strings.Contains(got, "real warning on line 3") {
		t.Errorf("should include valid diagnostic on line 3, got:\n%s", got)
	}
	if strings.Contains(got, "phantom error on line 11") {
		t.Errorf("should NOT include out-of-bounds diagnostic on line 11, got:\n%s", got)
	}
	if strings.Contains(got, "phantom error on line 100") {
		t.Errorf("should NOT include out-of-bounds diagnostic on line 100, got:\n%s", got)
	}
	if errors != 1 {
		t.Errorf("errors = %d, want 1 (only line-1 error is in bounds)", errors)
	}
	if warnings != 1 {
		t.Errorf("warnings = %d, want 1", warnings)
	}
}

func TestFormatGroupedDiagnostics_AllOutOfBoundsReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "small.go")
	if err := os.WriteFile(f, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diags := []lsp.Diagnostic{
		{Range: lsp.Range{Start: lsp.Position{Line: 5, Character: 0}}, Severity: 1, Message: "ghost"},
		{Range: lsp.Range{Start: lsp.Position{Line: 31, Character: 0}}, Severity: 1, Message: "ghost2"},
	}

	got, errors, warnings := formatGroupedDiagnostics(f, "small.go", diags)
	if errors != 0 || warnings != 0 {
		t.Errorf("all out-of-bounds: errors=%d warnings=%d, want 0/0", errors, warnings)
	}
	if !strings.Contains(got, "0 unique") {
		t.Errorf("all out-of-bounds should show 0 unique, got:\n%s", got)
	}
}

func TestFormatGroupedDiagnostics_NonexistentFilePassesThrough(t *testing.T) {
	diags := []lsp.Diagnostic{
		{Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}, Severity: 1, Message: "error"},
	}

	got, errors, _ := formatGroupedDiagnostics("/nonexistent/file.go", "file.go", diags)
	if got == "" {
		t.Error("diagnostics for nonexistent file should still pass through")
	}
	if errors != 1 {
		t.Errorf("errors = %d, want 1", errors)
	}
}

func TestFormatGroupedDiagnostics_DeduplicatesIdenticalMessages(t *testing.T) {
	diags := []lsp.Diagnostic{
		{Range: lsp.Range{Start: lsp.Position{Line: 10, Character: 0}}, Severity: 1, Message: "Cannot find name 'req'."},
		{Range: lsp.Range{Start: lsp.Position{Line: 20, Character: 0}}, Severity: 1, Message: "Cannot find name 'req'."},
		{Range: lsp.Range{Start: lsp.Position{Line: 30, Character: 0}}, Severity: 1, Message: "Cannot find name 'req'."},
		{Range: lsp.Range{Start: lsp.Position{Line: 5, Character: 0}}, Severity: 1, Message: "Unique error"},
	}

	got, errors, _ := formatGroupedDiagnostics("/nonexistent/file.ts", "file.ts", diags)

	if errors != 4 {
		t.Errorf("errors = %d, want 4", errors)
	}
	if !strings.Contains(got, "×3") {
		t.Errorf("should show ×3 for deduplicated 'req' errors, got:\n%s", got)
	}
	if !strings.Contains(got, "2 unique") {
		t.Errorf("should show 2 unique messages, got:\n%s", got)
	}
	if strings.Count(got, "Cannot find name 'req'.") != 1 {
		t.Errorf("deduplicated message should appear only once, got:\n%s", got)
	}
}

func TestFormatGroupedDiagnostics_SummaryLine(t *testing.T) {
	diags := []lsp.Diagnostic{
		{Range: lsp.Range{Start: lsp.Position{Line: 0}}, Severity: 1, Message: "err1"},
		{Range: lsp.Range{Start: lsp.Position{Line: 1}}, Severity: 2, Message: "warn1"},
	}

	got, _, _ := formatGroupedDiagnostics("/x/y.ts", "y.ts", diags)

	if !strings.Contains(got, "1 errors") {
		t.Errorf("summary should contain error count, got:\n%s", got)
	}
	if !strings.Contains(got, "1 warnings") {
		t.Errorf("summary should contain warning count, got:\n%s", got)
	}
	if !strings.Contains(got, "in y.ts") {
		t.Errorf("summary should show display path, got:\n%s", got)
	}
}

func TestCountFileLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"empty", "", 0},
		{"one line no newline", "hello", 1},
		{"one line with newline", "hello\n", 1},
		{"three lines", "a\nb\nc\n", 3},
		{"three lines no trailing newline", "a\nb\nc", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			f := filepath.Join(dir, "test.txt")
			if err := os.WriteFile(f, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got := countFileLines(f)
			if got != tt.want {
				t.Errorf("countFileLines() = %d, want %d", got, tt.want)
			}
		})
	}
}
