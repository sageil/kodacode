package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func testSessionHistoryArtifactStream() provider.Stream {
	return provider.NewSliceStream([]provider.Event{
		{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(
			testSimpleHistoryContinuationArtifact(
				"continue the task",
				[]string{"history compacted"},
				[]string{"review the remaining history"},
				"internal/app/runtime_compaction.go",
			),
		)},
	})
}

func TestCompactSessionHistoryCreatesStandaloneCompactionTurn(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first done"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second done"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(
					testSimpleHistoryContinuationArtifact(
						"continue the task",
						[]string{"turn 1 compacted"},
						[]string{"review the remaining history"},
						"internal/app/runtime_compaction.go",
					),
				)},
			}),
		},
		counts:       []int{2000, 2000, 2000, 2000, 2000, 2000},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact", "exact", "exact"},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turnID := range []string{"turn-1", "turn-2"} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turnID,
			UserText:  turnID,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turnID, err)
		}
	}
	baselineCountRequests := len(client.countRequests)

	result, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact",
	})
	if err != nil {
		t.Fatalf("CompactSessionHistory() error = %v", err)
	}
	if result.Continuation == nil {
		t.Fatal("CompactSessionHistory() returned nil continuation payload")
	}
	if got := result.Continuation.UpdateReason; got != sessionHistoryManualCompactionReason {
		t.Fatalf("compaction reason = %q, want %q", got, sessionHistoryManualCompactionReason)
	}

	state, err := runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	turn := state.Turns["turn-compact"]
	if turn == nil {
		t.Fatal("compaction turn missing from session state")
	}
	if turn.Status != events.TurnStatusCompleted {
		t.Fatalf("compaction turn status = %q, want completed", turn.Status)
	}
	if turn.Continuation == nil {
		t.Fatal("compaction turn missing compaction state")
	}
	if got := turn.Continuation.Attribution.MeasurementSource; got != "estimated" {
		t.Fatalf("compaction measurement source = %q, want estimated", got)
	}
	if got := turn.UserText; got != "" {
		t.Fatalf("compaction turn user text = %q, want empty", got)
	}
	if got := len(client.countRequests); got != baselineCountRequests {
		t.Fatalf("count requests = %d, want unchanged from baseline %d for manual compaction estimates", got, baselineCountRequests)
	}

	history, err := runtime.Runner.loadSessionHistoryStateForModel(context.Background(), sessionID, "turn-next", runtime.Config.ModelRoute)
	if err != nil {
		t.Fatalf("loadSessionHistoryStateForModel() error = %v", err)
	}
	if len(history.CompletedOrder) != 2 {
		t.Fatalf("completed order = %#v, want 2 conversational turns", history.CompletedOrder)
	}
	for _, turnID := range history.CompletedOrder {
		if turnID == "turn-compact" {
			t.Fatalf("completed order should exclude compaction-only turn: %#v", history.CompletedOrder)
		}
	}
}

func TestCompactSessionHistoryAllowsCompactingSingleCompletedTurn(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
			testSessionHistoryArtifactStream(),
		},
	})

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "first",
		AgentID:   "builder",
	}); err != nil {
		t.Fatalf("StartSessionTurn() error = %v", err)
	}

	result, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact",
	})
	if err != nil {
		t.Fatalf("CompactSessionHistory() error = %v", err)
	}
	if result.Continuation == nil {
		t.Fatal("CompactSessionHistory() returned nil continuation payload")
	}
	if got := result.Continuation.ConsolidatedTurnCount; got != 1 {
		t.Fatalf("compacted turn count = %d, want 1", got)
	}

	state, err := runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	turn := state.Turns["turn-compact"]
	if turn == nil {
		t.Fatal("compaction turn missing from session state")
	}
	if turn.Status != events.TurnStatusCompleted {
		t.Fatalf("compaction turn status = %q, want completed", turn.Status)
	}

	history, err := runtime.Runner.loadSessionHistoryStateForModel(context.Background(), sessionID, "turn-next", runtime.Config.ModelRoute)
	if err != nil {
		t.Fatalf("loadSessionHistoryStateForModel() error = %v", err)
	}
	if len(history.CompletedOrder) != 1 || history.CompletedOrder[0] != "turn-1" {
		t.Fatalf("completed order = %#v, want original conversational turn retained", history.CompletedOrder)
	}
	if history.ExistingContinuation == nil || history.ExistingContinuation.ConsolidatedTurnCount != 1 {
		t.Fatalf("existing compaction = %#v, want one compacted turn", history.ExistingContinuation)
	}
}

