package lsp

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
)

const failureCooldown = 30 * time.Second

type failedEntry struct {
	err error
	at  time.Time
}

// Manager manages multiple LSP servers keyed by file extension.
// Servers are started on demand and kept warm for subsequent queries.
type Manager struct {
	configs []config.LSPServerConfig

	mu       sync.Mutex
	servers  map[string]*Server          // config name → running server
	extMap   map[string]string           // extension → config name (read-only after init)
	failed   map[string]failedEntry      // config name → cached start error with timestamp
	starting map[string]*sync.Once       // config name → start-once guard
}

// NewManager creates a manager from the resolved server configs.
// No servers are started yet — they start on demand when a query arrives
// for a matching file extension.
func NewManager(configs []config.LSPServerConfig) *Manager {
	extMap := make(map[string]string)
	for _, cfg := range configs {
		for _, ext := range cfg.Extensions {
			extMap[strings.ToLower(ext)] = cfg.Name
		}
	}
	return &Manager{
		configs:  configs,
		servers:  make(map[string]*Server),
		extMap:   extMap,
		failed:   make(map[string]failedEntry),
		starting: make(map[string]*sync.Once),
	}
}

// ServerFor returns a running server for the given file extension,
// starting one if necessary. rootURI is the project root as a file:// URI.
func (m *Manager) ServerFor(ctx context.Context, ext, rootURI string) (*Server, error) {
	ext = strings.ToLower(ext)
	name, ok := m.extMap[ext]
	if !ok {
		return nil, fmt.Errorf("no LSP server configured for %q files", ext)
	}

	m.mu.Lock()
	if srv, ok := m.servers[name]; ok && srv.Alive() {
		m.mu.Unlock()
		return srv, nil
	}

	if cached, ok := m.failed[name]; ok {
		if time.Since(cached.at) < failureCooldown {
			m.mu.Unlock()
			return nil, cached.err
		}
		delete(m.failed, name)
		delete(m.starting, name)
	}

	// Dead server — clean up so we can restart.
	if _, ok := m.servers[name]; ok {
		delete(m.servers, name)
		delete(m.starting, name)
	}

	once, ok := m.starting[name]
	if !ok {
		once = &sync.Once{}
		m.starting[name] = once
	}
	m.mu.Unlock()

	var startErr error
	once.Do(func() {
		cfg := m.findConfig(name)
		if cfg == nil {
			startErr = fmt.Errorf("lsp config %q not found", name)
			return
		}
		log.Printf("lsp: starting %s (%s)", cfg.Name, cfg.Command)
		var srv *Server
		srv, startErr = Start(ctx, *cfg, rootURI)
		if startErr != nil {
			log.Printf("lsp: failed to start %s: %v", cfg.Name, startErr)
			m.mu.Lock()
			m.failed[name] = failedEntry{err: startErr, at: time.Now()}
			m.mu.Unlock()
			return
		}
		m.mu.Lock()
		m.servers[name] = srv
		m.mu.Unlock()
	})

	if startErr != nil {
		return nil, startErr
	}

	m.mu.Lock()
	srv := m.servers[name]
	m.mu.Unlock()
	if srv == nil {
		return nil, fmt.Errorf("lsp server %q failed to start", name)
	}
	return srv, nil
}

// WorkspaceSymbol queries all running servers and merges results.
func (m *Manager) WorkspaceSymbol(ctx context.Context, query string) ([]SymbolInformation, error) {
	m.mu.Lock()
	servers := make([]*Server, 0, len(m.servers))
	for _, s := range m.servers {
		if s.Alive() {
			servers = append(servers, s)
		}
	}
	m.mu.Unlock()

	if len(servers) == 0 {
		return nil, nil
	}

	// Query all servers concurrently.
	type result struct {
		symbols []SymbolInformation
		err     error
	}
	ch := make(chan result, len(servers))
	for _, srv := range servers {
		go func(s *Server) {
			syms, err := s.WorkspaceSymbol(ctx, query)
			ch <- result{syms, err}
		}(srv)
	}

	var all []SymbolInformation
	for range servers {
		r := <-ch
		if r.err != nil {
			log.Printf("lsp: workspace/symbol error: %v", r.err)
			continue
		}
		all = append(all, r.symbols...)
	}
	return all, nil
}

// SymbolResult is a simplified workspace symbol with human-readable fields.
// Used by the TUI palette to avoid depending on raw LSP protocol types.
type SymbolResult struct {
	Name string
	Kind string // human-readable: "Function", "Struct", "Method", etc.
	File string // absolute file path
	Line int    // 1-indexed
}

