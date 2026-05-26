package app

import (
	"context"
	"sync"

	"github.com/sageil/kodacode/internal/events"
)

type sessionWatcher struct {
	out    chan events.Event
	notify chan struct{}

	mu     sync.Mutex
	queue  []events.Event
	closed bool
}

func newSessionWatcher(initial []events.Event) *sessionWatcher {
	return &sessionWatcher{
		out:    make(chan events.Event),
		notify: make(chan struct{}, 1),
		queue:  append([]events.Event(nil), initial...),
	}
}

func (w *sessionWatcher) push(event events.Event) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.queue = append(w.queue, event)
	w.mu.Unlock()

	select {
	case w.notify <- struct{}{}:
	default:
	}
}

func (w *sessionWatcher) run(ctx context.Context) {
	defer func() {
		w.mu.Lock()
		w.closed = true
		w.mu.Unlock()
		close(w.out)
	}()

	for {
		event, ok := w.pop()
		if ok {
			select {
			case <-ctx.Done():
				return
			case w.out <- event:
			}
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-w.notify:
		}
	}
}

func (w *sessionWatcher) pop() (events.Event, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.queue) == 0 {
		return events.Event{}, false
	}
	event := w.queue[0]
	w.queue = w.queue[1:]
	return event, true
}
