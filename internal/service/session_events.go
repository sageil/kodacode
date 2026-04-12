package service

import (
	"sync"

	"github.com/sageil/kodacode/v1/internal/logging"
)

type sessionEventHub struct {
	mu   sync.Mutex
	subs map[string][]*SSEQueue
}

func newSessionEventHub() *sessionEventHub {
	return &sessionEventHub{
		subs: make(map[string][]*SSEQueue),
	}
}

func (h *sessionEventHub) Subscribe(sessionID string) (<-chan SSEEvent, func()) {
	q := NewSSEQueue()

	h.mu.Lock()
	h.subs[sessionID] = append(h.subs[sessionID], q)
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		queues := h.subs[sessionID]
		for i, qq := range queues {
			if qq == q {
				h.subs[sessionID] = append(queues[:i], queues[i+1:]...)
				break
			}
		}
		if len(h.subs[sessionID]) == 0 {
			delete(h.subs, sessionID)
		}
		q.Close()
	}
	return q.C(), cancel
}

func (h *sessionEventHub) Publish(sessionID string, ev SSEEvent) {
	h.mu.Lock()
	queues := make([]*SSEQueue, len(h.subs[sessionID]))
	copy(queues, h.subs[sessionID])
	h.mu.Unlock()

	if ev.Type == "reasoning_delta" || ev.Type == "reasoning_done" {
		logging.Debugf("[3-publish] %s event to %d subscribers", ev.Type, len(queues))
	}
	for _, q := range queues {
		q.Push(ev)
	}
}

func (h *sessionEventHub) CleanupSession(sessionID string) {
	h.mu.Lock()
	queues := h.subs[sessionID]
	delete(h.subs, sessionID)
	h.mu.Unlock()
	for _, q := range queues {
		q.Close()
	}
}
