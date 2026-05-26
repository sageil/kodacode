package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestAppendStepToolCallBatchPersistsMultiCallBoundary(t *testing.T) {
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

	callIDs, err := runner.appendStepToolCallBatch(context.Background(), "session-1", "turn-1", stepToolBatch{
		StepIndex: 1,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: "search", Arguments: `{}`},
			{CallID: "call-2", ToolName: "read", Arguments: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("appendStepToolCallBatch() error = %v", err)
	}
	if len(callIDs) != 2 || callIDs[0] != "call-1" || callIDs[1] != "call-2" {
		t.Fatalf("callIDs = %v", callIDs)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("events = %#v", replayed)
	}
	payload, ok := replayed[1].Payload.(events.ToolCallBatchPayload)
	if !ok {
		t.Fatalf("payload = %#v", replayed[1].Payload)
	}
	if len(payload.CallIDs) != 2 || payload.CallIDs[0] != "call-1" || payload.CallIDs[1] != "call-2" {
		t.Fatalf("payload.CallIDs = %v", payload.CallIDs)
	}
}

func TestAppendStepToolCallBatchSkipsSingleCallBoundary(t *testing.T) {
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

	callIDs, err := runner.appendStepToolCallBatch(context.Background(), "session-1", "turn-1", stepToolBatch{
		StepIndex: 1,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: "read", Arguments: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("appendStepToolCallBatch() error = %v", err)
	}
	if len(callIDs) != 1 || callIDs[0] != "call-1" {
		t.Fatalf("callIDs = %v", callIDs)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 1 {
		t.Fatalf("events = %#v", replayed)
	}
}