func TestCompactSessionHistoryReusesActiveTurnWhenRequestedMidTurn(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first done"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second done"},
			}),
			testSessionHistoryArtifactStream(),
		},
		counts:       []int{2000, 2000, 2000, 2000, 2000, 2000},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact", "exact", "exact"},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turnID := range []string{"turn-1", "turn-2"} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turnID,
			UserText:  turnID,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turnID, err)
		}
	}
	if _, err := runtime.Sessions.append(context.Background(), events.Draft{
		SessionID: sessionID,
		TurnID:    "turn-3",
		Type:      events.TypeUserMessage,
		Payload:   events.UserMessagePayload{Content: "pending answer"},
	}); err != nil {
		t.Fatalf("append running turn user message error = %v", err)
	}
	baselineCountRequests := len(client.countRequests)

	result, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-3",
	})
	if err != nil {
		t.Fatalf("CompactSessionHistory() error = %v", err)
	}
	if result.TurnID != "turn-3" {
		t.Fatalf("turn id = %q, want turn-3", result.TurnID)
	}

	state, err := runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	turn := state.Turns["turn-3"]
	if turn == nil {
		t.Fatal("turn-3 missing from session state")
	}
	if turn.Status != events.TurnStatusRunning {
		t.Fatalf("turn-3 status = %q, want running", turn.Status)
	}
	if turn.Continuation == nil {
		t.Fatal("turn-3 compaction missing")
	}
	if got := turn.Continuation.Attribution.MeasurementSource; got != "estimated" {
		t.Fatalf("turn-3 compaction measurement source = %q, want estimated", got)
	}
	if got := len(client.countRequests); got != baselineCountRequests {
		t.Fatalf("count requests = %d, want unchanged from baseline %d for manual compaction estimates", got, baselineCountRequests)
	}
}

func TestCompactSessionHistoryInlineKeepsPendingPermissionResumable(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first done"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["` + outsidePath + `"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			testSessionHistoryArtifactStream(),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "resumed after permission"},
			}),
		},
		counts:       []int{2000, 2000, 2000, 2000, 2000, 2000},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact", "exact", "exact"},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "finish first",
		AgentID:   "builder",
	}); err != nil {
		t.Fatalf("StartSessionTurn(turn-1) error = %v", err)
	}
	pending, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-2",
		UserText:  "inspect the outside file",
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("StartSessionTurn(turn-2) error = %v", err)
	}
	if pending.Status != TurnRunStatusPending || pending.PendingPermission == nil {
		t.Fatalf("pending result = %#v", pending)
	}

	if _, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-2",
	}); err != nil {
		t.Fatalf("CompactSessionHistory() error = %v", err)
	}

	resumed, err := runtime.ResolveSessionTurn(context.Background(), ResolveSessionTurnInput{
		SessionID:           sessionID,
		TurnID:              "turn-2",
		PermissionRequestID: pending.PendingRequestID,
		Decision:            events.PermissionDecisionApproved,
		Scope:               events.PermissionScopeOnce,
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurn() error = %v", err)
	}
	if resumed.Status != TurnRunStatusCompleted || resumed.AssistantText != "resumed after permission" {
		t.Fatalf("resumed result = %#v", resumed)
	}
}

func TestCompactSessionHistoryKeepsStandaloneTurnCompletedWhenCheckpointAppendFails(t *testing.T) {
	store := &executionServiceFailingStore{
		MemoryStore: events.NewMemoryStore(),
		failTypes:   map[events.Type]error{},
	}
	runtime := newRuntimeWithClientAndStore(t, &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first done"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second done"},
			}),
			testSessionHistoryArtifactStream(),
		},
		counts:       []int{2000, 2000, 2000, 2000, 2000, 2000},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact", "exact", "exact"},
	}, store)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turnID := range []string{"turn-1", "turn-2"} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turnID,
			UserText:  turnID,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turnID, err)
		}
	}
	store.failTypes[events.TypeSessionHistoryCheckpoint] = errors.New("checkpoint write failed")

	_, err = runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact",
	})
	if err == nil || !strings.Contains(err.Error(), "checkpoint write failed") {
		t.Fatalf("CompactSessionHistory() error = %v, want checkpoint failure", err)
	}

	state, err := runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	turn := state.Turns["turn-compact"]
	if turn == nil {
		t.Fatal("compaction turn missing from session state")
	}
	if turn.Status != events.TurnStatusCompleted {
		t.Fatalf("compaction turn status = %q, want completed after checkpoint failure", turn.Status)
	}
}

