package app

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestRuntimeRunLocalShellCommandCompletesTurn(t *testing.T) {
	useLocalExecRunner(t)

	runtime := newRuntimeWithClient(t, &fakeProvider{})
	workspaceRoot := t.TempDir()

	session, err := runtime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession() error = %v", err)
	}

	result, err := runtime.RunLocalShellCommand(context.Background(), RunLocalShellCommandInput{
		SessionID: session.SessionID,
		TurnID:    "turn-1",
		Command:   "printf 'hello\\n'",
	})
	if err != nil {
		t.Fatalf("RunLocalShellCommand() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), session.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusCompleted {
		t.Fatalf("turn = %#v", turn)
	}
	if !strings.Contains(turn.UserText, "$ printf 'hello\\n'") || !strings.Contains(turn.UserText, "hello") {
		t.Fatalf("user text = %q", turn.UserText)
	}
}

func TestRuntimeResolveSessionTurnCompletesLocalShellAfterExecutionApproval(t *testing.T) {
	var runs int
	useExecutionRunnerHooks(t,
		func(context.Context, executionContract, executionRunOptions) (executionRunResult, error) {
			runs++
			return executionRunResult{
				Output:   []byte("network ok\n"),
				ExitCode: intPointer(0),
			}, nil
		},
	)

	runtime := newRuntimeWithClient(t, &fakeProvider{})
	workspaceRoot := t.TempDir()

	session, err := runtime.OpenWorkspaceSession(context.Background(), workspaceRoot, nil, false)
	if err != nil {
		t.Fatalf("OpenWorkspaceSession() error = %v", err)
	}

	first, err := runtime.RunLocalShellCommand(context.Background(), RunLocalShellCommandInput{
		SessionID: session.SessionID,
		TurnID:    "turn-1",
		Command:   "curl https://example.com",
	})
	if err != nil {
		t.Fatalf("RunLocalShellCommand() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingRequestID == "" || first.PendingExecution == nil {
		t.Fatalf("first = %#v", first)
	}
	if runs != 0 {
		t.Fatalf("runs = %d, want no execution before approval", runs)
	}

	resolved, err := runtime.ResolveSessionTurn(context.Background(), ResolveSessionTurnInput{
		SessionID:           session.SessionID,
		TurnID:              "turn-1",
		PermissionRequestID: first.PendingRequestID,
		ExecutionDecision:   events.ExecutionApprovalDecisionAccept,
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurn() error = %v", err)
	}
	if resolved.Status != TurnRunStatusCompleted {
		t.Fatalf("resolved = %#v", resolved)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), session.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || !strings.Contains(turn.UserText, "network ok") {
		t.Fatalf("turn = %#v", turn)
	}
}

func intPointer(value int) *int {
	return &value
}
