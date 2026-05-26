package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

func TestToolExecutorExecuteInvalidReadArgumentsPersistSpecificError(t *testing.T) {
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
		Arguments:  json.RawMessage(`{"paths":123}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result.Status = %q", result.Status)
	}
	want := "`read` failed. paths must be an array of strings; got number. Use either path for one file or paths for one or more files; do not send both."
	if result.Error != want {
		t.Fatalf("result.Error = %q, want %q", result.Error, want)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 3 || replayed[2].Type != events.TypeToolExecEnd {
		t.Fatalf("events = %#v", replayed)
	}
	payload, ok := replayed[2].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("payload = %#v", replayed[2].Payload)
	}
	if payload.Error != want {
		t.Fatalf("payload.Error = %q, want %q", payload.Error, want)
	}
}

func TestToolExecutorExecuteReadAllowsExternalDirectoryByPolicy(t *testing.T) {
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
	externalDir := t.TempDir()
	externalPath := filepath.Join(externalDir, "notes.txt")
	if err := os.WriteFile(externalPath, []byte("external content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	externalScope, err := workspace.New(externalDir)
	if err != nil {
		t.Fatalf("workspace.New(externalDir) error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		ExternalDirectory: permissionpolicy.SubjectRules{
			{Pattern: externalScope.Root() + "/**", Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args, err := json.Marshal(map[string]any{
		"paths": []string{externalPath},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Output, "external content") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestToolExecutorExecuteReadDeniesExternalDirectoryByPolicy(t *testing.T) {
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
	externalDir := t.TempDir()
	externalPath := filepath.Join(externalDir, "notes.txt")
	if err := os.WriteFile(externalPath, []byte("external content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	externalScope, err := workspace.New(externalDir)
	if err != nil {
		t.Fatalf("workspace.New(externalDir) error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		ExternalDirectory: permissionpolicy.SubjectRules{
			{Pattern: externalScope.Root() + "/**", Action: permissionpolicy.ActionDeny},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args, err := json.Marshal(map[string]any{
		"paths": []string{externalPath},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if result.FailureClass != toolFailureClassPermissionDenied {
		t.Fatalf("failure class = %q", result.FailureClass)
	}
	if !strings.Contains(result.Error, "permissions.external_directory") {
		t.Fatalf("error = %q", result.Error)
	}
}
