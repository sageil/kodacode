package provider

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/sageil/kodacode/v1/internal/config"
)

// AuthType discriminates auth entry variants.
type AuthType string

const (
	AuthTypeAPI   AuthType = "api"
	AuthTypeOAuth AuthType = "oauth"
)

// AuthEntry is a single credential stored in auth.yaml.
type AuthEntry struct {
	Type      AuthType `yaml:"type"`
	Access    string   `yaml:"access,omitempty"`
	Refresh   string   `yaml:"refresh,omitempty"`
	Expires   int64    `yaml:"expires,omitempty"`
	Project   string   `yaml:"project,omitempty"`
	AccountID string   `yaml:"account_id,omitempty"` // OpenAI: ChatGPT account ID from JWT
}

// AuthStore manages credential storage in auth.yaml (mode 0600).
type AuthStore struct {
	mu   sync.RWMutex
	path string
}

// NewAuthStore creates an AuthStore at ~/.config/kodacode/auth.yaml.
func NewAuthStore() *AuthStore {
	return &AuthStore{path: filepath.Join(config.ConfigDir(), "auth.yaml")}
}

// NewAuthStoreAt creates an AuthStore with a custom path (for testing).
func NewAuthStoreAt(path string) *AuthStore {
	return &AuthStore{path: path}
}

// Get returns the auth entry for a provider, or nil if not found.
func (s *AuthStore) Get(providerID string) *AuthEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.read()
	entry, ok := entries[providerID]
	if !ok {
		return nil
	}
	return &entry
}

// Set writes or updates an auth entry.
func (s *AuthStore) Set(providerID string, entry AuthEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.read()
	entries[providerID] = entry
	return s.write(entries)
}

// Remove deletes an auth entry.
func (s *AuthStore) Remove(providerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.read()
	if _, ok := entries[providerID]; !ok {
		return nil
	}
	delete(entries, providerID)
	return s.write(entries)
}

func (s *AuthStore) read() map[string]AuthEntry {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return map[string]AuthEntry{}
	}
	// Check file permissions and tighten if too permissive.
	if info, err := os.Stat(s.path); err == nil {
		mode := info.Mode().Perm()
		if mode&0o077 != 0 {
			log.Printf("auth: %s has mode %04o (should be 0600), fixing permissions", s.path, mode)
			_ = os.Chmod(s.path, 0o600)
		}
	}
	var entries map[string]AuthEntry
	if yaml.Unmarshal(data, &entries) != nil {
		return map[string]AuthEntry{}
	}
	return entries
}

func (s *AuthStore) write(entries map[string]AuthEntry) error {
	data, err := yaml.Marshal(entries)
	if err != nil {
		return fmt.Errorf("auth: marshal: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("auth: mkdir: %w", err)
	}
	return os.WriteFile(s.path, data, 0o600)
}

