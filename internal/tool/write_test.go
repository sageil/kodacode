package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/workspace"
)

func TestWriteToolPathRequestsDeclaresWriteAccess(t *testing.T) {
	requests, err := NewWriteTool().PathRequests(json.RawMessage(`{"path":"notes.txt","content":"hello"}`))
	if err != nil {
		t.Fatalf("PathRequests() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Access != workspace.AccessWrite || requests[0].Path != "notes.txt" {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestWriteToolDefinitionRequiresAllSchemaProperties(t *testing.T) {
	definition := NewWriteTool().Definition()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(schema.Properties) != 2 {
		t.Fatalf("properties = %#v", schema.Properties)
	}
	if !slices.Contains(schema.Required, "path") || !slices.Contains(schema.Required, "content") {
		t.Fatalf("required = %#v", schema.Required)
	}
	if !strings.Contains(definition.Description, "submitted `content` becomes the whole file") {
		t.Fatalf("description missing whole-file replacement warning: %q", definition.Description)
	}
	if !strings.Contains(definition.Description, "omitted text is deleted") {
		t.Fatalf("description missing deletion warning: %q", definition.Description)
	}
	if !strings.Contains(definition.Description, "Prefer `apply_patch` for localized edits.") {
		t.Fatalf("description missing apply_patch guidance: %q", definition.Description)
	}
	if !strings.Contains(definition.ProviderDescription, "submitted `content` becomes the whole file") {
		t.Fatalf("provider description missing deletion warning: %q", definition.ProviderDescription)
	}
	if !strings.Contains(string(schema.Properties["content"]), "This replaces the whole file; omitted text is deleted.") {
		t.Fatalf("content schema missing whole-file replacement warning: %s", string(schema.Properties["content"]))
	}
}

func TestWriteToolExecuteWritesAuthorizedFile(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewWriteTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"notes.txt","content":"hello\n"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasPrefix(result.Output, "wrote 6 bytes to ") || !strings.HasSuffix(result.Output, "/notes.txt") {
		t.Fatalf("output = %q", result.Output)
	}

	content, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "hello\n" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestWriteToolExecuteWritesExternalPath(t *testing.T) {
	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	external := filepath.Join(t.TempDir(), "outside.txt")

	result, err := NewWriteTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"`+external+`","content":"hello"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasSuffix(result.Output, "/outside.txt") {
		t.Fatalf("output = %q", result.Output)
	}
	content, err := os.ReadFile(external)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestWriteToolExecuteRequiresContentField(t *testing.T) {
	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewWriteTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"notes.txt"}`))
	if !errors.Is(err, ErrWriteContentRequired) {
		t.Fatalf("Execute() error = %v, want ErrWriteContentRequired", err)
	}
}

func TestWriteToolExecutePreservesExistingFileMode(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	path := filepath.Join(root, "script.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho old\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := NewWriteTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"script.sh","content":"#!/bin/sh\necho new\n"}`)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("mode = %#o, want %#o", got, 0o755)
	}
}
