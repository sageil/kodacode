package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorExecuteReadsMultipleAuthorizedFiles(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello docs\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "read",
		Arguments:  json.RawMessage(`{"paths":["app.go","README.md"]}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || !strings.Contains(result.Output, "=== app.go ===") || !strings.Contains(result.Output, "1: package main") || !strings.Contains(result.Output, "=== README.md ===") || !strings.Contains(result.Output, "1: hello docs") {
		t.Fatalf("result = %#v", result)
	}
}

func TestToolExecutorExecuteReadPartialMultiFileFailureIsSuccessfulWarning(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  json.RawMessage(`{"paths":["app.go","missing.go"]}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" || !strings.Contains(result.Output, "Partial read: 1 of 2 requested files read successfully.") || !strings.Contains(result.Output, "=== app.go ===") {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || !call.Succeeded || call.Error != "" || len(call.ObservedResources) != 1 {
		t.Fatalf("call = %#v", call)
	}
}
