package app

import (
	"context"
	"sync"

	"github.com/sageil/kodacode/internal/events"
)

type sessionRuntime struct {
	mu sync.Mutex

	watchers         map[*sessionWatcher]struct{}
	lastDurable      int64
	snapshotSequence int64
	projector        *events.Projector
	budget           budgetSessionSummary
	budgetWarm       bool
}

func newSessionRuntime() *sessionRuntime {
	return &sessionRuntime{
		watchers:         make(map[*sessionWatcher]struct{}),
		lastDurable:      -1,
		snapshotSequence: -1,
	}
}

func (r *sessionRuntime) pushLocked(event events.Event) int {
	count := len(r.watchers)
	for watcher := range r.watchers {
		watcher.push(event)
	}
	return count
}

func (s *SessionService) runtimeForSession(sessionID string) *sessionRuntime {
	s.registryMu.Lock()
	defer s.registryMu.Unlock()

	runtime := s.sessions[sessionID]
	if runtime != nil {
		return runtime
	}
	runtime = newSessionRuntime()
	s.sessions[sessionID] = runtime
	return runtime
}

func (s *SessionService) ensureProjectorLocked(ctx context.Context, sessionID string, runtime *sessionRuntime) error {
	if runtime == nil || runtime.projector != nil {
		return nil
	}
	projector, lastDurable, snapshotSequence, err := s.loadProjector(ctx, sessionID)
	if err != nil {
		return err
	}
	runtime.projector = projector
	runtime.lastDurable = lastDurable
	runtime.snapshotSequence = snapshotSequence
	return nil
}

func (s *SessionService) ensureBudgetSummaryLocked(runtime *sessionRuntime) {
	if runtime == nil || runtime.budgetWarm {
		return
	}
	if runtime.projector == nil {
		runtime.budget = budgetSessionSummary{}
		runtime.budgetWarm = true
		return
	}
	runtime.budget = budgetSessionSummaryFromState(runtime.projector.CurrentState())
	runtime.budgetWarm = true
}
