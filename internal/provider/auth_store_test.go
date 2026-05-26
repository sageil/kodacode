package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthStoreGetParsesOAuthEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.yaml")
	content := strings.Join([]string{
		"anthropic:",
		`  type: "api"`,
		`  access: "anthropic-key"`,
		"openai:",
		`  type: "oauth"`,
		`  access: "access-token"`,
		`  refresh: "refresh-token"`,
		`  expires: "12345"`,
		`  account_id: "acct_123"`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	entry := NewAuthStoreAt(path).Get("openai")
	if entry == nil {
		t.Fatal("Get() = nil, want entry")
	}
	if entry.Type != AuthTypeOAuth || entry.Access != "access-token" || entry.Refresh != "refresh-token" {
		t.Fatalf("entry = %#v", entry)
	}
	if entry.Expires != 12345 || entry.AccountID != "acct_123" {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestAuthStoreSetPreservesOtherProviders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.yaml")
	store := NewAuthStoreAt(path)

	if err := store.Set("anthropic", AuthEntry{Type: AuthTypeAPI, Access: "anthropic-key"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Set("openai", AuthEntry{
		Type:      AuthTypeOAuth,
		Access:    "next-access",
		Refresh:   "next-refresh",
		Expires:   67890,
		AccountID: "acct_456",
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	anthropic := store.Get("anthropic")
	if anthropic == nil || anthropic.Type != AuthTypeAPI || anthropic.Access != "anthropic-key" {
		t.Fatalf("anthropic entry = %#v", anthropic)
	}
	openai := store.Get("openai")
	if openai == nil || openai.Access != "next-access" || openai.Refresh != "next-refresh" {
		t.Fatalf("openai entry = %#v", openai)
	}
}
