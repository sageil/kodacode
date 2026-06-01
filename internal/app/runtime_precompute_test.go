package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

type recordingPrecomputeHook struct {
	calls chan runtimePrecomputeHint
	err   error
}

func (h *recordingPrecomputeHook) Precompute(ctx context.Context, hint runtimePrecomputeHint) error {
	select {
	case h.calls <- hint:
	case <-ctx.Done():
		return ctx.Err()
	}
	return h.err
}

func TestRuntimeRunSessionTurnTriggersPrecomputeAfterCompletedTurn(t *testing.T) {
	hook := &recordingPrecomputeHook{calls: make(chan runtimePrecomputeHint, 1)}
	runtime := newRuntimeWithClient(t, &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	})
	runtime.precomputeHooks = []runtimePrecomputeHook{hook}
	workspaceRoot := t.TempDir()
	resolvedWorkspaceRoot, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", workspaceRoot, err)
	}

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: workspaceRoot,
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	hint := waitForPrecomputeHint(t, hook.calls)
	if hint.SessionID != result.SessionID || hint.TurnID != result.TurnID {
		t.Fatalf("hint ids = %#v, want session %q turn %q", hint, result.SessionID, result.TurnID)
	}
	if hint.WorkspaceRoot != resolvedWorkspaceRoot {
		t.Fatalf("hint workspace = %q, want %q", hint.WorkspaceRoot, resolvedWorkspaceRoot)
	}
	if hint.Status != TurnRunStatusCompleted {
		t.Fatalf("hint status = %q, want completed", hint.Status)
	}
	if hint.ChangedAt.IsZero() {
		t.Fatal("hint ChangedAt is zero")
	}
	if !hasString(hint.Tags, "turn:completed") {
		t.Fatalf("hint tags = %#v, want turn:completed", hint.Tags)
	}
}

func TestRuntimeRunSessionTurnIgnoresPrecomputeFailure(t *testing.T) {
	hook := &recordingPrecomputeHook{
		calls: make(chan runtimePrecomputeHint, 1),
		err:   errors.New("precompute failed"),
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	})
	runtime.precomputeHooks = []runtimePrecomputeHook{hook}

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "say hello",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted || result.AssistantText != "hello" {
		t.Fatalf("result = %#v", result)
	}
	_ = waitForPrecomputeHint(t, hook.calls)
}

func waitForPrecomputeHint(t *testing.T, calls <-chan runtimePrecomputeHint) runtimePrecomputeHint {
	t.Helper()
	select {
	case hint := <-calls:
		return hint
	case <-time.After(time.Second):
		t.Fatal("precompute hook was not called")
		return runtimePrecomputeHint{}
	}
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
