package tool

import (
	"errors"
	"testing"
)

type stubMemoryStore struct {
	saved   []string
	entries []MemoryEntry
	deleted []string
}

type invalidMemoryStore struct{}

func (s *stubMemoryStore) Save(content string) error {
	s.saved = append(s.saved, content)
	return nil
}

func (s *stubMemoryStore) List() ([]MemoryEntry, error) {
	return append([]MemoryEntry(nil), s.entries...), nil
}

func (s *stubMemoryStore) Delete(id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *invalidMemoryStore) Save(content string) error { return nil }

func (s *invalidMemoryStore) List() ([]MemoryEntry, error) { return nil, nil }

func (s *invalidMemoryStore) Delete(id string) error {
	return errors.New(`invalid memory id "bad id"`)
}

func TestExecuteMemory_DeleteRejectsPathLikeID(t *testing.T) {
	store := &stubMemoryStore{}
	result, err := executeMemory(store, []byte(`{"action":"delete","id":"../../secret"}`))
	if err != nil {
		t.Fatalf("executeMemory(delete): err = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("executeMemory(delete) returned nil result")
	}
	if result.ErrorCode != ErrCodeInvalidArgs {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, ErrCodeInvalidArgs)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("Delete called with %v, want no store call on invalid id", store.deleted)
	}
}

func TestExecuteMemory_DeleteRejectsMalformedBareID(t *testing.T) {
	store := &stubMemoryStore{}
	result, err := executeMemory(store, []byte(`{"action":"delete","id":"bad id"}`))
	if err != nil {
		t.Fatalf("executeMemory(delete malformed): err = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("executeMemory(delete malformed) returned nil result")
	}
	if result.ErrorCode != ErrCodeInvalidArgs {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, ErrCodeInvalidArgs)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("Delete called with %v, want no store call on invalid id", store.deleted)
	}
}

func TestExecuteMemory_DeleteMapsStoreValidationErrorToInvalidArgs(t *testing.T) {
	store := &invalidMemoryStore{}
	result, err := executeMemory(store, []byte(`{"action":"delete","id":"bad-id"}`))
	if err != nil {
		t.Fatalf("executeMemory(delete store validation): err = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("executeMemory(delete store validation) returned nil result")
	}
	if result.ErrorCode != ErrCodeInvalidArgs {
		t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, ErrCodeInvalidArgs)
	}
}

func TestExecuteMemory_ListFormatsEntries(t *testing.T) {
	store := &stubMemoryStore{
		entries: []MemoryEntry{
			{ID: "one", Content: "remember this"},
			{ID: "two", Content: "and this"},
		},
	}
	result, err := executeMemory(store, []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("executeMemory(list): err = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("executeMemory(list) returned nil result")
	}
	if want := "## one\nremember this\n\n## two\nand this"; result.Output != want {
		t.Fatalf("Output = %q, want %q", result.Output, want)
	}
}
