package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
)

// Server wraps a Client with LSP protocol knowledge: initialization handshake,
// document tracking, and high-level query methods.
type Server struct {
	cfg     config.LSPServerConfig
	client  *Client
	rootURI string

	mu        sync.Mutex
	openFiles map[string]int

	diagMu      sync.RWMutex
	diagnostics map[string][]Diagnostic
	diagCond    *sync.Cond
}

// Start launches an LSP server process and performs the initialize/initialized handshake.
func Start(ctx context.Context, cfg config.LSPServerConfig, rootURI string) (*Server, error) {
	workDir := URIToPath(rootURI)

	resolvedCmd, err := resolveCommand(cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("%s not found. %s", cfg.Command, installHint(cfg.Name, cfg.Command))
	}

	client, err := NewClient(resolvedCmd, cfg.Args, cfg.Env, workDir)
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:         cfg,
		client:      client,
		rootURI:     rootURI,
		openFiles:   make(map[string]int),
		diagnostics: make(map[string][]Diagnostic),
	}
	s.diagCond = sync.NewCond(s.diagMu.RLocker())

	// Register diagnostics handler before initialization so we capture
	// any diagnostics the server sends during startup.
	client.SetDiagnosticsHandler(func(p PublishDiagnosticsParams) {
		s.diagMu.Lock()
		s.diagnostics[p.URI] = p.Diagnostics
		s.diagMu.Unlock()
		s.diagCond.Broadcast()
	})

	empty := json.RawMessage("{}")
	params := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   rootURI,
		Capabilities: ClientCapabilities{
			TextDocument: &TextDocumentClientCapabilities{
				Definition: &empty,
				References: &empty,
				Hover:      &empty,
				CodeAction: &empty,
				PublishDiagnostics: &PublishDiagnosticsCapability{
					RelatedInformation: true,
					VersionSupport:     true,
				},
			},
			Workspace: &WorkspaceClientCapabilities{
				Symbol:           &empty,
				WorkspaceFolders: true,
			},
		},
		ClientInfo: ClientInfo{Name: "kodacode", Version: "1.0.0"},
		WorkspaceFolders: []WorkspaceFolder{
			{URI: rootURI, Name: filepath.Base(strings.TrimPrefix(rootURI, "file://"))},
		},
		InitializationOptions: initOptions(cfg),
	}

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var result InitializeResult
	if err := client.Call(initCtx, "initialize", params, &result); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("lsp initialize %s: %w", cfg.Name, err)
	}

	if err := client.Notify("initialized", struct{}{}); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("lsp initialized notification: %w", err)
	}

	return s, nil
}

// initOptions builds the initializationOptions for an LSP server.
// User-provided init_options from config are used as-is. For known servers
// without user overrides, sensible defaults are applied.
func initOptions(cfg config.LSPServerConfig) any {
	if len(cfg.InitOptions) > 0 {
		return cfg.InitOptions
	}
	switch cfg.Name {
	case "vtsls":
		return map[string]any{
			"typescript": map[string]any{
				"tsserver": map[string]any{
					"maxTsServerMemory": 3072,
				},
				"preferences": map[string]any{
					"disableSuggestions": true,
				},
			},
		}
	default:
		return nil
	}
}

