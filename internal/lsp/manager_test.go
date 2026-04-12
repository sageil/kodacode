package lsp

import (
	"context"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
)

func TestManager_FailureCooldown(t *testing.T) {
	cfg := config.LSPServerConfig{
		Name:       "fake-lsp",
		Command:    "nonexistent-binary-that-will-fail",
		Extensions: []string{".fake"},
	}
	m := NewManager([]config.LSPServerConfig{cfg})
	ctx := context.Background()

	_, err1 := m.ServerFor(ctx, ".fake", "file:///tmp")
	if err1 == nil {
		t.Fatal("expected error on first start, got nil")
	}

	_, err2 := m.ServerFor(ctx, ".fake", "file:///tmp")
	if err2 == nil {
		t.Fatal("expected cached error within cooldown, got nil")
	}
	if err2.Error() != err1.Error() {
		t.Errorf("expected same cached error, got %q vs %q", err2, err1)
	}

	m.mu.Lock()
	entry := m.failed["fake-lsp"]
	entry.at = time.Now().Add(-failureCooldown - time.Second)
	m.failed["fake-lsp"] = entry
	m.mu.Unlock()

	_, err3 := m.ServerFor(ctx, ".fake", "file:///tmp")
	if err3 == nil {
		t.Fatal("expected error on retry after cooldown, got nil")
	}
	if err3.Error() == err1.Error() && err3 == err1 {
		t.Error("expected a fresh start attempt after cooldown, but got the exact same error object")
	}
}
