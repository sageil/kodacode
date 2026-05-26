package app

import (
	"context"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

const crossSessionBlockedWindow = 150 * time.Millisecond

// These tests guard the Workstream 2 contract: unrelated sessions must not
// serialize on cold store I/O for snapshot, watch, or append paths.
func TestSessionServiceCrossSessionContentionColdSnapshotDoesNotBlockUnrelatedAppend(t *testing.T) {
	store := newGatedSessionReplayStore()
	seedSessionContentionTestSession(t, store, "session-a", t.TempDir(),
		events.Draft{
			SessionID: "session-a",
			TurnID:    "turn-1",
			Type:      events.TypeAssistantCommit,
			Payload:   events.AssistantCommitPayload{Content: "snapshot baseline"},
		},
	)
	seedSessionContentionTestSession(t, store, "session-b", t.TempDir())

	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	gate := store.installGate(sessionStoreOpReplay, "session-a")
	snapshotDone := make(chan error, 1)
	go func() {
		_, err := service.Snapshot(context.Background(), "session-a")
		snapshotDone <- err
	}()
	gate.waitEntered(t)

	appendDone := make(chan error, 1)
	go func() {
		_, err := service.append(context.Background(), events.Draft{
			SessionID: "session-b",
			TurnID:    "turn-1",
			Type:      events.TypeAssistantCommit,
			Payload:   events.AssistantCommitPayload{Content: "blocked by snapshot"},
		})
		appendDone <- err
	}()

	waitOperationResultWithin(t, appendDone, "append(session-b)", crossSessionBlockedWindow)
	gate.release()
	waitOperationResult(t, snapshotDone, "Snapshot(session-a)")
}

func TestSessionServiceCrossSessionContentionWatchReplayDoesNotBlockUnrelatedAppend(t *testing.T) {
	store := newGatedSessionReplayStore()
	seedSessionContentionTestSession(t, store, "session-a", t.TempDir(),
		events.Draft{
			SessionID: "session-a",
			TurnID:    "turn-1",
			Type:      events.TypeAssistantCommit,
			Payload:   events.AssistantCommitPayload{Content: "watch baseline"},
		},
	)
	seedSessionContentionTestSession(t, store, "session-b", t.TempDir())

	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	gate := store.installGate(sessionStoreOpReplay, "session-a")
	watchDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := service.Watch(ctx, "session-a", -1)
		if err != nil {
			cancel()
			watchDone <- err
			return
		}
		cancel()
		for event := range stream {
			_ = event
		}
		watchDone <- nil
	}()
	gate.waitEntered(t)

	appendDone := make(chan error, 1)
	go func() {
		_, err := service.append(context.Background(), events.Draft{
			SessionID: "session-b",
			TurnID:    "turn-1",
			Type:      events.TypeAssistantCommit,
			Payload:   events.AssistantCommitPayload{Content: "blocked by watch"},
		})
		appendDone <- err
	}()

	waitOperationResultWithin(t, appendDone, "append(session-b)", crossSessionBlockedWindow)
	gate.release()
	waitOperationResult(t, watchDone, "Watch(session-a)")
}

func TestSessionServiceCrossSessionContentionAppendDoesNotBlockUnrelatedAppend(t *testing.T) {
	store := newGatedSessionReplayStore()
	seedSessionContentionTestSession(t, store, "session-a", t.TempDir())
	seedSessionContentionTestSession(t, store, "session-b", t.TempDir())

	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	gate := store.installGate(sessionStoreOpAppend, "session-a")
	firstAppendDone := make(chan error, 1)
	go func() {
		_, err := service.append(context.Background(), events.Draft{
			SessionID: "session-a",
			TurnID:    "turn-1",
			Type:      events.TypeAssistantCommit,
			Payload:   events.AssistantCommitPayload{Content: "append baseline"},
		})
		firstAppendDone <- err
	}()
	gate.waitEntered(t)

	secondAppendDone := make(chan error, 1)
	go func() {
		_, err := service.append(context.Background(), events.Draft{
			SessionID: "session-b",
			TurnID:    "turn-1",
			Type:      events.TypeAssistantCommit,
			Payload:   events.AssistantCommitPayload{Content: "blocked by append"},
		})
		secondAppendDone <- err
	}()

	waitOperationResultWithin(t, secondAppendDone, "append(session-b)", crossSessionBlockedWindow)
	gate.release()
	waitOperationResult(t, firstAppendDone, "append(session-a)")
}