func TestCompactSessionHistoryMarksStandaloneTurnFailedWhenCompactionAppendFails(t *testing.T) {
	store := &executionServiceFailingStore{
		MemoryStore: events.NewMemoryStore(),
		failTypes: map[events.Type]error{
			events.TypeSessionHistoryContinuationUpdated: errors.New("context compacted write failed"),
		},
	}
	runtime := newRuntimeWithClientAndStore(t, &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first done"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second done"},
			}),
			testSessionHistoryArtifactStream(),
		},
		counts:       []int{2000, 2000, 2000, 2000, 2000, 2000},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact", "exact", "exact"},
	}, store)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turnID := range []string{"turn-1", "turn-2"} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turnID,
			UserText:  turnID,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turnID, err)
		}
	}

	_, err = runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact",
	})
	if err == nil || !strings.Contains(err.Error(), "context compacted write failed") {
		t.Fatalf("CompactSessionHistory() error = %v, want context compacted append failure", err)
	}

	state, err := runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	turn := state.Turns["turn-compact"]
	if turn == nil {
		t.Fatal("compaction turn missing from session state")
	}
	if turn.Status != events.TurnStatusFailed {
		t.Fatalf("compaction turn status = %q, want failed after compaction append failure", turn.Status)
	}
}

func TestCompactSessionHistoryDoesNotRetryTurnDoneAfterExplicitFailure(t *testing.T) {
	store := &failFirstAppendStore{
		MemoryStore: events.NewMemoryStore(),
		failures: map[events.Type]error{
			events.TypeTurnDone: errors.New("turn done failed"),
		},
		failTurnID: "turn-compact",
	}
	runtime := newRuntimeWithClientAndStore(t, &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first done"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second done"},
			}),
			testSessionHistoryArtifactStream(),
		},
		counts:       []int{2000, 2000, 2000, 2000, 2000, 2000},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact", "exact", "exact"},
	}, store)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turnID := range []string{"turn-1", "turn-2"} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turnID,
			UserText:  turnID,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turnID, err)
		}
	}

	_, err = runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact",
	})
	if err == nil || !strings.Contains(err.Error(), "turn done failed") {
		t.Fatalf("CompactSessionHistory() error = %v, want turn done failure", err)
	}
	if got := store.targetTurnDoneAttempts; got != 1 {
		t.Fatalf("compaction turn_done append attempts = %d, want 1", got)
	}

	state, err := runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	turn := state.Turns["turn-compact"]
	if turn == nil {
		t.Fatal("compaction turn missing from session state")
	}
	if turn.Status != events.TurnStatusFailed {
		t.Fatalf("compaction turn status = %q, want failed after turn_done failure", turn.Status)
	}

	if _, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact-2",
	}); errors.Is(err, ErrSessionCompactionTurnRunning) {
		t.Fatalf("CompactSessionHistory() error = %v, want no dangling running compaction turn", err)
	}
}

type failFirstAppendStore struct {
	*events.MemoryStore
	failures     map[events.Type]error
	appendCounts map[events.Type]int
	failTurnID   string

	targetTurnDoneAttempts int
}

func (s *failFirstAppendStore) Append(ctx context.Context, draft events.Draft) (events.Event, error) {
	if s.appendCounts == nil {
		s.appendCounts = make(map[events.Type]int)
	}
	s.appendCounts[draft.Type]++
	if draft.Type == events.TypeTurnDone && draft.TurnID == s.failTurnID {
		s.targetTurnDoneAttempts++
	}
	if draft.TurnID == s.failTurnID {
		if err := s.failures[draft.Type]; err != nil {
			delete(s.failures, draft.Type)
			return events.Event{}, err
		}
	}
	if draft.TurnID != s.failTurnID {
		return s.MemoryStore.Append(ctx, draft)
	}
	if err := s.failures[draft.Type]; err != nil {
		delete(s.failures, draft.Type)
		return events.Event{}, err
	}
	return s.MemoryStore.Append(ctx, draft)
}