// WorkspaceSymbolSearch queries all running servers and returns simplified results.
// This is the high-level API intended for UI consumers (e.g. command palette).
func (m *Manager) WorkspaceSymbolSearch(ctx context.Context, query string) ([]SymbolResult, error) {
	symbols, err := m.WorkspaceSymbol(ctx, query)
	if err != nil {
		return nil, err
	}
	results := make([]SymbolResult, len(symbols))
	for i, sym := range symbols {
		results[i] = SymbolResult{
			Name: sym.Name,
			Kind: SymbolKindName(sym.Kind),
			File: URIToPath(sym.Location.URI),
			Line: sym.Location.Range.Start.Line + 1,
		}
	}
	return results, nil
}

func (m *Manager) liveServerForPath(filePath string) *Server {
	ext := strings.ToLower(filepath.Ext(filePath))
	name, ok := m.extMap[ext]
	if !ok {
		return nil
	}
	m.mu.Lock()
	srv, exists := m.servers[name]
	m.mu.Unlock()
	if !exists || !srv.Alive() {
		return nil
	}
	return srv
}

// SyncChanged notifies the appropriate LSP server that a file has been modified.
// If no matching server is running for the file's extension, this is a no-op.
func (m *Manager) SyncChanged(ctx context.Context, filePath string) error {
	srv := m.liveServerForPath(filePath)
	if srv == nil {
		return nil
	}
	return srv.NotifyChanged(ctx, filePath)
}

// SyncDeleted closes any tracked open document for the given file path.
func (m *Manager) SyncDeleted(_ context.Context, filePath string) error {
	srv := m.liveServerForPath(filePath)
	if srv == nil {
		return nil
	}
	return srv.CloseDocument(filePath)
}

// SyncRenamed closes old/new tracked documents and refreshes the new path.
func (m *Manager) SyncRenamed(ctx context.Context, oldPath, newPath string) error {
	if oldSrv := m.liveServerForPath(oldPath); oldSrv != nil {
		if err := oldSrv.CloseDocument(oldPath); err != nil {
			return err
		}
	}
	if newSrv := m.liveServerForPath(newPath); newSrv != nil {
		if err := newSrv.CloseDocument(newPath); err != nil {
			return err
		}
		if err := newSrv.NotifyChanged(ctx, newPath); err != nil {
			return err
		}
	}
	return nil
}

// NotifyChanged notifies the appropriate LSP server that a file has been modified.
// If no server is running for the file's extension, this is a no-op.
func (m *Manager) NotifyChanged(ctx context.Context, filePath string) {
	if err := m.SyncChanged(ctx, filePath); err != nil {
		log.Printf("lsp: didChange %s: %v", filePath, err)
	}
}

// DocumentVersion returns the current tracked LSP document version for an open file.
func (m *Manager) DocumentVersion(filePath string) (int, bool) {
	ext := strings.ToLower(filepath.Ext(filePath))
	name, ok := m.extMap[ext]
	if !ok {
		return 0, false
	}
	m.mu.Lock()
	srv, exists := m.servers[name]
	m.mu.Unlock()
	if !exists || !srv.Alive() {
		return 0, false
	}
	return srv.DocumentVersion(filePath)
}

// HasRunningServers reports whether any LSP servers are currently running.
func (m *Manager) HasRunningServers() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.servers {
		if s.Alive() {
			return true
		}
	}
	return false
}

// Extensions returns all file extensions that have a configured server.
func (m *Manager) Extensions() []string {
	exts := make([]string, 0, len(m.extMap))
	for ext := range m.extMap {
		exts = append(exts, ext)
	}
	return exts
}

// CheckProjectLanguages inspects file extensions in the project and logs
// install hints for languages that have a configured LSP server but the
// binary is not available. Called once at startup or first palette open.
func (m *Manager) CheckProjectLanguages(files []string) {
	if len(files) == 0 {
		return
	}

	// Count extensions that have a configured server.
	extCounts := make(map[string]int)
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if _, ok := m.extMap[ext]; ok {
			extCounts[ext]++
		}
	}

	// Check each configured server that matches project files.
	checked := make(map[string]bool)
	for ext, count := range extCounts {
		name, ok := m.extMap[ext]
		if !ok || checked[name] {
			continue
		}
		checked[name] = true

		cfg := m.findConfig(name)
		if cfg == nil {
			continue
		}

		if _, err := resolveCommand(cfg.Command); err != nil {
			log.Printf("lsp: project has %d %s files but %s is not installed. %s",
				count, ext, cfg.Command, installHint(cfg.Name, cfg.Command))
		}
	}
}

// Shutdown gracefully shuts down all running servers.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	servers := make(map[string]*Server, len(m.servers))
	for k, v := range m.servers {
		servers[k] = v
	}
	m.servers = make(map[string]*Server)
	m.mu.Unlock()

	for name, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("lsp: shutdown %s: %v", name, err)
		}
	}
}

func (m *Manager) findConfig(name string) *config.LSPServerConfig {
	for i := range m.configs {
		if m.configs[i].Name == name {
			return &m.configs[i]
		}
	}
	return nil
}
