package tui

import (
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

func TestWaitForEventCmdBatchesAvailableEvents(t *testing.T) {
	stream := make(chan events.Event, 2)
	stream <- draftEvent(1, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "hel",
	})
	stream <- draftEvent(2, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "lo",
	})

	msg := waitForEventCmd(stream, 7)()
	batch, ok := msg.(watchEventsMsg)
	if !ok {
		t.Fatalf("waitForEventCmd() msg = %T, want watchEventsMsg", msg)
	}
	if batch.id != 7 {
		t.Fatalf("batch.id = %d, want 7", batch.id)
	}
	if batch.closed {
		t.Fatalf("batch.closed = true, want false")
	}
	if len(batch.events) != 2 {
		t.Fatalf("len(batch.events) = %d, want 2", len(batch.events))
	}
}

func TestWaitForEventCmdReportsClosedAfterBufferedEvents(t *testing.T) {
	stream := make(chan events.Event, 1)
	stream <- draftEvent(1, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "done",
	})
	close(stream)

	msg := waitForEventCmd(stream, 9)()
	batch, ok := msg.(watchEventsMsg)
	if !ok {
		t.Fatalf("waitForEventCmd() msg = %T, want watchEventsMsg", msg)
	}
	if batch.id != 9 {
		t.Fatalf("batch.id = %d, want 9", batch.id)
	}
	if !batch.closed {
		t.Fatalf("batch.closed = false, want true")
	}
	if len(batch.events) != 1 {
		t.Fatalf("len(batch.events) = %d, want 1", len(batch.events))
	}
}

func TestWaitForEventCmdBatchesStreamingEventsArrivingWithinWindow(t *testing.T) {
	stream := make(chan events.Event)
	go func() {
		stream <- draftEvent(1, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
			Content: "hel",
		})
		time.Sleep(5 * time.Millisecond)
		stream <- draftEvent(2, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
			Content: "lo",
		})
		close(stream)
	}()

	msg := waitForEventCmd(stream, 13)()
	batch, ok := msg.(watchEventsMsg)
	if !ok {
		t.Fatalf("waitForEventCmd() msg = %T, want watchEventsMsg", msg)
	}
	if batch.id != 13 {
		t.Fatalf("batch.id = %d, want 13", batch.id)
	}
	if !batch.closed {
		t.Fatalf("batch.closed = false, want true")
	}
	if len(batch.events) != 2 {
		t.Fatalf("len(batch.events) = %d, want 2", len(batch.events))
	}
}

func TestWaitForEventCmdFlushesContextCompactionStartedImmediately(t *testing.T) {
	stream := make(chan events.Event, 2)
	stream <- draftEvent(1, events.TypeContextCompactionStarted, "session-1", "turn-1", events.ContextCompactionStartedPayload{
		Scope:                  events.CompactionScopeHistory,
		InputLimitTokens:       3072,
		TriggerTokens:          2560,
		TargetTokens:           2048,
		EstimatedRequestTokens: 3600,
	})
	stream <- draftEvent(2, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", testHistoryContinuationPayload(
		"Compaction Summary:\n## Critical Context\n- earlier work compacted",
		"",
		events.HistoryContinuationUpdateReasonTokenPressure,
		"turn-0",
		1,
		1,
	))

	firstMsg := waitForEventCmd(stream, 11)()
	first, ok := firstMsg.(watchEventMsg)
	if !ok {
		t.Fatalf("first waitForEventCmd() msg = %T, want watchEventMsg", firstMsg)
	}
	if first.id != 11 {
		t.Fatalf("first.id = %d, want 11", first.id)
	}
	if !first.open {
		t.Fatalf("first.open = false, want true")
	}
	if first.event.Type != events.TypeContextCompactionStarted {
		t.Fatalf("first.event.Type = %q, want %q", first.event.Type, events.TypeContextCompactionStarted)
	}

	secondMsg := waitForEventCmd(stream, 11)()
	second, ok := secondMsg.(watchEventMsg)
	if !ok {
		t.Fatalf("second waitForEventCmd() msg = %T, want watchEventMsg", secondMsg)
	}
	if second.event.Type != events.TypeSessionHistoryContinuationUpdated {
		t.Fatalf("second.event.Type = %q, want %q", second.event.Type, events.TypeSessionHistoryContinuationUpdated)
	}
}

func TestShouldSyncInspectorForEvent(t *testing.T) {
	tests := []struct {
		name string
		typ  events.Type
		want bool
	}{
		{name: "assistant preview delta", typ: events.TypeAssistantPreviewDelta, want: false},
		{name: "assistant preview reset", typ: events.TypeAssistantPreviewReset, want: false},
		{name: "reasoning delta", typ: events.TypeReasoningDelta, want: false},
		{name: "tool call delta", typ: events.TypeToolCallDelta, want: false},
		{name: "tool exec output", typ: events.TypeToolExecOutput, want: false},
		{name: "execution output", typ: events.TypeExecutionOutput, want: false},
		{name: "tool end", typ: events.TypeToolExecEnd, want: true},
		{name: "permission requested", typ: events.TypePermissionRequested, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSyncInspectorForEvent(events.Event{Type: tt.typ})
			if got != tt.want {
				t.Fatalf("shouldSyncInspectorForEvent(%q) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestShouldSyncTranscriptForEvent(t *testing.T) {
	tests := []struct {
		name string
		typ  events.Type
		want bool
	}{
		{name: "session configured", typ: events.TypeSessionConfigured, want: false},
		{name: "model updated", typ: events.TypeSessionModelRouteUpdated, want: false},
		{name: "history checkpoint", typ: events.TypeSessionHistoryCheckpoint, want: false},
		{name: "assistant preview delta", typ: events.TypeAssistantPreviewDelta, want: true},
		{name: "assistant preview reset", typ: events.TypeAssistantPreviewReset, want: true},
		{name: "context compaction started", typ: events.TypeContextCompactionStarted, want: false},
		{name: "provider usage recorded", typ: events.TypeTurnProviderUsageRecorded, want: false},
		{name: "provider usage reported", typ: events.TypeTurnProviderUsageReported, want: false},
		{name: "tool call delta", typ: events.TypeToolCallDelta, want: false},
		{name: "tool exec output", typ: events.TypeToolExecOutput, want: false},
		{name: "execution output", typ: events.TypeExecutionOutput, want: false},
		{name: "background observed", typ: events.TypeExecutionBackgroundObserved, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSyncTranscriptForEvent(events.Event{Type: tt.typ})
			if got != tt.want {
				t.Fatalf("shouldSyncTranscriptForEvent(%q) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}
