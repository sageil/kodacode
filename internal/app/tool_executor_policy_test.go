package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorRejectsDisallowedTool(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWriteTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	sessionID := "session-1"
	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     sessionID,
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:    sessionID,
		TurnID:       "turn-1",
		ToolCallID:   "call-1",
		ToolName:     "write",
		AllowedTools: []string{"read"},
		Arguments:    json.RawMessage(`{"path":"file.txt","content":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || !strings.Contains(result.Error, ErrToolNotAllowed.Error()) {
		t.Fatalf("result = %#v", result)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: sessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 || replayed[1].Type != events.TypeToolExecEnd {
		t.Fatalf("events = %#v", replayed)
	}
}

func TestToolAllowedDistinguishesNilFromExplicitEmpty(t *testing.T) {
	if !toolAllowed("read", nil) {
		t.Fatal("toolAllowed(read, nil) = false, want true")
	}
	if toolAllowed("read", []string{}) {
		t.Fatal("toolAllowed(read, empty) = true, want false")
	}
	if !toolAllowed("mcp_docs__search", []string{"mcp:*"}) {
		t.Fatal("toolAllowed(mcp_docs__search, wildcard) = false, want true")
	}
}
