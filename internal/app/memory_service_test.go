package app

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/tool"
)

func TestMemoryServiceRoundTripsMemoriesAndBuildsPromptFragment(t *testing.T) {
	service := NewMemoryService()
	root := t.TempDir()
	store := service.Store(root)
	if store == nil {
		t.Fatal("Store() returned nil")
	}

	record, err := store.SaveMemory(tool.MemorySaveRequest{
		Content: "The runtime is the authority for orchestration and permissions.",
	})
	if err != nil {
		t.Fatalf("SaveMemory() error = %v", err)
	}

	memories, err := store.ListMemories()
	if err != nil {
		t.Fatalf("ListMemories() error = %v", err)
	}
	if len(memories) != 1 || memories[0].ID != record.ID {
		t.Fatalf("memories = %#v", memories)
	}
	if record.Path == "" || memories[0].Path == "" {
		t.Fatalf("memory paths should be populated: record=%#v memories=%#v", record, memories)
	}

	fragment, ok, err := service.PromptFragment(root)
	if err != nil {
		t.Fatalf("PromptFragment() error = %v", err)
	}
	if !ok {
		t.Fatal("PromptFragment() ok = false, want true")
	}
	if !strings.Contains(fragment.Content, "Project memory:") || !strings.Contains(fragment.Content, record.Content) {
		t.Fatalf("fragment = %#v", fragment)
	}
}

func TestMemoryServiceRejectsOversizedMemoryContent(t *testing.T) {
	service := NewMemoryService()
	store := service.Store(t.TempDir())
	if store == nil {
		t.Fatal("Store() returned nil")
	}

	_, err := store.SaveMemory(tool.MemorySaveRequest{
		Content: strings.Repeat("x", tool.MemoryContentMaxChars+1),
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("SaveMemory() error = %v, want size error", err)
	}
}
