package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestSQLiteToolResultBlobStoreSaveAndLoad(t *testing.T) {
	store, err := events.NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("events.NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	blobs := NewSQLiteToolResultBlobStore(store)
	original := strings.Repeat("x", toolResultBlobInlineLimit+512)

	ref, err := blobs.Save(context.Background(), ToolResultBlobKey{
		SessionID: "session-1",
		TurnID:    "turn-1",
		CallID:    "call-1",
		Stream:    "output",
	}, original)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if ref == nil {
		t.Fatal("Save() ref = nil, want blob reference")
	}
	if ref.Ref != "session-1/turn-1/call-1-output.txt" {
		t.Fatalf("Save() ref = %q", ref.Ref)
	}

	loaded, err := blobs.Load(context.Background(), ref.Ref)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != original {
		t.Fatalf("Load() content length = %d, want %d", len(loaded), len(original))
	}
}
