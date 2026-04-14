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

func TestManager_RunningServerNames(t *testing.T) {
	m := NewManager(nil)

	m.mu.Lock()
	m.servers["eslint"] = &Server{
		cfg:    config.LSPServerConfig{Name: "eslint"},
		client: &Client{},
	}
	m.servers["tsserver"] = &Server{
		cfg:    config.LSPServerConfig{Name: "tsserver"},
		client: &Client{},
	}
	deadDone := make(chan struct{})
	close(deadDone)
	m.servers["dead"] = &Server{
		cfg:    config.LSPServerConfig{Name: "dead"},
		client: &Client{done: deadDone},
	}
	m.mu.Unlock()

	got := m.RunningServerNames()
	want := []string{"eslint", "tsserver"}
	if len(got) != len(want) {
		t.Fatalf("len(RunningServerNames()) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RunningServerNames()[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}
