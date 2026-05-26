package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func TestHandleStepAssistantDeltaAppendsPreviewBeforeTools(t *testing.T) {
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
	segment := "hello"

	result, err := runner.handleStepAssistantDelta("session-1", "turn-1", provider.Event{
		Kind:           provider.EventKindAssistantDelta,
		AssistantDelta: " world",
	}, false, false, &segment)
	if err != nil {
		t.Fatalf("handleStepAssistantDelta() error = %v", err)
	}
	if !result.Accepted {
		t.Fatal("result.Accepted = false")
	}
	if result.CompletionTokens != provider.EstimateTextTokens(" world") {
		t.Fatalf("CompletionTokens = %d", result.CompletionTokens)
	}
	if segment != "hello world" {
		t.Fatalf("segment = %q", segment)
	}
}

func TestHandleStepAssistantDeltaIgnoresTextAfterToolCalls(t *testing.T) {
	runner := &TurnRunner{}
	segment := "hello"

	result, err := runner.handleStepAssistantDelta("session-1", "turn-1", provider.Event{
		Kind:           provider.EventKindAssistantDelta,
		AssistantDelta: " ignored",
	}, false, true, &segment)
	if err != nil {
		t.Fatalf("handleStepAssistantDelta() error = %v", err)
	}
	if !result.Accepted {
		t.Fatal("result.Accepted = false")
	}
	if segment != "hello" {
		t.Fatalf("segment = %q", segment)
	}
}

func TestHandleStepAssistantDeltaContinuesAfterBatchableResultsButAccountsTokens(t *testing.T) {
	runner := &TurnRunner{}
	segment := "hello"

	result, err := runner.handleStepAssistantDelta("session-1", "turn-1", provider.Event{
		Kind:           provider.EventKindAssistantDelta,
		AssistantDelta: " stop",
	}, true, false, &segment)
	if err != nil {
		t.Fatalf("handleStepAssistantDelta() error = %v", err)
	}
	if !result.Accepted {
		t.Fatal("result.Accepted = false")
	}
	if result.CompletionTokens != provider.EstimateTextTokens(" stop") {
		t.Fatalf("CompletionTokens = %d", result.CompletionTokens)
	}
	if segment != "hello" {
		t.Fatalf("segment = %q", segment)
	}
}
