package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	websearchsvc "github.com/sageil/kodacode/internal/websearch"
)

type stubToolExecutorWebSearchBackend struct {
	lastReq  websearchsvc.Request
	response websearchsvc.Response
}

func (s *stubToolExecutorWebSearchBackend) ID() string {
	return "exa"
}

func (s *stubToolExecutorWebSearchBackend) Search(_ context.Context, req websearchsvc.Request) (websearchsvc.Response, error) {
	s.lastReq = req
	return s.response, nil
}

func TestToolExecutorExecuteWebSearchRunsWithoutNetworkPermissionPrompt(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWebSearchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	backend := &stubToolExecutorWebSearchBackend{
		response: websearchsvc.Response{
			Provider: "exa",
			Results: []websearchsvc.Result{{
				Title:   "Code Agents Survey",
				URL:     "https://example.com/code-agents",
				Domain:  "example.com",
				Snippet: "A survey of code agents.",
			}},
		},
	}
	service, err := websearchsvc.NewService("exa", backend)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	executor.SetWebSearchService(service)

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
		ToolName:   tool.WebSearchToolName,
		Arguments:  json.RawMessage(`{"query":"code agents","limit":"3"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(string(result.StructuredResult), `"provider":"exa"`) {
		t.Fatalf("structured result = %s", result.StructuredResult)
	}
	if backend.lastReq.Query != "code agents" || backend.lastReq.Limit != 3 {
		t.Fatalf("backend request = %#v", backend.lastReq)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.PendingPermissionOrder) != 0 {
		t.Fatalf("pending permission order = %#v", state.PendingPermissionOrder)
	}
	if len(state.NetworkGrants) != 0 {
		t.Fatalf("network grants = %#v", state.NetworkGrants)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil {
		t.Fatal("tool call missing from session state")
	}
	if !strings.Contains(call.Input, `"provider":"exa"`) {
		t.Fatalf("call.Input = %q, want canonical provider", call.Input)
	}
}
