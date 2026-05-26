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

func TestNewRuntimeToolExecutorNilBackgroundPreservesSQLiteBackgroundStore(t *testing.T) {
	useExecutionRunnerHooks(t, func(context.Context, executionContract, executionRunOptions) (executionRunResult, error) {
		t.Fatal("foreground execution runner should not be used for server intent")
		return executionRunResult{}, nil
	})
	useBackgroundExecutionRunnerHooks(t, func(context.Context, executionContract, executionBackgroundRunOptions) (executionBackgroundHandle, error) {
		readyCh := make(chan executionBackgroundReadyEvent, 1)
		exitCh := make(chan executionBackgroundExitEvent, 1)
		readyCh <- executionBackgroundReadyEvent{
			Message: "Local: http://127.0.0.1:5173/",
			Port:    5173,
		}
		go func() {
			time.Sleep(10 * time.Millisecond)
			exitCh <- executionBackgroundExitEvent{
				RunResult: executionRunResult{
					Backend:  "background_process",
					ExitCode: intPointer(0),
				},
			}
			close(exitCh)
		}()
		return executionBackgroundHandle{
			PID:             5252,
			ProcessIdentity: "identity-5252",
			Ready:           readyCh,
			Exited:          exitCh,
		}, nil
	})

	store := newTestSQLiteStore(t)
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := newRuntimeToolExecutor(runtimeToolExecutorConfig{
		Sessions:     sessions,
		Execution:    defaultExecutionConfig(),
		RuntimeTools: []tool.Tool{tool.NewBashTool()},
	})
	if err != nil {
		t.Fatalf("newRuntimeToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	clientDir := filepath.Join(root, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(clientDir, "package.json"), []byte(`{"scripts":{"dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	args := json.RawMessage(`{"cmd":"npm run dev","workdir":"client"}`)
	first, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(first) error = %v", err)
	}
	if first.Status != ToolExecutionStatusPending || first.PendingRequestID == "" {
		t.Fatalf("first = %#v", first)
	}
	if _, err := sessions.ResolvePermission(context.Background(), ResolvePermissionInput{
		SessionID:         "session-1",
		TurnID:            "turn-1",
		RequestID:         first.PendingRequestID,
		ExecutionDecision: events.ExecutionApprovalDecisionAccept,
	}); err != nil {
		t.Fatalf("ResolvePermission() error = %v", err)
	}

	second, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
		Arguments:  args,
	})
	if err != nil {
		t.Fatalf("Execute(second) error = %v", err)
	}
	if second.Status != ToolExecutionStatusExecuted || second.Error != "" {
		t.Fatalf("second = %#v", second)
	}
	if !strings.Contains(second.Output, "Started server in background (pid 5252).") {
		t.Fatalf("output = %q", second.Output)
	}
}
