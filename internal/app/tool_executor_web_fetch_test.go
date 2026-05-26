package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorExecuteWebFetchRequestsNetworkPermissionWithHumanPreview(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWebFetchTool())
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

	args := json.RawMessage(`{"url":"https://example.com/docs","method":null,"headers":null,"body":null,"format":null,"selector":null}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.WebFetchToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	request := state.PendingPermissions[result.PendingRequestID]
	if request == nil {
		t.Fatalf("pending permission %q not found", result.PendingRequestID)
	}
	if request.Kind != events.PermissionRequestKindNetwork {
		t.Fatalf("Kind = %q, want network", request.Kind)
	}
	if request.Path != "example.com" {
		t.Fatalf("Path = %q, want example.com", request.Path)
	}
	if request.Command != "web_fetch https://example.com/docs" {
		t.Fatalf("Command = %q", request.Command)
	}
	if strings.Contains(request.Command, "{") {
		t.Fatalf("Command = %q, want human preview", request.Command)
	}
}

func TestToolExecutorExecuteWebFetchShowsMethodInPreviewForNonGETRequests(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWebFetchTool())
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

	args := json.RawMessage(`{"url":"https://example.com/api/tasks","method":"POST","headers":null,"body":"{}","format":null,"selector":null}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.WebFetchToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusPending || result.PendingRequestID == "" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	request := state.PendingPermissions[result.PendingRequestID]
	if request == nil {
		t.Fatalf("pending permission %q not found", result.PendingRequestID)
	}
	if request.Command != "web_fetch POST https://example.com/api/tasks" {
		t.Fatalf("Command = %q", request.Command)
	}
}

func TestToolExecutorExecuteWebFetchUsesSessionNetworkGrantAfterApproval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("hello from server"))
	}))
	defer server.Close()

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWebFetchTool())
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

	args := json.RawMessage(`{"url":"` + server.URL + `","method":null,"headers":null,"body":null,"format":"text","selector":null}`)
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.WebFetchToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusPending || first.PendingRequestID == "" {
		t.Fatalf("first = %#v", first)
	}
	if _, err := sessions.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Decision:  events.PermissionDecisionApproved,
		Scope:     events.PermissionScopeSession,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-2",
		ToolCallID: "call-2",
		ToolName:   tool.WebFetchToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Error != "" {
		t.Fatalf("second = %#v", second)
	}
	if second.Output != "hello from server" {
		t.Fatalf("Output = %q", second.Output)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.NetworkGrants) != 1 {
		t.Fatalf("network grants = %#v", state.NetworkGrants)
	}
}

func TestToolExecutorExecuteWebFetchAllowsByURLPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("allowed by policy"))
	}))
	defer server.Close()

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		WebFetch: permissionpolicy.SubjectRules{
			{Pattern: server.URL + "/docs*", Action: permissionpolicy.ActionAllow},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWebFetchTool())
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
		ToolName:   tool.WebFetchToolName,
		Arguments:  json.RawMessage(`{"url":"` + server.URL + `/docs?id=1","method":null,"headers":null,"body":null,"format":"text","selector":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != "allowed by policy" {
		t.Fatalf("Output = %q", result.Output)
	}
}

func TestToolExecutorExecuteWebFetchDeniesByHostPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be reached when policy denies")
	}))
	defer server.Close()

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if err := sessions.SetPermissionPolicy(permissionpolicy.Config{
		NetworkTarget: permissionpolicy.SubjectRules{
			{Pattern: strings.TrimPrefix(server.URL, "http://"), Action: permissionpolicy.ActionDeny},
		},
	}); err != nil {
		t.Fatalf("SetPermissionPolicy() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWebFetchTool())
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
		ToolName:   tool.WebFetchToolName,
		Arguments:  json.RawMessage(`{"url":"` + server.URL + `/docs","method":null,"headers":null,"body":null,"format":"text","selector":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Error, "permissions.network_target") {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestToolExecutorExecuteWebFetchBypassesApprovalInFullAccessMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("full access"))
	}))
	defer server.Close()

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewWebFetchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:      "session-1",
		WorkspaceRoot:  root,
		PermissionMode: PermissionModeFullAccess,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.WebFetchToolName,
		Arguments:  json.RawMessage(`{"url":"` + server.URL + `","method":null,"headers":null,"body":null,"format":"text","selector":null}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if result.Output != "full access" {
		t.Fatalf("Output = %q", result.Output)
	}
}
