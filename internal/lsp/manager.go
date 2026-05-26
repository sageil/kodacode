package lsp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const failureCooldown = 30 * time.Second

var ErrServerNotConfigured = errors.New("no LSP server configured for file type")

type failedEntry struct {
	err error
	at  time.Time
}

type Manager struct {
	root    string
	rootURI string
	configs []ServerConfig

	mu       sync.Mutex
	servers  map[string]*Server
	extMap   map[string]string
	failed   map[string]failedEntry
	starting map[string]*sync.Once
}

func NewManager(root string, configs []ServerConfig) *Manager {
	extMap := make(map[string]string)
	for _, cfg := range configs {
		for _, ext := range cfg.Extensions {
			extMap[strings.ToLower(ext)] = cfg.Name
		}
	}
	return &Manager{
		root:     filepath.Clean(root),
		rootURI:  FileURI(root),
		configs:  append([]ServerConfig(nil), configs...),
		servers:  make(map[string]*Server),
		extMap:   extMap,
		failed:   make(map[string]failedEntry),
		starting: make(map[string]*sync.Once),
	}
}

func (m *Manager) ServerForPath(ctx context.Context, filePath string) (*Server, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	name, ok := m.extMap[ext]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrServerNotConfigured, ext)
	}
	return m.serverByName(ctx, name)
}

func (m *Manager) serverByName(ctx context.Context, name string) (*Server, error) {
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
		srv, err := Start(ctx, *cfg, m.rootURI)
		if err != nil {
			startErr = err
			m.mu.Lock()
			m.failed[name] = failedEntry{err: err, at: time.Now()}
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
	defer m.mu.Unlock()
	srv := m.servers[name]
	if srv == nil {
		return nil, fmt.Errorf("lsp server %q failed to start", name)
	}
	return srv, nil
}

func (m *Manager) EnsureWorkspaceServers(ctx context.Context) {
	seen := map[string]bool{}
	for _, cfg := range m.configs {
		if seen[cfg.Name] || len(cfg.Extensions) == 0 {
			continue
		}
		seen[cfg.Name] = true
		_, _ = m.serverByName(ctx, cfg.Name)
	}
}

func (m *Manager) WorkspaceSymbol(ctx context.Context, query string) ([]SymbolInformation, error) {
	m.EnsureWorkspaceServers(ctx)

	m.mu.Lock()
	servers := make([]*Server, 0, len(m.servers))
	for _, srv := range m.servers {
		if srv.Alive() {
			servers = append(servers, srv)
		}
	}
	m.mu.Unlock()
	if len(servers) == 0 {
		if err := m.workspaceSymbolUnavailableError(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	type result struct {
		symbols []SymbolInformation
		err     error
	}
	ch := make(chan result, len(servers))
	for _, srv := range servers {
		go func(server *Server) {
			symbols, err := server.WorkspaceSymbol(ctx, query)
			ch <- result{symbols: symbols, err: err}
		}(srv)
	}

	var all []SymbolInformation
	errs := make([]error, 0, len(servers))
	for range servers {
		res := <-ch
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		all = append(all, res.symbols...)
	}
	if len(all) == 0 && len(errs) > 0 {
		return nil, joinManagerErrors(errs)
	}
	return all, nil
}

func (m *Manager) NotifyChanged(ctx context.Context, filePath string) error {
	srv, err := m.ServerForPath(ctx, filePath)
	if err != nil {
		return nil
	}
	return srv.NotifyChanged(ctx, filePath)
}

func (m *Manager) NotifyDeleted(filePath string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	name, ok := m.extMap[ext]
	if !ok {
		return nil
	}
	m.mu.Lock()
	srv := m.servers[name]
	m.mu.Unlock()
	if srv == nil || !srv.Alive() {
		return nil
	}
	return srv.CloseDocument(filePath)
}

func (m *Manager) NotifyRenamed(ctx context.Context, oldPath, newPath string) error {
	if err := m.NotifyDeleted(oldPath); err != nil {
		return err
	}
	return m.NotifyChanged(ctx, newPath)
}

func (m *Manager) DocumentVersion(filePath string) (int, bool) {
	ext := strings.ToLower(filepath.Ext(filePath))
	name, ok := m.extMap[ext]
	if !ok {
		return 0, false
	}
	m.mu.Lock()
	srv := m.servers[name]
	m.mu.Unlock()
	if srv == nil || !srv.Alive() {
		return 0, false
	}
	return srv.DocumentVersion(filePath)
}

func (m *Manager) RunningServerNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.servers))
	for name, srv := range m.servers {
		if srv != nil && srv.Alive() {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	servers := make(map[string]*Server, len(m.servers))
	for key, value := range m.servers {
		servers[key] = value
	}
	m.servers = make(map[string]*Server)
	m.mu.Unlock()

	for _, srv := range servers {
		_ = srv.Shutdown(ctx)
	}
}

func (m *Manager) findConfig(name string) *ServerConfig {
	for i := range m.configs {
		if m.configs[i].Name == name {
			return &m.configs[i]
		}
	}
	return nil
}

func (m *Manager) workspaceSymbolUnavailableError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.configs) == 0 {
		return errors.New("no LSP servers are configured for this workspace")
	}
	if len(m.failed) == 0 {
		return nil
	}
	errs := make([]error, 0, len(m.failed))
	for _, failed := range m.failed {
		if failed.err == nil || time.Since(failed.at) >= failureCooldown {
			continue
		}
		errs = append(errs, failed.err)
	}
	return joinManagerErrors(errs)
}

func joinManagerErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		message := strings.TrimSpace(err.Error())
		if message == "" {
			continue
		}
		messages = append(messages, message)
	}
	if len(messages) == 0 {
		return nil
	}
	sort.Strings(messages)
	unique := make([]string, 0, len(messages))
	for _, message := range messages {
		if len(unique) > 0 && unique[len(unique)-1] == message {
			continue
		}
		unique = append(unique, message)
	}
	return errors.New(strings.Join(unique, "; "))
}
