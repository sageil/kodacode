package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	searchsvc "github.com/sageil/kodacode/internal/search"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorExecuteRunsAuthorizedSearchAndEmitsExecutionEvents(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewSearchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\n// TODO wire search\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "search",
		Arguments:  json.RawMessage(`{"query":"TODO","path":".","glob":"*.go","case_sensitive":true,"max_matches":10}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || !strings.Contains(result.Output, "main.go:3:// TODO wire search") {
		t.Fatalf("result = %#v", result)
	}
}

func TestToolExecutorExecuteNormalizesImplicitSearchModeToHybridWhenEmbeddingsAreConfigured(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewSearchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	executor.SetSearchService(searchsvc.NewService(&stubToolExecutorSearchEmbedder{
		vectors: map[string][]float32{
			"permission":                       {1, 0},
			"notes.txt:1\nauthorization guard": {1, 0},
		},
	}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, t.TempDir(), nil))

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("authorization guard\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "search",
		Arguments:  json.RawMessage(`{"query":"permission","path":".","glob":"","case_sensitive":false,"max_matches":10}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Output != "[semantic] notes.txt:1:authorization guard" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.TODO(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil {
		t.Fatal("tool call missing from session state")
	}
	if !strings.Contains(call.Input, `"mode":"hybrid"`) {
		t.Fatalf("call.Input = %q, want normalized hybrid mode", call.Input)
	}
}

func TestToolExecutorExecuteNormalizesImplicitRegexSearchModeToLexical(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewSearchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	executor.SetSearchService(searchsvc.NewService(&stubToolExecutorSearchEmbedder{}, provider.ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"}, 0, t.TempDir(), nil))

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.txt"), []byte("TODO-123 fix search\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "search",
		Arguments:  json.RawMessage(`{"query":"TODO-[0-9]{3}","path":".","regex":true,"case_sensitive":true,"max_matches":10}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Output != "tasks.txt:1:TODO-123 fix search" {
		t.Fatalf("result = %#v", result)
	}

	state, err := sessions.Snapshot(context.TODO(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil {
		t.Fatal("tool call missing from session state")
	}
	if !strings.Contains(call.Input, `"mode":"lexical"`) {
		t.Fatalf("call.Input = %q, want normalized lexical mode", call.Input)
	}
}

func TestToolExecutorExecuteSearchRejectsFilesystemRootWithoutPermissionPrompt(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewSearchTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if _, err := sessions.CreateSession(context.TODO(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.TODO(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "search",
		Arguments:  json.RawMessage(`{"query":"TODO","path":"` + filesystemRootPathForSearchExecutorTest(t) + `","glob":"*.go","case_sensitive":true,"max_matches":10}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.FailureClass != toolFailureClassInvalidArguments {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Error, `use "." or a workspace-relative path for project-wide search`) {
		t.Fatalf("result.Error = %q", result.Error)
	}

	state, err := sessions.Snapshot(context.TODO(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.PendingPermissionOrder) != 0 {
		t.Fatalf("pending permission order = %#v", state.PendingPermissionOrder)
	}
}

func filesystemRootPathForSearchExecutorTest(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd() error = %v", err)
		}
		volume := filepath.VolumeName(wd)
		if volume == "" {
			volume = "C:"
		}
		return volume + `\`
	}
	return string(os.PathSeparator)
}

type stubToolExecutorSearchEmbedder struct {
	vectors map[string][]float32
}

func (s *stubToolExecutorSearchEmbedder) Embed(_ context.Context, req provider.EmbeddingRequest) ([][]float32, error) {
	out := make([][]float32, len(req.Inputs))
	for idx, input := range req.Inputs {
		vector := s.vectors[input]
		if vector == nil {
			vector = []float32{0, 0}
		}
		out[idx] = append([]float32(nil), vector...)
	}
	return out, nil
}
