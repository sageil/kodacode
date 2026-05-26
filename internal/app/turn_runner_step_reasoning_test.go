package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestHandleStepReasoningDeltaAppendsDurableReasoningAndCollectorReplay(t *testing.T) {
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
	collector := newStepToolCallCollector(true)

	result, err := runner.handleStepReasoningDelta(context.Background(), "session-1", "turn-1", provider.Event{
		Kind:               provider.EventKindReasoningDelta,
		ReasoningDelta:     "inspect first",
		ReasoningSegmentID: "segment-1",
	}, false, collector)
	if err != nil {
		t.Fatalf("handleStepReasoningDelta() error = %v", err)
	}
	if !result.Accepted || !result.DurableProgress {
		t.Fatalf("result = %#v", result)
	}
	if result.CompletionTokens != provider.EstimateTextTokens("inspect first") {
		t.Fatalf("CompletionTokens = %d", result.CompletionTokens)
	}
	call := collector.completeToolCall(provider.Event{
		Kind:       provider.EventKindToolCallDone,
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
	})
	if call.OpenAIReasoningContent != "inspect first" {
		t.Fatalf("OpenAIReasoningContent = %q", call.OpenAIReasoningContent)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(replayed) != 2 || replayed[1].Type != events.TypeReasoningDelta {
		t.Fatalf("events = %#v", replayed)
	}
}

func TestHandleStepReasoningDeltaContinuesAfterBatchableResultsButAccountsTokens(t *testing.T) {
	runner := &TurnRunner{}
	collector := newStepToolCallCollector(true)

	result, err := runner.handleStepReasoningDelta(context.Background(), "session-1", "turn-1", provider.Event{
		Kind:           provider.EventKindReasoningDelta,
		ReasoningDelta: "stop here",
	}, true, collector)
	if err != nil {
		t.Fatalf("handleStepReasoningDelta() error = %v", err)
	}
	if !result.Accepted || result.DurableProgress {
		t.Fatalf("result = %#v", result)
	}
	if result.CompletionTokens != provider.EstimateTextTokens("stop here") {
		t.Fatalf("CompletionTokens = %d", result.CompletionTokens)
	}
	call := collector.completeToolCall(provider.Event{
		Kind:       provider.EventKindToolCallDone,
		ToolCallID: "call-1",
		ToolName:   tool.ReadToolName,
	})
	if call.OpenAIReasoningContent != "" {
		t.Fatalf("OpenAIReasoningContent = %q", call.OpenAIReasoningContent)
	}
}
