package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyConfiguredRetentionPurgesExpiredSearchCache(t *testing.T) {
	searchDir := filepath.Join(t.TempDir(), "search")
	oldSearch := filepath.Join(searchDir, "workspace-old", "files", "old.json")
	freshSearch := filepath.Join(searchDir, "workspace-fresh", "files", "fresh.json")
	if err := os.MkdirAll(filepath.Dir(oldSearch), 0o755); err != nil {
		t.Fatalf("MkdirAll(oldSearch) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(freshSearch), 0o755); err != nil {
		t.Fatalf("MkdirAll(freshSearch) error = %v", err)
	}
	if err := os.WriteFile(oldSearch, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile(oldSearch) error = %v", err)
	}
	if err := os.WriteFile(freshSearch, []byte(`{"fresh":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile(freshSearch) error = %v", err)
	}
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(oldSearch, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes(oldSearch) error = %v", err)
	}

	if err := applyConfiguredRetention(context.Background(), Config{
		Retention: RetentionConfig{ExpiryDays: 7},
		Search:    SearchConfig{IndexDir: searchDir},
	}, nil); err != nil {
		t.Fatalf("applyConfiguredRetention() error = %v", err)
	}

	if _, err := os.Stat(oldSearch); !os.IsNotExist(err) {
		t.Fatalf("Stat(oldSearch) error = %v, want os.ErrNotExist", err)
	}
	if _, err := os.Stat(freshSearch); err != nil {
		t.Fatalf("Stat(freshSearch) error = %v", err)
	}
}
