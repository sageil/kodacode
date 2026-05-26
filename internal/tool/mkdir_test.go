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

func TestMkdirToolPathRequestsDeclaresWriteAccess(t *testing.T) {
	requests, err := NewMkdirTool().PathRequests(json.RawMessage(`{"path":"build/cache"}`))
	if err != nil {
		t.Fatalf("PathRequests() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Access != workspace.AccessWrite || requests[0].Path != "build/cache" {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestMkdirToolDefinitionRequiresPathOnly(t *testing.T) {
	definition := NewMkdirTool().Definition()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(schema.Properties) != 1 {
		t.Fatalf("properties = %#v", schema.Properties)
	}
	if !slices.Equal(schema.Required, []string{"path"}) {
		t.Fatalf("required = %#v", schema.Required)
	}
}

func TestMkdirToolExecuteCreatesDirectory(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	result, err := NewMkdirTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"build"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasPrefix(result.Output, "created directory ") || !strings.HasSuffix(result.Output, "/build") {
		t.Fatalf("output = %q", result.Output)
	}
	info, err := os.Stat(filepath.Join(root, "build"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("created path is not a directory")
	}
}

func TestMkdirToolExecuteCreatesParentDirectories(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewMkdirTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"build/cache/output"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "build", "cache", "output"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("created path is not a directory")
	}
}

func TestMkdirToolExecuteFailsWhenPathExistsAsFile(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	filePath := filepath.Join(root, "build")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err = NewMkdirTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"build"}`))
	if !errors.Is(err, ErrMkdirPathExistsFile) {
		t.Fatalf("Execute() error = %v, want ErrMkdirPathExistsFile", err)
	}
}

func TestMkdirToolExecuteReturnsAlreadyExistsForExistingDirectory(t *testing.T) {
	root := t.TempDir()
	scope, err := workspace.New(root)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "build"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	result, err := NewMkdirTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"build"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasPrefix(result.Output, "directory already exists: ") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestMkdirToolExecuteCreatesExternalPath(t *testing.T) {
	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}
	external := filepath.Join(t.TempDir(), "outside-dir")

	result, err := NewMkdirTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{"path":"`+external+`"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, external) {
		t.Fatalf("output = %q", result.Output)
	}
	info, err := os.Stat(external)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", external)
	}
}

func TestMkdirToolExecuteRequiresPathField(t *testing.T) {
	scope, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	_, err = NewMkdirTool().Execute(context.TODO(), ExecutionContext{Workspace: scope}, json.RawMessage(`{}`))
	if !errors.Is(err, ErrMkdirPathRequired) {
		t.Fatalf("Execute() error = %v, want ErrMkdirPathRequired", err)
	}
}
