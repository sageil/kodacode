package service

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSSEQueuePushNeverBlocks(t *testing.T) {
	q := NewSSEQueue()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10000; i++ {
			q.Push(SSEEvent{Type: fmt.Sprintf("ev-%d", i)})
		}
		q.Close()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Push blocked for more than 2s")
	}

	var count int
	for range q.C() {
		count++
	}
	if count != 10000 {
		t.Fatalf("got %d events, want 10000", count)
	}
}

func TestSSEQueueOrderPreserved(t *testing.T) {
	q := NewSSEQueue()
	const n = 1000
	for i := 0; i < n; i++ {
		q.Push(SSEEvent{Type: fmt.Sprintf("ev-%d", i)})
	}
	q.Close()

	for i := 0; i < n; i++ {
		ev, ok := <-q.C()
		if !ok {
			t.Fatalf("channel closed after %d events, want %d", i, n)
		}
		want := fmt.Sprintf("ev-%d", i)
		if ev.Type != want {
			t.Fatalf("event %d: got %q, want %q", i, ev.Type, want)
		}
	}
	if _, ok := <-q.C(); ok {
		t.Fatal("expected channel to be closed after draining all events")
	}
}

func TestSSEQueueCloseFlushesAndClosesChannel(t *testing.T) {
	q := NewSSEQueue()
	const n = 100
	for i := 0; i < n; i++ {
		q.Push(SSEEvent{Type: fmt.Sprintf("ev-%d", i)})
	}
	q.Close()

	var got int
	for range q.C() {
		got++
	}
	if got != n {
		t.Fatalf("got %d events, want %d", got, n)
	}
}

func TestSSEQueueCloseIdempotent(t *testing.T) {
	q := NewSSEQueue()
	q.Close()
	q.Close()
	q.Close()
	for range q.C() {
	}
}

func TestSSEQueuePushAfterCloseReturnsFalse(t *testing.T) {
	q := NewSSEQueue()
	q.Close()
	for range q.C() {
	}
	if ok := q.Push(SSEEvent{Type: "late"}); ok {
		t.Fatal("Push() after Close() = true, want false")
	}
}

func TestSSEQueueConcurrentPushers(t *testing.T) {
	q := NewSSEQueue()

	const pushers = 10
	const eventsPerPusher = 500

	var wg sync.WaitGroup
	for p := 0; p < pushers; p++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < eventsPerPusher; i++ {
				q.Push(SSEEvent{Type: fmt.Sprintf("p%d-ev%d", id, i)})
			}
		}(p)
	}

	go func() {
		wg.Wait()
		q.Close()
	}()

	var count int
	for range q.C() {
		count++
	}
	if count != pushers*eventsPerPusher {
		t.Fatalf("got %d events, want %d", count, pushers*eventsPerPusher)
	}
}

func TestSSEQueueSignalsOverflowAndKeepsCriticalEvents(t *testing.T) {
	q := &SSEQueue{
		out:    make(chan SSEEvent),
		maxBuf: 3,
	}
	q.cond = sync.NewCond(&q.mu)
	go q.drain()

	q.Push(SSEEvent{Type: "delta", Data: SSEDeltaData{Content: "a"}})
	q.Push(SSEEvent{Type: "delta", Data: SSEDeltaData{Content: "b"}})
	q.Push(SSEEvent{Type: "tool_end", Data: SSEToolEndData{Tool: "read", Output: "ok"}})
	q.Push(SSEEvent{Type: "done", Data: SSEDoneData{}})
	q.Close()

	var events []SSEEvent
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-q.C():
			if !ok {
				if len(events) != 3 {
					t.Fatalf("events = %d, want 3", len(events))
				}
				if events[0].Type != "overflow" {
					t.Fatalf("first event type = %q, want overflow", events[0].Type)
				}
				overflow, ok := events[0].Data.(SSEOverflowData)
				if !ok {
					t.Fatalf("overflow payload type = %T, want SSEOverflowData", events[0].Data)
				}
				if overflow.Dropped != 2 {
					t.Fatalf("overflow dropped = %d, want 2", overflow.Dropped)
				}
				if overflow.Critical {
					t.Fatal("overflow should not mark critical drops when only deltas were removed")
				}
				if events[1].Type != "tool_end" || events[2].Type != "done" {
					t.Fatalf("kept events = %q, %q; want tool_end, done", events[1].Type, events[2].Type)
				}
				return
			}
			events = append(events, ev)
		case <-timeout:
			t.Fatal("timed out reading queue events")
		}
	}
}

func TestSSEQueueSignalsCriticalOverflowWhenNonLossyEventsDropped(t *testing.T) {
	q := &SSEQueue{
		out:    make(chan SSEEvent),
		maxBuf: 2,
	}
	q.cond = sync.NewCond(&q.mu)
	go q.drain()

	q.Push(SSEEvent{Type: "tool_start", Data: SSEToolStartData{Tool: "read"}})
	q.Push(SSEEvent{Type: "tool_end", Data: SSEToolEndData{Tool: "read", Output: "ok"}})
	q.Push(SSEEvent{Type: "done", Data: SSEDoneData{}})
	q.Close()

	var events []SSEEvent
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-q.C():
			if !ok {
				if len(events) != 3 {
					t.Fatalf("events = %d, want 3", len(events))
				}
				if events[0].Type != "tool_start" || events[1].Type != "tool_end" || events[2].Type != "done" {
					t.Fatalf("event types = %q, %q, %q; want tool_start, tool_end, done", events[0].Type, events[1].Type, events[2].Type)
				}
				return
			}
			events = append(events, ev)
		case <-timeout:
			t.Fatal("timed out reading queue events")
		}
	}
}

func TestSSEQueueRetainsQuestionEventsUnderOverflow(t *testing.T) {
	q := &SSEQueue{
		out:    make(chan SSEEvent),
		maxBuf: 3,
	}
	q.cond = sync.NewCond(&q.mu)
	go q.drain()

	q.Push(SSEEvent{Type: "delta", Data: SSEDeltaData{Content: "stream"}})
	q.Push(SSEEvent{Type: "user_question", Data: SSEUserQuestionData{QuestionID: "uq-1", Question: "Which approach?"}})
	q.Push(SSEEvent{Type: "question", Data: SSEQuestionData{QuestionID: "pq-1", Tool: "bash", Input: "{}"}})
	q.Push(SSEEvent{Type: "done", Data: SSEDoneData{}})
	q.Close()

	var events []SSEEvent
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-q.C():
			if !ok {
				if len(events) != 4 {
					t.Fatalf("events = %d, want 4", len(events))
				}
				if events[0].Type != "overflow" {
					t.Fatalf("first event type = %q, want overflow", events[0].Type)
				}
				overflow, ok := events[0].Data.(SSEOverflowData)
				if !ok {
					t.Fatalf("overflow payload type = %T, want SSEOverflowData", events[0].Data)
				}
				if overflow.Dropped != 1 {
					t.Fatalf("overflow dropped = %d, want 1", overflow.Dropped)
				}
				if overflow.Critical {
					t.Fatal("overflow should not be critical when only lossy events were removed")
				}
				if events[1].Type != "user_question" || events[2].Type != "question" || events[3].Type != "done" {
					t.Fatalf("retained events = %q, %q, %q; want user_question, question, done", events[1].Type, events[2].Type, events[3].Type)
				}
				return
			}
			events = append(events, ev)
		case <-timeout:
			t.Fatal("timed out reading queue events")
		}
	}
}
