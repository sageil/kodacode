package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestHandleStepToolCallDeltaStartsCollectsAndPublishes(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	runner := &TurnRunner{sessions: sessions}
	collector := newStepToolCallCollector(false)
	started := 0

	result, err := runner.handleStepToolCallDelta(context.Background(), "session-1", "turn-1", provider.Event{
		Kind:       provider.EventKindToolCallDelta,
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
		InputDelta: `{"paths":["app.go"]}`,
	}, false, collector, func(toolName string) error {
		started++
		if toolName != tool.ReadToolName {
			t.Fatalf("toolName = %q", toolName)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("handleStepToolCallDelta() error = %v", err)
	}
	if !result.Accepted {
		t.Fatal("result.Accepted = false")
	}
	if started != 1 {
		t.Fatalf("started = %d, want 1", started)
	}
	call := collector.completeToolCall(provider.Event{
		Kind:       provider.EventKindToolCallDone,
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
	})
	if call.Arguments != `{"paths":["app.go"]}` {
		t.Fatalf("call.Arguments = %q", call.Arguments)
	}
}

func TestHandleStepToolCallDeltaAcceptsSequentialCallAfterPriorTool(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	runner := &TurnRunner{sessions: sessions}
	collector := newStepToolCallCollector(false)
	started := 0

	result, err := runner.handleStepToolCallDelta(context.Background(), "session-1", "turn-1", provider.Event{
		Kind:       provider.EventKindToolCallDelta,
		ToolCallID: "call-patch",
		ToolName:   tool.ApplyPatchToolName,
		InputDelta: `{"path":"app.go"}`,
	}, true, collector, func(toolName string) error {
		started++
		return nil
	})
	if err != nil {
		t.Fatalf("handleStepToolCallDelta() error = %v", err)
	}
	if !result.Accepted {
		t.Fatal("result.Accepted = false")
	}
	if started != 1 {
		t.Fatalf("started = %d, want 1", started)
	}
	call := collector.completeToolCall(provider.Event{
		Kind:       provider.EventKindToolCallDone,
		ToolCallID: "call-patch",
		ToolName:   tool.ApplyPatchToolName,
	})
	if call.Arguments != `{"path":"app.go"}` {
		t.Fatalf("call.Arguments = %q", call.Arguments)
	}
}
