package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestRuntimeRestoreSessionTurnWritesRestoresCompletedTurn(t *testing.T) {
	runtime := newWriteRestoreRuntime(t, nil)
	root := t.TempDir()
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	executeReadTool(t, runtime, sessionID, "turn-1", "call-read-1", "notes.txt")
	executeReadToolWithArgs(t, runtime, sessionID, "turn-1", "call-read-2", map[string]any{
		"paths":  []string{"notes.txt"},
		"offset": 200,
		"limit":  200,
	})
	executeReadToolWithArgs(t, runtime, sessionID, "turn-1", "call-read-3", map[string]any{
		"paths":  []string{"notes.txt"},
		"offset": 400,
		"limit":  200,
	})
	executeWriteTool(t, runtime, sessionID, "turn-1", "call-1", "notes.txt", "after\n")
	appendTurnDoneEvent(t, runtime.Sessions, sessionID, "turn-1")

	result, err := runtime.RestoreSessionTurnWrites(context.Background(), RestoreSessionTurnWritesInput{
		SessionID:    sessionID,
		SourceTurnID: "turn-1",
	})
	if err != nil {
		t.Fatalf("RestoreSessionTurnWrites() error = %v", err)
	}
	if len(result.Paths) != 1 || !strings.HasSuffix(result.Paths[0], "/notes.txt") {
		t.Fatalf("result paths = %#v", result.Paths)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "before\n" {
		t.Fatalf("restored content = %q", string(data))
	}

	replayed, err := runtime.Store.Replay(context.Background(), events.Query{SessionID: sessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	payload, ok := replayed[len(replayed)-1].Payload.(events.WorkspaceWriteRestoredPayload)
	if !ok {
		t.Fatalf("restore payload = %T", replayed[len(replayed)-1].Payload)
	}
	if payload.SourceTurnID != "turn-1" {
		t.Fatalf("source turn id = %q", payload.SourceTurnID)
	}
	if len(payload.Restores) != 1 || payload.Restores[0].CallID != "call-1" || !strings.HasSuffix(payload.Restores[0].Path, "/notes.txt") || !payload.Restores[0].ExistedBefore {
		t.Fatalf("restores = %#v", payload.Restores)
	}
}

func TestRuntimeRestoreSessionTurnWritesReplaysMultipleWritesInReverse(t *testing.T) {
	runtime := newWriteRestoreRuntime(t, nil)
	root := t.TempDir()
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	executeReadTool(t, runtime, sessionID, "turn-1", "call-read", "notes.txt")
	executeWriteTool(t, runtime, sessionID, "turn-1", "call-1", "notes.txt", "b\n")
	executeReadTool(t, runtime, sessionID, "turn-1", "call-read-2", "notes.txt")
	executeWriteTool(t, runtime, sessionID, "turn-1", "call-2", "notes.txt", "c\n")
	appendTurnDoneEvent(t, runtime.Sessions, sessionID, "turn-1")

	result, err := runtime.RestoreSessionTurnWrites(context.Background(), RestoreSessionTurnWritesInput{
		SessionID:    sessionID,
		SourceTurnID: "turn-1",
	})
	if err != nil {
		t.Fatalf("RestoreSessionTurnWrites() error = %v", err)
	}
	if len(result.Paths) != 1 || !strings.HasSuffix(result.Paths[0], "/notes.txt") {
		t.Fatalf("result paths = %#v", result.Paths)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "a\n" {
		t.Fatalf("restored content = %q", string(data))
	}

	replayed, err := runtime.Store.Replay(context.Background(), events.Query{SessionID: sessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	payload, ok := replayed[len(replayed)-1].Payload.(events.WorkspaceWriteRestoredPayload)
	if !ok {
		t.Fatalf("restore payload = %T", replayed[len(replayed)-1].Payload)
	}
	if len(payload.Restores) != 2 || payload.Restores[0].CallID != "call-2" || payload.Restores[1].CallID != "call-1" {
		t.Fatalf("restores = %#v", payload.Restores)
	}
}

func TestRuntimeRestoreSessionTurnWritesRejectsDriftedFiles(t *testing.T) {
	runtime := newWriteRestoreRuntime(t, nil)
	root := t.TempDir()
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	executeReadTool(t, runtime, sessionID, "turn-1", "call-read-1", "notes.txt")
	executeReadToolWithArgs(t, runtime, sessionID, "turn-1", "call-read-2", map[string]any{
		"paths":  []string{"notes.txt"},
		"offset": 200,
		"limit":  200,
	})
	executeReadToolWithArgs(t, runtime, sessionID, "turn-1", "call-read-3", map[string]any{
		"paths":  []string{"notes.txt"},
		"offset": 400,
		"limit":  200,
	})
	executeWriteTool(t, runtime, sessionID, "turn-1", "call-1", "notes.txt", "after\n")
	appendTurnDoneEvent(t, runtime.Sessions, sessionID, "turn-1")
	if err := os.WriteFile(path, []byte("manual drift\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(drift) error = %v", err)
	}

	_, err = runtime.RestoreSessionTurnWrites(context.Background(), RestoreSessionTurnWritesInput{
		SessionID:    sessionID,
		SourceTurnID: "turn-1",
	})
	if !errors.Is(err, ErrWriteRestoreConflict) {
		t.Fatalf("RestoreSessionTurnWrites() error = %v, want ErrWriteRestoreConflict", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if string(data) != "manual drift\n" {
		t.Fatalf("content after failed restore = %q", string(data))
	}

	replayed, err := runtime.Store.Replay(context.Background(), events.Query{SessionID: sessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	for _, event := range replayed {
		if event.Type == events.TypeWorkspaceWriteRestored {
			t.Fatalf("unexpected restore event = %#v", event)
		}
	}
}

func TestRuntimeRestoreSessionTurnWritesLoadsLargeBeforeContentFromBlob(t *testing.T) {
	blobStore := newTestSQLiteBlobStore(t)
	runtime := newWriteRestoreRuntime(t, blobStore)
	root := t.TempDir()
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	path := filepath.Join(root, "notes.txt")
	before := strings.Repeat("before line\n", 500)
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	executeReadTool(t, runtime, sessionID, "turn-1", "call-read-1", "notes.txt")
	executeReadToolWithArgs(t, runtime, sessionID, "turn-1", "call-read-2", map[string]any{
		"paths":  []string{"notes.txt"},
		"offset": 200,
		"limit":  200,
	})
	executeReadToolWithArgs(t, runtime, sessionID, "turn-1", "call-read-3", map[string]any{
		"paths":  []string{"notes.txt"},
		"offset": 400,
		"limit":  200,
	})
	executeWriteTool(t, runtime, sessionID, "turn-1", "call-1", "notes.txt", "after\n")
	appendTurnDoneEvent(t, runtime.Sessions, sessionID, "turn-1")

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.WriteMutation == nil || !call.WriteMutation.BeforeTruncated || call.WriteMutation.BeforeBlob == nil {
		t.Fatalf("write mutation = %#v", call)
	}

	if _, err := runtime.RestoreSessionTurnWrites(context.Background(), RestoreSessionTurnWritesInput{
		SessionID:    sessionID,
		SourceTurnID: "turn-1",
	}); err != nil {
		t.Fatalf("RestoreSessionTurnWrites() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != before {
		t.Fatalf("restored content length = %d, want %d", len(data), len(before))
	}
}

func newWriteRestoreRuntime(t *testing.T, blobs ToolResultBlobStore) *Runtime {
	t.Helper()
	store := events.NewMemoryStore()
	var (
		sessions *SessionService
		err      error
	)
	if blobs != nil {
		sessions, err = NewSessionServiceWithBlobs(store, blobs)
	} else {
		sessions, err = NewSessionService(store)
	}
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	tools, err := NewToolExecutor(sessions, tool.NewReadTool(), tool.NewWriteTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	return &Runtime{
		Store:    store,
		Sessions: sessions,
		Tools:    tools,
	}
}

func executeWriteTool(t *testing.T, runtime *Runtime, sessionID, turnID, callID, path, content string) {
	t.Helper()
	args, err := json.Marshal(map[string]string{
		"path":    path,
		"content": content,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	result, err := runtime.Tools.Execute(context.Background(), ExecuteToolInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		ToolCallID: callID,
		ToolName:   "write",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
}

func executeReadTool(t *testing.T, runtime *Runtime, sessionID, turnID, callID, path string) {
	t.Helper()
	executeReadToolWithArgs(t, runtime, sessionID, turnID, callID, map[string]any{
		"paths": []string{path},
	})
}

func executeReadToolWithArgs(t *testing.T, runtime *Runtime, sessionID, turnID, callID string, payload map[string]any) {
	t.Helper()
	args, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	result, err := runtime.Tools.Execute(context.Background(), ExecuteToolInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		ToolCallID: callID,
		ToolName:   tool.ReadToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if (result.Status != ToolExecutionStatusExecuted && result.Status != ToolExecutionStatusReused) || result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
}

func appendTurnDoneEvent(t *testing.T, sessions *SessionService, sessionID, turnID string) {
	t.Helper()
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnDone,
		Payload:   events.TurnDonePayload{},
	}); err != nil {
		t.Fatalf("append(turn_done) error = %v", err)
	}
}
