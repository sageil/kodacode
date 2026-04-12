package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryStore_SaveAndList(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStore(dir)

	mem, err := store.Save("always use snake_case")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if mem.Content != "always use snake_case" {
		t.Errorf("Content = %q, want %q", mem.Content, "always use snake_case")
	}
	if mem.ID == "" {
		t.Error("ID should not be empty")
	}

	memories, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("List returned %d, want 1", len(memories))
	}
	if memories[0].Content != "always use snake_case" {
		t.Errorf("Listed content = %q", memories[0].Content)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStore(dir)

	mem, _ := store.Save("temp memory")
	if err := store.Delete(mem.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	memories, _ := store.List()
	if len(memories) != 0 {
		t.Errorf("List returned %d after delete, want 0", len(memories))
	}
}

func TestMemoryStore_DeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStore(dir)

	if err := store.Delete("nonexistent"); err == nil {
		t.Error("Delete of nonexistent ID should return error")
	}
}

func TestMemoryStore_DeleteRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStore(dir)

	outside := filepath.Join(dir, "outside.md")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside): %v", err)
	}
	if err := store.Delete("../../outside"); err == nil {
		t.Fatal("Delete traversal should return error")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file should remain untouched, stat error = %v", err)
	}
}

func TestMemoryStore_LoadWithBudget(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStore(dir)
	memDir := filepath.Join(dir, ".kodacode", "memories")
	_ = os.MkdirAll(memDir, 0o755)

	// Write files with controlled timestamps.
	write := func(id, content string, age time.Duration) {
		path := filepath.Join(memDir, id+".md")
		_ = os.WriteFile(path, []byte(content), 0o644)
		t := time.Now().Add(-age)
		_ = os.Chtimes(path, t, t)
	}
	write("oldest", "AAAA", 3*time.Hour)
	write("middle", "BBBB", 2*time.Hour)
	write("newest", "CCCC", 1*time.Hour)

	result := store.LoadWithBudget(100)
	if result == "" {
		t.Fatal("LoadWithBudget returned empty")
	}
	if !contains(result, "AAAA") || !contains(result, "BBBB") || !contains(result, "CCCC") {
		t.Errorf("All memories should be included within budget, got: %q", result)
	}

	headerLen := len("# Project Memories\n\n")
	// Budget that fits only 2 memories (4 chars each + one separator).
	result = store.LoadWithBudget(headerLen + len("CCCC") + 1 + len("BBBB"))
	if !contains(result, "CCCC") || !contains(result, "BBBB") || contains(result, "AAAA") {
		t.Errorf("budgeted memories = %q, want newest two only", result)
	}

	// A too-large newest memory should be truncated instead of exceeding budget.
	result = store.LoadWithBudget(headerLen + 2)
	if len(result) > headerLen+2 {
		t.Fatalf("LoadWithBudget exceeded limit: len=%d want<=%d", len(result), headerLen+2)
	}
	if !contains(result, "CC") {
		t.Errorf("truncated newest memory missing, got %q", result)
	}
}

func TestMemoryStore_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := NewMemoryStore(dir)

	memories, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("List returned %d, want 0", len(memories))
	}
	if result := store.LoadWithBudget(1000); result != "" {
		t.Errorf("LoadWithBudget should return empty, got %q", result)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
