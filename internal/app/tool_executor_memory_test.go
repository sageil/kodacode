package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorExecuteMemoryToolPersistsProjectMemory(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewMemoryTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	executor.SetMemoryService(NewMemoryService())

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	saveArgs, err := json.Marshal(map[string]any{
		"action":  "save",
		"content": "Use explicit runtime permission prompts for risky execution.",
		"id":      nil,
	})
	if err != nil {
		t.Fatalf("json.Marshal(save) error = %v", err)
	}
	saveResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.MemoryToolName,
		Arguments:  saveArgs,
	})
	if err != nil {
		t.Fatalf("Execute(save) error = %v", err)
	}
	if saveResult.Status != ToolExecutionStatusExecuted || !strings.Contains(saveResult.Output, `"memory":{"id":"`) {
		t.Fatalf("save result = %#v", saveResult)
	}

	listResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.MemoryToolName,
		Arguments:  json.RawMessage(`{"action":"list","content":null,"id":null}`),
	})
	if err != nil {
		t.Fatalf("Execute(list) error = %v", err)
	}
	if listResult.Status != ToolExecutionStatusExecuted {
		t.Fatalf("list result = %#v", listResult)
	}
	if !strings.Contains(listResult.Output, "Use explicit runtime permission prompts") {
		t.Fatalf("list output = %q", listResult.Output)
	}
}
