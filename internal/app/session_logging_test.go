package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/observability"
)

func TestSessionServiceDebugLogIncludesTestToolAndCancelEvents(t *testing.T) {
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	sessions.SetLogger(logger.With("component", "session_service"))

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeToolCallDeclared,
		Payload: events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "test",
			Input:    `{"command":"npm test"}`,
		},
	}); err != nil {
		t.Fatalf("append(tool_call_declared) error = %v", err)
	}
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeExecutionStarted,
		Payload: events.ExecutionStartedPayload{
			ExecutionID: "exec-1",
			ToolCallID:  "call-1",
			ToolName:    "test",
			Input:       `{"command":"npm test"}`,
		},
	}); err != nil {
		t.Fatalf("append(execution_started) error = %v", err)
	}
	if err := sessions.publishEphemeral("session-1", "turn-1", events.TypeExecutionOutput, events.ExecutionOutputPayload{
		ExecutionID: "exec-1",
		ToolCallID:  "call-1",
		Stream:      "combined",
		Chunk:       "running\n",
	}); err != nil {
		t.Fatalf("publishEphemeral(execution_output) error = %v", err)
	}
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeToolExecEnd,
		Payload: events.ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        "test",
			ExecutionID:     "exec-1",
			ExecutionStatus: string(events.ExecutionStatusCompleted),
			Succeeded:       true,
			Output:          "ok\tpkg\t0.123s",
			CommandActions:  []string{"exec"},
		},
	}); err != nil {
		t.Fatalf("append(tool_exec_end) error = %v", err)
	}
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-2",
		Type:      events.TypeTurnCanceled,
		Payload:   events.TurnCanceledPayload{Message: "turn canceled by user"},
	}); err != nil {
		t.Fatalf("append(turn_canceled) error = %v", err)
	}

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	for _, needle := range []string{
		"session event appended",
		"event_type=tool_call_declared",
		"event_type=execution_started",
		"session event published",
		"event_type=execution_output",
		"event_type=tool_exec_end",
		"event_type=turn_canceled",
		"tool_name=test",
	} {
		if !strings.Contains(debugLog, needle) {
			t.Fatalf("debug log missing %q: %q", needle, debugLog)
		}
	}
}
