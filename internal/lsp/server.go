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
)

type Server struct {
	cfg     ServerConfig
	client  *Client
	rootURI string

	mu                     sync.Mutex
	openFiles              map[string]openDocumentState
	pullDiagnosticsSupport diagnosticsPullSupport

	diagMu      sync.RWMutex
	diagnostics map[string]diagnosticSnapshot
	diagSeqs    map[string]int64
	diagCond    *sync.Cond
}

type openDocumentState struct {
	Version int
	ModTime int64
	Size    int64
}

type diagnosticsPullSupport uint8

const (
	diagnosticsPullUnknown diagnosticsPullSupport = iota
	diagnosticsPullSupported
	diagnosticsPullUnsupported
)

type diagnosticSnapshot struct {
	Diagnostics []Diagnostic
	Version     int
	HasVersion  bool
	PublishSeq  int64
}

func Start(ctx context.Context, cfg ServerConfig, rootURI string) (*Server, error) {
	workDir := URIToPath(rootURI)
	command, err := resolveCommand(cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("%s not found. %s", cfg.Command, installHint(cfg.Name, cfg.Command))
	}

	client, err := NewClient(command, cfg.Args, cfg.Env, workDir)
	if err != nil {
		return nil, err
	}

	server := &Server{
		cfg:                    cfg,
		client:                 client,
		rootURI:                rootURI,
		openFiles:              make(map[string]openDocumentState),
		pullDiagnosticsSupport: diagnosticsPullUnknown,
		diagnostics:            make(map[string]diagnosticSnapshot),
		diagSeqs:               make(map[string]int64),
	}
	server.diagCond = sync.NewCond(server.diagMu.RLocker())
	client.SetDiagnosticsHandler(server.handlePublishedDiagnostics)
	client.SetWorkspaceFolders([]WorkspaceFolder{
		{URI: rootURI, Name: filepath.Base(strings.TrimPrefix(rootURI, "file://"))},
	})

	empty := json.RawMessage("{}")
	workspaceFolders := []WorkspaceFolder{
		{URI: rootURI, Name: filepath.Base(strings.TrimPrefix(rootURI, "file://"))},
	}
	params := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   rootURI,
		Capabilities: ClientCapabilities{
			TextDocument: &TextDocumentClientCapabilities{
				Definition:        &empty,
				CallHierarchy:     &empty,
				DocumentHighlight: &empty,
				Rename:            &empty,
				CodeAction:        &empty,
				Diagnostic:        &empty,
				PublishDiagnostics: &PublishDiagnosticsCapability{
					RelatedInformation: true,
					VersionSupport:     true,
				},
			},
			Workspace: &WorkspaceClientCapabilities{
				Symbol:           &empty,
				Diagnostic:       &empty,
				WorkspaceFolders: true,
			},
		},
		ClientInfo:            ClientInfo{Name: "kodacode", Version: "1.0.0"},
		WorkspaceFolders:      workspaceFolders,
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
	if err := client.Notify("workspace/didChangeConfiguration", map[string]any{
		"settings": map[string]any{},
	}); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("lsp didChangeConfiguration notification: %w", err)
	}
	return server, nil
}

func initOptions(cfg ServerConfig) any {
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
			},
		}
	default:
		return nil
	}
}

func resolveCommand(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	for _, dir := range lspBinDirs() {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
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
}

func installHint(name, command string) string {
	if hint, ok := installHints[command]; ok {
		return hint
	}
	if hint, ok := installHints[name]; ok {
		return hint
	}
	return fmt.Sprintf("Install %s and ensure it is in PATH, ~/.local/share/nvim/mason/bin/, or /opt/homebrew/bin/", command)
}

func (s *Server) Alive() bool {
	return s.client.Alive()
}

func (s *Server) Name() string {
	return s.cfg.Name
}

func (s *Server) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_ = s.client.Call(shutdownCtx, "shutdown", nil, nil)
	_ = s.client.Notify("exit", nil)
	return s.client.Close()
}

func (s *Server) handlePublishedDiagnostics(payload PublishDiagnosticsParams) {
	s.diagMu.Lock()
	nextSeq := s.diagSeqs[payload.URI] + 1
	s.diagSeqs[payload.URI] = nextSeq
	snapshot := diagnosticSnapshot{
		Diagnostics: cloneDiagnostics(payload.Diagnostics),
		PublishSeq:  nextSeq,
	}
	if payload.Version != nil {
		snapshot.Version = *payload.Version
		snapshot.HasVersion = true
	}
	s.diagnostics[payload.URI] = snapshot
	s.diagMu.Unlock()
	s.diagCond.Broadcast()
}
