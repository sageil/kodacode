package lsp

import (
	"testing"
)

func TestDiagnosticsClearUsesURI(t *testing.T) {
	s := &Server{
		openFiles:   make(map[string]int),
		diagnostics: make(map[string][]Diagnostic),
	}

	filePath := "/tmp/test-project/src/app.ts"
	uri := FileURI(filePath)

	// Simulate the LSP server pushing diagnostics (keyed by URI).
	s.diagMu.Lock()
	s.diagnostics[uri] = []Diagnostic{
		{Range: Range{Start: Position{Line: 5}}, Severity: 1, Message: "type error"},
	}
	s.diagMu.Unlock()

	// Verify diagnostics are retrievable.
	diags := s.Diagnostics(filePath)
	if len(diags) != 1 {
		t.Fatalf("before clear: want 1 diagnostic, got %d", len(diags))
	}

	// Simulate what NotifyChanged does internally: clear by URI.
	s.diagMu.Lock()
	delete(s.diagnostics, uri)
	s.diagMu.Unlock()

	diags = s.Diagnostics(filePath)
	if len(diags) != 0 {
		t.Errorf("after clear by URI: want 0 diagnostics, got %d", len(diags))
	}
}

func TestDiagnosticsClearByRawPathDoesNotWork(t *testing.T) {
	s := &Server{
		openFiles:   make(map[string]int),
		diagnostics: make(map[string][]Diagnostic),
	}

	filePath := "/tmp/test-project/src/app.ts"
	uri := FileURI(filePath)

	s.diagMu.Lock()
	s.diagnostics[uri] = []Diagnostic{
		{Range: Range{Start: Position{Line: 5}}, Severity: 1, Message: "type error"},
	}
	s.diagMu.Unlock()

	// Deleting by raw path (the old bug) doesn't clear anything.
	s.diagMu.Lock()
	delete(s.diagnostics, filePath)
	s.diagMu.Unlock()

	diags := s.Diagnostics(filePath)
	if len(diags) != 1 {
		t.Errorf("delete by raw path should be a no-op (keys are URIs), want 1 diagnostic, got %d", len(diags))
	}
}
