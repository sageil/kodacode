package provider

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/sageil/kodacode/internal/configdir"
)

type AuthType string

const (
	AuthTypeAPI   AuthType = "api"
	AuthTypeOAuth AuthType = "oauth"
)

type AuthEntry struct {
	Type          AuthType
	Access        string
	Refresh       string
	Expires       int64
	Project       string
	AccountID     string
	ClientVersion string
}

type AuthStore struct {
	mu   sync.RWMutex
	path string
}

func NewAuthStore() *AuthStore {
	return &AuthStore{path: filepath.Join(providerConfigDir(), "auth.yaml")}
}

func NewAuthStoreAt(path string) *AuthStore {
	return &AuthStore{path: path}
}

func (s *AuthStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *AuthStore) Get(providerID string) *AuthEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	document := s.read()
	fields, ok := document[providerID]
	if !ok {
		return nil
	}
	entry, ok := decodeAuthEntry(fields)
	if !ok {
		return nil
	}
	return &entry
}

func (s *AuthStore) Set(providerID string, entry AuthEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	document := s.read()
	fields := document[providerID]
	document[providerID] = mergeAuthEntry(fields, entry)
	return s.write(document)
}

func (s *AuthStore) Remove(providerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	document := s.read()
	delete(document, providerID)
	return s.write(document)
}

func (s *AuthStore) read() authDocument {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return authDocument{}
	}
	if info, statErr := os.Stat(s.path); statErr == nil && info.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(s.path, 0o600)
	}
	return parseAuthDocument(data)
}

func (s *AuthStore) write(document authDocument) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, renderAuthDocument(document), 0o600)
}

func providerConfigDir() string {
	return configdir.Root()
}
