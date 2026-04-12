package tool_test

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/lsp"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func defaultLSPManager() *lsp.Manager {
	servers := tool.ResolveLSPServers(nil) // defaults to gopls for .go
	return lsp.NewManager(servers)
}

func TestLSPTool_missingAction(t *testing.T) {
	mgr := defaultLSPManager()
	tl := tool.NewLSPTool(mgr)
	args := []byte(`{"filePath":"/tmp/main.go"}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: "/tmp"}, args)
	if err == nil {
		t.Fatal("expected error for missing action")
	}
	if !strings.Contains(err.Error(), "action") || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected action/operations required error, got: %s", err.Error())
	}
}

func TestLSPTool_missingFilePath(t *testing.T) {
	mgr := defaultLSPManager()
	tl := tool.NewLSPTool(mgr)
	args := []byte(`{"action":"definition"}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: "/tmp"}, args)
	if err == nil {
		t.Fatal("expected error for missing filePath")
	}
	if !strings.Contains(err.Error(), "filePath is required") {
		t.Fatalf("expected 'filePath is required' error, got: %s", err.Error())
	}
}

func TestLSPTool_unknownAction(t *testing.T) {
	mgr := defaultLSPManager()
	tl := tool.NewLSPTool(mgr)
	args := []byte(`{"action":"bogus","filePath":"/tmp/main.go"}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: "/tmp"}, args)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("expected 'unknown action' error, got: %s", err.Error())
	}
}

func TestLSPTool_missingLineForDefinition(t *testing.T) {
	mgr := defaultLSPManager()
	tl := tool.NewLSPTool(mgr)
	args := []byte(`{"action":"definition","filePath":"/tmp/main.go"}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: "/tmp"}, args)
	if err == nil {
		t.Fatal("expected error for missing line")
	}
	if !strings.Contains(err.Error(), "line is required") {
		t.Fatalf("expected 'line is required' error, got: %s", err.Error())
	}
}

func TestLSPTool_invalidJSON(t *testing.T) {
	mgr := defaultLSPManager()
	tl := tool.NewLSPTool(mgr)
	args := []byte(`{invalid}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: "/tmp"}, args)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid parameters") {
		t.Fatalf("expected 'invalid parameters' error, got: %s", err.Error())
	}
}

func TestLSPTool_noServerForExtension(t *testing.T) {
	mgr := defaultLSPManager()
	tl := tool.NewLSPTool(mgr)
	args := []byte(`{"action":"definition","filePath":"/tmp/app.rb","line":1,"character":0}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: "/tmp"}, args)
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
	if !strings.Contains(err.Error(), "no LSP server configured") {
		t.Fatalf("expected 'no LSP server configured' error, got: %s", err.Error())
	}
}

func TestLSPTool_disabledServer(t *testing.T) {
	disabled := false
	cfgs := []config.LSPServerConfig{
		{
			Name:       "gopls",
			Command:    "gopls",
			Extensions: []string{".go"},
			Enabled:    &disabled,
		},
	}
	servers := tool.ResolveLSPServers(cfgs) // disabled → falls back to defaults
	mgr := lsp.NewManager(servers)
	tl := tool.NewLSPTool(mgr)
	// Use .rb which has no server even in defaults.
	args := []byte(`{"action":"definition","filePath":"/tmp/app.rb","line":1,"character":0}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: "/tmp"}, args)
	if err == nil {
		t.Fatal("expected error for .rb with no configured server")
	}
	if !strings.Contains(err.Error(), "no LSP server configured") {
		t.Fatalf("expected 'no LSP server configured' error, got: %s", err.Error())
	}
}

func TestLSPTool_symbolsMissingQuery(t *testing.T) {
	mgr := defaultLSPManager()
	tl := tool.NewLSPTool(mgr)
	args := []byte(`{"action":"symbols"}`)
	_, err := tl.Execute(t.Context(), tool.ExecutionContext{WorkDir: "/tmp"}, args)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("expected 'query is required' error, got: %s", err.Error())
	}
}