type gatedSessionReplayStore struct {
	*events.MemoryStore

	gates map[sessionStoreGateKey]*sessionStoreGate
}

type sessionStoreGateKey struct {
	op        string
	sessionID string
}

type sessionStoreGate struct {
	entered chan struct{}
	unblock chan struct{}
}

const (
	sessionStoreOpAppend = "append"
	sessionStoreOpLatest = "latest"
	sessionStoreOpReplay = "replay"
)

func newGatedSessionReplayStore() *gatedSessionReplayStore {
	return &gatedSessionReplayStore{
		MemoryStore: events.NewMemoryStore(),
		gates:       make(map[sessionStoreGateKey]*sessionStoreGate),
	}
}

func (s *gatedSessionReplayStore) Append(ctx context.Context, draft events.Draft) (events.Event, error) {
	s.waitIfGated(ctx, sessionStoreOpAppend, draft.SessionID)
	return s.MemoryStore.Append(ctx, draft)
}

func (s *gatedSessionReplayStore) Replay(ctx context.Context, query events.Query) ([]events.Event, error) {
	s.waitIfGated(ctx, sessionStoreOpReplay, query.SessionID)
	return s.MemoryStore.Replay(ctx, query)
}

func (s *gatedSessionReplayStore) Latest(ctx context.Context, query events.LatestQuery) (events.Event, bool, error) {
	s.waitIfGated(ctx, sessionStoreOpLatest, query.SessionID)
	return s.MemoryStore.Latest(ctx, query)
}

func (s *gatedSessionReplayStore) installGate(op, sessionID string) *sessionStoreGate {
	gate := &sessionStoreGate{
		entered: make(chan struct{}),
		unblock: make(chan struct{}),
	}
	s.gates[sessionStoreGateKey{op: op, sessionID: sessionID}] = gate
	return gate
}

func (s *gatedSessionReplayStore) waitIfGated(ctx context.Context, op, sessionID string) {
	gate := s.gates[sessionStoreGateKey{op: op, sessionID: sessionID}]
	if gate == nil {
		return
	}
	select {
	case <-gate.entered:
	default:
		close(gate.entered)
	}
	select {
	case <-gate.unblock:
	case <-ctx.Done():
	}
}

func (g *sessionStoreGate) waitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gated store operation to start")
	}
}

func (g *sessionStoreGate) release() {
	close(g.unblock)
}

func seedSessionContentionTestSession(t *testing.T, store *gatedSessionReplayStore, sessionID, workspaceRoot string, extraDrafts ...events.Draft) {
	t.Helper()

	drafts := append([]events.Draft{{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionConfigured,
		Payload: events.SessionConfiguredPayload{
			WorkspaceRoot: workspaceRoot,
		},
	}}, extraDrafts...)

	for _, draft := range drafts {
		if _, err := store.MemoryStore.Append(context.Background(), draft); err != nil {
			t.Fatalf("seed session draft %s/%s/%s error = %v", draft.SessionID, draft.TurnID, draft.Type, err)
		}
	}
}

func waitOperationResultWithin(t *testing.T, done <-chan error, name string, within time.Duration) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
	case <-time.After(within):
		t.Fatalf("timed out waiting for %s within %s", name, within)
	}
}

func waitOperationResult(t *testing.T, done <-chan error, name string) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}
