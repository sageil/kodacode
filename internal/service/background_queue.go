package service

import "sync"

type sessionBackgroundBroker struct {
	mu       sync.Mutex
	draining map[string]bool
}

func newSessionBackgroundBroker() *sessionBackgroundBroker {
	return &sessionBackgroundBroker{
		draining: make(map[string]bool),
	}
}

func (b *sessionBackgroundBroker) StartDrain(sessionID string) bool {
	if b == nil || sessionID == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.draining[sessionID] {
		return false
	}
	b.draining[sessionID] = true
	return true
}

func (b *sessionBackgroundBroker) FinishDrain(sessionID string) {
	if b == nil || sessionID == "" {
		return
	}
	b.mu.Lock()
	delete(b.draining, sessionID)
	b.mu.Unlock()
}

func (b *sessionBackgroundBroker) CleanupSession(sessionID string) {
	b.FinishDrain(sessionID)
}