// resolveCommand checks if a command is available in PATH or common LSP dirs.
func resolveCommand(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, dir := range lspBinDirs() {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("not found")
}

var installHints = map[string]string{
	"gopls":                      "Install: go install golang.org/x/tools/gopls@latest",
	"vtsls":                      "Install: npm install -g @vtsls/language-server",
	"typescript-language-server": "Install: npm install -g typescript-language-server typescript",
	"pyright":                    "Install: pip install pyright (binary: pyright-langserver)",
	"pyright-langserver":         "Install: pip install pyright",
	"rust-analyzer":              "Install: rustup component add rust-analyzer",
	"clangd":                     "Install: brew install llvm (macOS) or apt install clangd (Linux)",
}

func installHint(name, command string) string {
	if hint, ok := installHints[command]; ok {
		return hint
	}
	if hint, ok := installHints[name]; ok {
		return hint
	}
	return fmt.Sprintf("Install %s and ensure it is in your PATH or ~/.local/share/nvim/mason/bin/", command)
}

// Alive reports whether the underlying server process is still running.
func (s *Server) Alive() bool {
	return s.client.Alive()
}

// Name returns the server's configured name.
func (s *Server) Name() string {
	return s.cfg.Name
}

// EnsureOpen sends textDocument/didOpen if we haven't opened this file yet.
// This is required by the LSP spec before making queries on a file.
func (s *Server) EnsureOpen(ctx context.Context, filePath string) error {
	uri := FileURI(filePath)

	s.mu.Lock()
	if _, ok := s.openFiles[uri]; ok {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file for didOpen: %w", err)
	}

	ext := filepath.Ext(filePath)
	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: LanguageID(ext),
			Version:    1,
			Text:       string(content),
		},
	}
	if err := s.client.Notify("textDocument/didOpen", params); err != nil {
		return fmt.Errorf("didOpen: %w", err)
	}

	s.mu.Lock()
	s.openFiles[uri] = 1
	s.mu.Unlock()
	return nil
}

// DocumentVersion returns the current tracked LSP document version for an open file.
func (s *Server) DocumentVersion(filePath string) (int, bool) {
	uri := FileURI(filePath)
	s.mu.Lock()
	defer s.mu.Unlock()
	version, ok := s.openFiles[uri]
	return version, ok
}

// CloseDocument sends textDocument/didClose for an open file and clears local tracking.
func (s *Server) CloseDocument(filePath string) error {
	uri := FileURI(filePath)

	s.mu.Lock()
	_, isOpen := s.openFiles[uri]
	s.mu.Unlock()
	if !isOpen {
		return nil
	}

	params := DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}
	if err := s.client.Notify("textDocument/didClose", params); err != nil {
		return fmt.Errorf("didClose: %w", err)
	}

	s.mu.Lock()
	delete(s.openFiles, uri)
	s.mu.Unlock()

	s.diagMu.Lock()
	delete(s.diagnostics, uri)
	s.diagMu.Unlock()

	return nil
}

// NotifyChanged sends textDocument/didChange with the current file content.
// If the file hasn't been opened yet, it opens it instead.
// This must be called after editing a file so the LSP server re-analyzes it.
func (s *Server) NotifyChanged(ctx context.Context, filePath string) error {
	uri := FileURI(filePath)

	s.mu.Lock()
	version, isOpen := s.openFiles[uri]
	if isOpen {
		version++
		s.openFiles[uri] = version
	}
	s.mu.Unlock()

	if !isOpen {
		return s.EnsureOpen(ctx, filePath)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file for didChange: %w", err)
	}

	params := DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			URI:     uri,
			Version: version,
		},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: string(content)},
		},
	}
	if err := s.client.Notify("textDocument/didChange", params); err != nil {
		return fmt.Errorf("didChange: %w", err)
	}

	// Clear stale diagnostics so the next query waits for fresh ones.
	s.diagMu.Lock()
	delete(s.diagnostics, uri)
	s.diagMu.Unlock()

	return nil
}

// Definition returns the definition location(s) for the symbol at the given position.
// Line and character are 0-indexed.
func (s *Server) Definition(ctx context.Context, filePath string, line, character int) ([]Location, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Position:     Position{Line: line, Character: character},
	}

	// Definition result can be Location, []Location, or LocationLink[].
	// We handle the common cases.
	var raw json.RawMessage
	if err := s.client.Call(ctx, "textDocument/definition", params, &raw); err != nil {
		return nil, err
	}

	// Try single location.
	var single Location
	if json.Unmarshal(raw, &single) == nil && single.URI != "" {
		return []Location{single}, nil
	}
	// Try array of locations.
	var multi []Location
	if json.Unmarshal(raw, &multi) == nil {
		return multi, nil
	}
	// Try LocationLink array (has targetUri instead of uri).
	var links []struct {
		TargetURI   string `json:"targetUri"`
		TargetRange Range  `json:"targetRange"`
	}
	if json.Unmarshal(raw, &links) == nil {
		for _, l := range links {
			multi = append(multi, Location{URI: l.TargetURI, Range: l.TargetRange})
		}
		return multi, nil
	}

	return nil, nil
}

