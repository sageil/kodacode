package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMemoryToolDefinitionMentionsContentLimit(t *testing.T) {
	definition := NewMemoryTool().Definition()
	for _, field := range []string{
		definition.Description,
		definition.ProviderDescription,
		string(definition.InputSchema),
	} {
		if !strings.Contains(field, "2000") {
			t.Fatalf("memory tool definition missing 2000-character limit: %q", field)
		}
	}
}

type stubMemoryManager struct {
	saved   MemoryRecord
	listed  []MemoryRecord
	deleted string
}

func (s *stubMemoryManager) SaveMemory(request MemorySaveRequest) (MemoryRecord, error) {
	s.saved = MemoryRecord{ID: "memory-1", Content: request.Content, UpdatedAt: "2026-04-24T12:00:00Z"}
	return s.saved, nil
}

func (s *stubMemoryManager) ListMemories() ([]MemoryRecord, error) {
	return s.listed, nil
}

func (s *stubMemoryManager) DeleteMemory(request MemoryDeleteRequest) error {
	s.deleted = request.ID
	return nil
}

func TestMemoryToolExecuteSaveUsesMemoryManager(t *testing.T) {
	manager := &stubMemoryManager{}
	result, err := NewMemoryTool().Execute(context.Background(), ExecutionContext{
		MemoryManager: manager,
	}, json.RawMessage(`{"action":"save","content":"Remember the repo uses event sourcing.","id":null}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if manager.saved.ID != "memory-1" || manager.saved.Content == "" {
		t.Fatalf("saved = %#v", manager.saved)
	}
	if !strings.Contains(result.Output, `"id":"memory-1"`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestMemoryToolExecuteListUsesMemoryManager(t *testing.T) {
	manager := &stubMemoryManager{
		listed: []MemoryRecord{{
			ID:      "memory-1",
			Path:    ".kodacode/memories/memory-1.md",
			Content: strings.Repeat("x", memoryListPreviewMaxChars+20),
		}},
	}
	result, err := NewMemoryTool().Execute(context.Background(), ExecutionContext{
		MemoryManager: manager,
	}, json.RawMessage(`{"action":"list","content":null,"id":null}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !containsAll(result.Output,
		`"memories":[`,
		`"memory-1"`,
		`".kodacode/memories/memory-1.md"`,
		`"truncated":true`,
	) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestMemoryToolExecuteRequiresMemoryManager(t *testing.T) {
	_, err := NewMemoryTool().Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"action":"list","content":null,"id":null}`))
	if !errors.Is(err, ErrMemoryManagerRequired) {
		t.Fatalf("Execute() error = %v, want ErrMemoryManagerRequired", err)
	}
}
