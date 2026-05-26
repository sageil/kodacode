package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorExecuteTestToolUsesRuntimeExecutionContract(t *testing.T) {
	useExecutionRunnerHooks(t, func(_ context.Context, contract executionContract, _ executionRunOptions) (executionRunResult, error) {
		if contract.WorkingDirectory == "" {
			t.Fatal("WorkingDirectory should be set")
		}
		if contract.Timeout != 90*time.Second {
			t.Fatalf("Timeout = %s, want 90s", contract.Timeout)
		}
		return executionRunResult{
			Output:  []byte("ok\tpkg\t0.123s\n"),
			Backend: "test_backend",
		}, nil
	})

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTestTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("MkdirAll(pkg) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "service_test.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(service_test.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args, err := json.Marshal(map[string]any{
		"command": nil,
		"path":    "pkg/service_test.go",
		"filter":  "TestService",
		"timeout": 90000,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TestToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed", result)
	}
	if result.Output != "ok\tpkg\t0.123s" {
		t.Fatalf("Output = %q", result.Output)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil || call.Execution == nil {
		t.Fatalf("call execution state missing: %#v", call)
	}
	if call.Execution.Kind != tool.TestToolName {
		t.Fatalf("Execution.Kind = %q, want %q", call.Execution.Kind, tool.TestToolName)
	}
	if call.Execution.CommandPreview != "go test ./pkg -run 'TestService'" {
		t.Fatalf("CommandPreview = %q", call.Execution.CommandPreview)
	}
	if call.Execution.TimeoutMS != 90000 {
		t.Fatalf("TimeoutMS = %d, want 90000", call.Execution.TimeoutMS)
	}
}

func TestToolExecutorExecuteInvalidTestArgumentsPersistFriendlyError(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTestTool())
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

	args := json.RawMessage(`{"command":"npx jest -t \"ProjectController\" --runInBand","path":"","filter":null,"timeout":120000}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TestToolName,
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result.Status = %q", result.Status)
	}
	if got := result.Error; !strings.Contains(got, "`test` failed.") || !strings.Contains(got, `Example: {"command":null,"path":"internal/tool","filter":null,"timeout":90000}.`) {
		t.Fatalf("result.Error = %q", got)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 || replayed[1].Type != events.TypeToolExecEnd {
		t.Fatalf("events = %#v", replayed)
	}
	payload, ok := replayed[1].Payload.(events.ToolExecEndPayload)
	if !ok {
		t.Fatalf("payload = %#v", replayed[1].Payload)
	}
	if !strings.Contains(payload.Error, "`test` failed.") || !strings.Contains(payload.Error, `Example: {"command":null,"path":"internal/tool","filter":null,"timeout":90000}.`) {
		t.Fatalf("payload.Error = %q", payload.Error)
	}
}

func TestToolExecutorExecuteCanceledTestToolPersistsTerminalToolEvent(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTestTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("MkdirAll(pkg) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "service_test.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(service_test.go) error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	started := make(chan struct{})
	useExecutionRunnerHooks(t, func(ctx context.Context, contract executionContract, _ executionRunOptions) (executionRunResult, error) {
		if got := contract.Command; len(got) == 0 {
			t.Fatalf("execution command = %#v", got)
		}
		select {
		case <-started:
		default:
			close(started)
		}
		<-ctx.Done()
		return executionRunResult{Backend: "test_backend"}, ctx.Err()
	})

	args, err := json.Marshal(map[string]any{
		"command": nil,
		"path":    "pkg/service_test.go",
		"filter":  "TestService",
		"timeout": 90000,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	execCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type executeResult struct {
		result ToolExecutionResult
		err    error
	}
	done := make(chan executeResult, 1)
	go func() {
		result, err := executor.Execute(execCtx, ExecuteToolInput{
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "call-1",
			ToolName:   tool.TestToolName,
			Arguments:  args,
		})
		done <- executeResult{result: result, err: err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for test execution to start")
	}
	cancel()

	var finished executeResult
	select {
	case finished = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canceled test execution to finish")
	}
	if finished.err != nil {
		t.Fatalf("Execute() error = %v", finished.err)
	}
	if finished.result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result.Status = %q, want executed", finished.result.Status)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	toolEndCount := 0
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ToolExecEndPayload:
			toolEndCount++
			if payload.CallID != "call-1" {
				t.Fatalf("CallID = %q, want call-1", payload.CallID)
			}
			if payload.ToolName != tool.TestToolName {
				t.Fatalf("ToolName = %q, want %q", payload.ToolName, tool.TestToolName)
			}
			if payload.Succeeded {
				t.Fatal("Succeeded = true, want false")
			}
			if payload.ExecutionStatus != string(events.ExecutionStatusFailed) {
				t.Fatalf("ExecutionStatus = %q, want failed", payload.ExecutionStatus)
			}
		}
	}
	if toolEndCount != 1 {
		t.Fatalf("tool_exec_end count = %d, want 1", toolEndCount)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil {
		t.Fatal("call state missing")
	}
	if call.Executing {
		t.Fatal("call.Executing = true, want false")
	}
	if !call.Completed {
		t.Fatal("call.Completed = false, want true")
	}
	if call.Execution == nil {
		t.Fatal("call.Execution = nil, want execution state")
	}
	if call.Execution.Executing {
		t.Fatal("call.Execution.Executing = true, want false")
	}
	if !call.Execution.Completed {
		t.Fatal("call.Execution.Completed = false, want true")
	}
}
