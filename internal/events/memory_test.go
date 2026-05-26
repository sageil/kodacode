package events

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreAppendAssignsSequenceAndReplayReadsFromCursor(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	first, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantPreviewDelta,
		Payload:   AssistantPreviewDeltaPayload{Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	second, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantCommit,
		Payload:   AssistantCommitPayload{Content: "done"},
	})
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if first.Sequence != 0 || second.Sequence != 1 {
		t.Fatalf("sequences = %d, %d; want 0, 1", first.Sequence, second.Sequence)
	}

	got, err := store.Replay(ctx, Query{SessionID: "session-1", AfterSequence: 0})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("replay len = %d, want 1", len(got))
	}
	if got[0].Sequence != 1 || got[0].Type != TypeAssistantCommit {
		t.Fatalf("replay event = %#v", got[0])
	}
}

func TestMemoryStoreWatchReplaysThenStreamsWithoutGap(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	first, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantPreviewDelta,
		Payload:   AssistantPreviewDeltaPayload{Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := store.Watch(watchCtx, Query{
		SessionID:     "session-1",
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	second, err := store.Append(ctx, Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantCommit,
		Payload:   AssistantCommitPayload{Content: "done"},
	})
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}

	gotFirst := mustReceiveEvent(t, stream)
	gotSecond := mustReceiveEvent(t, stream)

	if gotFirst.Sequence != first.Sequence || gotSecond.Sequence != second.Sequence {
		t.Fatalf("watch sequence order = %d, %d; want %d, %d", gotFirst.Sequence, gotSecond.Sequence, first.Sequence, second.Sequence)
	}
}

func mustReceiveEvent(t *testing.T, stream <-chan Event) Event {
	t.Helper()

	select {
	case event := <-stream:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}
