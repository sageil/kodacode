package service

import (
	"log"
	"sync"
)

const defaultMaxBuf = 10000

// SSEQueue is a bounded event queue that decouples publishers from consumers.
// Push is non-blocking. Under pressure, lossy stream events are dropped first
// and an overflow marker is emitted so clients can warn or resync. Workflow-
// critical events are retained even if that causes the queue to temporarily
// exceed maxBuf; correctness takes priority over strict bounding.
type SSEQueue struct {
	mu               sync.Mutex
	cond             *sync.Cond
	buf              []SSEEvent
	closed           bool
	out              chan SSEEvent
	maxBuf           int
	overflowDropped  int
	overflowCritical bool
}

// NewSSEQueue creates a queue and starts its drainer goroutine.
// The returned queue's C channel delivers events to consumers.
// Call Close to stop the drainer and close the output channel.
func NewSSEQueue() *SSEQueue {
	q := &SSEQueue{
		out:    make(chan SSEEvent, 64),
		maxBuf: defaultMaxBuf,
	}
	q.cond = sync.NewCond(&q.mu)
	go q.drain()
	return q
}

// C returns the read-only channel that consumers select on or range over.
func (q *SSEQueue) C() <-chan SSEEvent {
	return q.out
}

// Push appends an event to the queue. It never blocks the caller.
// Returns false if the queue is closed and the event was dropped.
func (q *SSEQueue) Push(ev SSEEvent) bool {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return false
	}
	q.buf = append(q.buf, ev)
	q.trimLocked()
	q.mu.Unlock()
	q.cond.Signal()
	return true
}

// Close shuts down the drainer goroutine and closes the output channel.
// Safe to call multiple times.
func (q *SSEQueue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	q.mu.Unlock()
	q.cond.Signal()
}

// drain runs in its own goroutine, moving events from buf to out.
func (q *SSEQueue) drain() {
	defer close(q.out)
	for {
		q.mu.Lock()
		for len(q.buf) == 0 && !q.closed {
			q.cond.Wait()
		}
		if q.closed && len(q.buf) == 0 {
			q.mu.Unlock()
			return
		}
		batch := q.buf
		q.buf = nil
		overflowDropped := q.overflowDropped
		overflowCritical := q.overflowCritical
		q.overflowDropped = 0
		q.overflowCritical = false
		q.mu.Unlock()

		if overflowDropped > 0 {
			batch = append([]SSEEvent{{
				Type: "overflow",
				Data: SSEOverflowData{Dropped: overflowDropped, Critical: overflowCritical},
			}}, batch...)
		}
		for _, ev := range batch {
			q.out <- ev
		}
	}
}

func (q *SSEQueue) trimLocked() {
	if len(q.buf) <= q.maxBuf {
		return
	}
	target := q.maxBuf - 1
	if target < 1 {
		target = 1
	}
	needDrop := len(q.buf) - target
	kept := make([]SSEEvent, 0, len(q.buf)-needDrop)
	dropped := 0
	for _, ev := range q.buf {
		if needDrop > 0 && isLossySSEType(ev.Type) {
			dropped++
			needDrop--
			continue
		}
		kept = append(kept, ev)
	}
	q.buf = kept
	q.overflowDropped += dropped
	q.overflowCritical = false
	if dropped > 0 {
		log.Printf("ssequeue: dropped %d lossy events (buffer=%d, retained=%d)", dropped, q.maxBuf, len(q.buf))
	}
}

func isLossySSEType(eventType string) bool {
	switch eventType {
	case "delta",
		"reasoning_delta",
		"reasoning_done",
		"tool_input_delta",
		"tool_output",
		"subagent_activity",
		"usage",
		"retry",
		"warning",
		"overflow",
		"step_trace",
		"loop_detected",
		"title_updated",
		"compaction_start",
		"compaction":
		return true
	default:
		return false
	}
}
