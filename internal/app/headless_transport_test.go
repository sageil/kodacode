package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func TestHeadlessRuntimeTransportOpensSessionStartsTurnAndStreamsEvents(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	})
	transport, err := NewHeadlessRuntimeTransport(runtime)
	if err != nil {
		t.Fatalf("NewHeadlessRuntimeTransport() error = %v", err)
	}

	open, err := transport.Execute(context.Background(), HeadlessCommand{
		Type: HeadlessCommandOpenSession,
		OpenSession: &HeadlessOpenSessionCommand{
			WorkspaceRoot: t.TempDir(),
		},
	})
	if err != nil {
		t.Fatalf("Execute(open) error = %v", err)
	}
	if open.SessionID == "" || open.OpenSession == nil {
		t.Fatalf("open result = %#v", open)
	}

	watchCtx, cancelWatch := context.WithCancel(context.Background())
	defer cancelWatch()
	stream, err := transport.Watch(watchCtx, HeadlessWatchRequest{SessionID: open.SessionID})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	start, err := transport.Execute(context.Background(), HeadlessCommand{
		Type: HeadlessCommandStartTurn,
		StartTurn: &HeadlessStartTurnCommand{
			SessionID: open.SessionID,
			UserText:  "say hello",
		},
	})
	if err != nil {
		t.Fatalf("Execute(start) error = %v", err)
	}
	if start.Run == nil || start.Run.Status != TurnRunStatusCompleted || start.Run.AssistantText != "hello" {
		t.Fatalf("start result = %#v", start)
	}
	if start.TurnID == "" {
		t.Fatal("start result missing generated turn id")
	}

	event := waitForHeadlessEvent(t, stream, events.TypeTurnDone)
	if event.Event.SessionID != open.SessionID || event.Event.TurnID != start.TurnID {
		t.Fatalf("turn_done event ids = %#v, want session %q turn %q", event.Event, open.SessionID, start.TurnID)
	}
}

func TestHeadlessRuntimeTransportRejectsUnsupportedCommand(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	transport, err := NewHeadlessRuntimeTransport(runtime)
	if err != nil {
		t.Fatalf("NewHeadlessRuntimeTransport() error = %v", err)
	}

	_, err = transport.Execute(context.Background(), HeadlessCommand{Type: "unknown"})
	if !errors.Is(err, ErrHeadlessCommandUnsupported) {
		t.Fatalf("Execute() error = %v, want ErrHeadlessCommandUnsupported", err)
	}
}

func TestHeadlessRuntimeTransportWatchRequiresSessionID(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	transport, err := NewHeadlessRuntimeTransport(runtime)
	if err != nil {
		t.Fatalf("NewHeadlessRuntimeTransport() error = %v", err)
	}

	_, err = transport.Watch(context.Background(), HeadlessWatchRequest{})
	if !errors.Is(err, ErrHeadlessSessionIDRequired) {
		t.Fatalf("Watch() error = %v, want ErrHeadlessSessionIDRequired", err)
	}
}

func waitForHeadlessEvent(t *testing.T, stream <-chan HeadlessEvent, typ events.Type) HeadlessEvent {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				t.Fatalf("headless event stream closed before %s", typ)
			}
			if event.Event.Type == typ {
				return event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for headless event %s", typ)
		}
	}
}
