package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrSessionRequired      = errors.New("session_id is required")
	ErrAfterSequenceInvalid = errors.New("after_sequence must be >= -1")
)

type MemoryStore struct {
	mu              sync.Mutex
	sessions        map[string][]Event
	watchers        map[string]map[*watcher]struct{}
	branchSummaries map[string]BranchSummaryArtifact
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions:        make(map[string][]Event),
		watchers:        make(map[string]map[*watcher]struct{}),
		branchSummaries: make(map[string]BranchSummaryArtifact),
	}
}

func (s *MemoryStore) Append(_ context.Context, draft Draft) (Event, error) {
	if err := draft.Validate(); err != nil {
		return Event{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	events := s.sessions[draft.SessionID]
	sequence := int64(len(events))
	event := Event{
		ID:        fmt.Sprintf("%s:%d", draft.SessionID, sequence),
		SessionID: draft.SessionID,
		TurnID:    draft.TurnID,
		Sequence:  sequence,
		Time:      time.Now().UTC(),
		Type:      draft.Type,
		Payload:   draft.Payload,
	}
	if err := event.Validate(); err != nil {
		return Event{}, err
	}

	s.sessions[draft.SessionID] = append(events, event)
	for w := range s.watchers[draft.SessionID] {
		w.push(event)
	}

	return event, nil
}

func (s *MemoryStore) Replay(_ context.Context, query Query) ([]Event, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return replayFrom(s.sessions[query.SessionID], query), nil
}

func (s *MemoryStore) Latest(_ context.Context, query LatestQuery) (Event, bool, error) {
	if err := query.Validate(); err != nil {
		return Event{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	typeSet := make(map[Type]struct{}, len(query.Types))
	for _, typ := range query.Types {
		typeSet[typ] = struct{}{}
	}
	events := s.sessions[query.SessionID]
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if _, ok := typeSet[event.Type]; ok {
			return event, true, nil
		}
	}
	return Event{}, false, nil
}

func (s *MemoryStore) Watch(ctx context.Context, query Query) (<-chan Event, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	w := newWatcher(query)

	s.mu.Lock()
	w.queue = replayFrom(s.sessions[query.SessionID], query)
	if s.watchers[query.SessionID] == nil {
		s.watchers[query.SessionID] = make(map[*watcher]struct{})
	}
	s.watchers[query.SessionID][w] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.watchers[query.SessionID], w)
			if len(s.watchers[query.SessionID]) == 0 {
				delete(s.watchers, query.SessionID)
			}
			s.mu.Unlock()
		}()
		w.run(ctx)
	}()

	return w.out, nil
}

func (s *MemoryStore) ListWorkspaceSessions(_ context.Context, workspaceRoot string) ([]SessionIndexEntry, error) {
	if workspaceRoot == "" {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var sessions []SessionIndexEntry
	for sessionID, events := range s.sessions {
		if len(events) == 0 {
			continue
		}
		configured, updatedAt, ok := memorySessionIndex(events)
		if !ok || configured.WorkspaceRoot != workspaceRoot {
			continue
		}
		sessions = append(sessions, SessionIndexEntry{
			SessionID:     sessionID,
			WorkspaceRoot: configured.WorkspaceRoot,
			UpdatedAt:     updatedAt,
		})
	}
	sortSessionIndexEntries(sessions)
	return sessions, nil
}

func (s *MemoryStore) ListSessions(_ context.Context) ([]SessionIndexEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var sessions []SessionIndexEntry
	for sessionID, events := range s.sessions {
		if len(events) == 0 {
			continue
		}
		configured, updatedAt, ok := memorySessionIndex(events)
		if !ok {
			continue
		}
		sessions = append(sessions, SessionIndexEntry{
			SessionID:     sessionID,
			WorkspaceRoot: configured.WorkspaceRoot,
			UpdatedAt:     updatedAt,
		})
	}
	sortSessionIndexEntries(sessions)
	return sessions, nil
}

func (s *MemoryStore) SaveBranchSummary(_ context.Context, artifact BranchSummaryArtifact) error {
	sessionID := strings.TrimSpace(artifact.SessionID)
	artifact.Summary = strings.TrimSpace(artifact.Summary)
	if sessionID == "" || artifact.Summary == "" {
		return nil
	}
	now := time.Now().UTC()
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = now
	}
	if artifact.UpdatedAt.IsZero() {
		artifact.UpdatedAt = now
	}
	artifact.SessionID = sessionID
	if artifact.SourceSequence < 0 {
		artifact.SourceSequence = 0
	}
	if artifact.PromptTokens < 0 {
		artifact.PromptTokens = 0
	}
	if artifact.CompletionTokens < 0 {
		artifact.CompletionTokens = 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.branchSummaries[sessionID] = artifact
	return nil
}

func (s *MemoryStore) LoadBranchSummary(_ context.Context, sessionID string) (BranchSummaryArtifact, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return BranchSummaryArtifact{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact, ok := s.branchSummaries[sessionID]
	return artifact, ok, nil
}

func memorySessionIndex(events []Event) (SessionConfiguredPayload, time.Time, bool) {
	if len(events) == 0 {
		return SessionConfiguredPayload{}, time.Time{}, false
	}
	var configured SessionConfiguredPayload
	ok := false
	var updatedAt time.Time
	for _, event := range events {
		if payload, match := event.Payload.(SessionConfiguredPayload); match {
			configured = payload
			ok = true
		}
		if event.Type != TypeSessionStateSnapshot {
			updatedAt = event.Time
		}
	}
	if !ok {
		return SessionConfiguredPayload{}, time.Time{}, false
	}
	if updatedAt.IsZero() {
		updatedAt = events[len(events)-1].Time
	}
	return configured, updatedAt, true
}

func replayFrom(events []Event, query Query) []Event {
	if len(events) == 0 {
		return nil
	}
	allow := queryTypeMatcher(query)
	out := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Sequence <= query.AfterSequence {
			continue
		}
		if allow != nil && !allow(event.Type) {
			continue
		}
		out = append(out, event)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type watcher struct {
	out    chan Event
	notify chan struct{}

	mu     sync.Mutex
	queue  []Event
	closed bool
	allow  func(Type) bool
}

func newWatcher(query Query) *watcher {
	return &watcher{
		out:    make(chan Event),
		notify: make(chan struct{}, 1),
		allow:  queryTypeMatcher(query),
	}
}

func (w *watcher) push(event Event) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	if w.allow != nil && !w.allow(event.Type) {
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

func (w *watcher) run(ctx context.Context) {
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

func (w *watcher) close() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()

	select {
	case w.notify <- struct{}{}:
	default:
	}
}

func (w *watcher) pop() (Event, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.queue) == 0 {
		return Event{}, false
	}
	event := w.queue[0]
	w.queue = w.queue[1:]
	return event, true
}

func queryTypeMatcher(query Query) func(Type) bool {
	if len(query.ExcludeTypes) == 0 {
		return nil
	}
	excluded := make(map[Type]struct{}, len(query.ExcludeTypes))
	for _, typ := range query.ExcludeTypes {
		excluded[typ] = struct{}{}
	}
	return func(typ Type) bool {
		_, blocked := excluded[typ]
		return !blocked
	}
}