// References returns all references to the symbol at the given position.
func (s *Server) References(ctx context.Context, filePath string, line, character int) ([]Location, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	params := ReferenceParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Position:     Position{Line: line, Character: character},
		Context:      ReferenceContext{IncludeDeclaration: true},
	}

	var locations []Location
	if err := s.client.Call(ctx, "textDocument/references", params, &locations); err != nil {
		return nil, err
	}
	return locations, nil
}

// Hover returns hover information for the symbol at the given position.
func (s *Server) Hover(ctx context.Context, filePath string, line, character int) (*HoverResult, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Position:     Position{Line: line, Character: character},
	}

	var result HoverResult
	if err := s.client.Call(ctx, "textDocument/hover", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Rename requests a workspace edit for renaming the symbol at the given position.
func (s *Server) Rename(ctx context.Context, filePath string, line, character int, newName string) (*WorkspaceEdit, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	params := RenameParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Position:     Position{Line: line, Character: character},
		NewName:      newName,
	}

	var edit WorkspaceEdit
	if err := s.client.Call(ctx, "textDocument/rename", params, &edit); err != nil {
		return nil, err
	}
	return &edit, nil
}

// CodeActions returns the code actions available for a file range.
func (s *Server) CodeActions(ctx context.Context, filePath string, rng Range, only []string) ([]CodeAction, error) {
	if err := s.EnsureOpen(ctx, filePath); err != nil {
		return nil, err
	}

	params := CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: FileURI(filePath)},
		Range:        rng,
		Context: CodeActionContext{
			Diagnostics: s.Diagnostics(filePath),
			Only:        only,
		},
	}

	var raw []json.RawMessage
	if err := s.client.Call(ctx, "textDocument/codeAction", params, &raw); err != nil {
		return nil, err
	}

	actions := make([]CodeAction, 0, len(raw))
	for _, item := range raw {
		var action CodeAction
		if err := json.Unmarshal(item, &action); err == nil && action.Title != "" {
			actions = append(actions, action)
			continue
		}
		var cmd Command
		if err := json.Unmarshal(item, &cmd); err == nil && cmd.Title != "" {
			actions = append(actions, CodeAction{Title: cmd.Title, Command: &cmd})
			continue
		}
	}
	return actions, nil
}

// Diagnostics returns the latest cached diagnostics for the given file.
// Diagnostics are pushed asynchronously by the server after didOpen.
func (s *Server) Diagnostics(filePath string) []Diagnostic {
	uri := FileURI(filePath)
	s.diagMu.RLock()
	defer s.diagMu.RUnlock()
	src := s.diagnostics[uri]
	if len(src) == 0 {
		return nil
	}
	return append([]Diagnostic{}, src...)
}

func (s *Server) WaitDiagnostics(ctx context.Context, filePath string) []Diagnostic {
	uri := FileURI(filePath)

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			s.diagCond.Broadcast()
		case <-done:
		}
	}()

	s.diagMu.RLock()
	defer s.diagMu.RUnlock()
	for len(s.diagnostics[uri]) == 0 {
		if ctx.Err() != nil {
			return nil
		}
		s.diagCond.Wait()
	}
	return append([]Diagnostic{}, s.diagnostics[uri]...)
}

// WorkspaceSymbol searches for symbols matching the query across the workspace.
func (s *Server) WorkspaceSymbol(ctx context.Context, query string) ([]SymbolInformation, error) {
	params := WorkspaceSymbolParams{Query: query}
	var symbols []SymbolInformation
	if err := s.client.Call(ctx, "workspace/symbol", params, &symbols); err != nil {
		return nil, err
	}
	return symbols, nil
}

// Shutdown performs the LSP shutdown/exit handshake and kills the process.
func (s *Server) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_ = s.client.Call(shutdownCtx, "shutdown", nil, nil)
	_ = s.client.Notify("exit", nil)

	return s.client.Close()
}
