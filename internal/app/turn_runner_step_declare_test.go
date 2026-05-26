package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestAppendStepToolCallDeclaredPersistsReplayMetadata(t *testing.T) {
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

	signature := []byte("sig-1")
	if err := runner.appendStepToolCallDeclared(context.Background(), "session-1", "turn-1", stepToolCall{
		CallID:                 "call-1",
		ToolName:               "read",
		Arguments:              `{"paths":["app.go"]}`,
		GoogleThoughtSignature: signature,
		OpenAIReasoningContent: "inspect first",
	}); err != nil {
		t.Fatalf("appendStepToolCallDeclared() error = %v", err)
	}
	signature[0] = 'X'

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("events = %#v", replayed)
	}
	payload, ok := replayed[1].Payload.(events.ToolCallDeclaredPayload)
	if !ok {
		t.Fatalf("payload = %#v", replayed[1].Payload)
	}
	if payload.CallID != "call-1" || payload.ToolName != "read" || payload.Input != `{"paths":["app.go"]}` {
		t.Fatalf("payload = %#v", payload)
	}
	if got := string(payload.GoogleThoughtSignature); got != "sig-1" {
		t.Fatalf("GoogleThoughtSignature = %q", got)
	}
	if payload.OpenAIReasoningContent != "inspect first" {
		t.Fatalf("OpenAIReasoningContent = %q", payload.OpenAIReasoningContent)
	}
}
