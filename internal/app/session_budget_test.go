package app

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

type failingBudgetStore struct {
	*events.MemoryStore
	extraSessions []events.SessionIndexEntry
	failReplayFor string
}

func (s *failingBudgetStore) ListSessions(ctx context.Context) ([]events.SessionIndexEntry, error) {
	entries, err := s.MemoryStore.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	return append(entries, s.extraSessions...), nil
}

func (s *failingBudgetStore) Replay(ctx context.Context, query events.Query) ([]events.Event, error) {
	if strings.TrimSpace(query.SessionID) == s.failReplayFor {
		return nil, errors.New("forced replay failure")
	}
	return s.MemoryStore.Replay(ctx, query)
}

func TestSessionServiceBudgetStatusTracksSessionAndCrossSessionBudgets(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	ctx := context.Background()
	workspaceRoot := t.TempDir()
	for _, sessionID := range []string{"session-1", "session-2"} {
		if _, err := sessions.CreateSession(ctx, CreateSessionInput{
			SessionID:     sessionID,
			WorkspaceRoot: workspaceRoot,
		}); err != nil {
			t.Fatalf("CreateSession(%q) error = %v", sessionID, err)
		}
	}
	appendUsage := func(sessionID, turnID, model string, requestTokens, completionTokens int, inputCost, outputCost float64) {
		t.Helper()
		if _, err := sessions.append(ctx, events.Draft{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeTurnProviderUsageRecorded,
			Payload: events.TurnProviderUsageRecordedPayload{
				Model:                     model,
				Step:                      1,
				Attempt:                   1,
				EstimatedRequestTokens:    requestTokens,
				EstimatedCompletionTokens: completionTokens,
				EstimatedInputCost:        inputCost,
				EstimatedOutputCost:       outputCost,
			},
		}); err != nil {
			t.Fatalf("append usage for %s/%s error = %v", sessionID, turnID, err)
		}
	}

	appendUsage("session-1", "turn-1", "openai/gpt-5", 1000, 250, 0.4, 0.2)
	appendUsage("session-1", "turn-2", "local/llama", 500, 100, 0, 0)
	appendUsage("session-2", "turn-1", "openai/gpt-5-mini", 1500, 300, 0.6, 0.3)

	status, err := sessions.BudgetStatus(ctx, "session-1", SessionConfig{
		Budget:          1.0,
		BudgetWarn:      0.5,
		TotalBudget:     1.2,
		TotalBudgetWarn: 0.8,
	})
	if err != nil {
		t.Fatalf("BudgetStatus() error = %v", err)
	}

	if math.Abs(status.SessionCost-0.6) > 1e-9 {
		t.Fatalf("SessionCost = %f, want 0.6", status.SessionCost)
	}
	if !status.SessionWarn || status.SessionExceeded {
		t.Fatalf("session budget flags = warn:%v exceeded:%v", status.SessionWarn, status.SessionExceeded)
	}
	if status.SessionMissingPricingTurns != 1 {
		t.Fatalf("SessionMissingPricingTurns = %d, want 1", status.SessionMissingPricingTurns)
	}
	if math.Abs(status.TotalCost-1.5) > 1e-9 {
		t.Fatalf("TotalCost = %f, want 1.5", status.TotalCost)
	}
	if !status.TotalWarn || !status.TotalExceeded {
		t.Fatalf("total budget flags = warn:%v exceeded:%v", status.TotalWarn, status.TotalExceeded)
	}
	if status.TotalMissingPricingTurns != 1 {
		t.Fatalf("TotalMissingPricingTurns = %d, want 1", status.TotalMissingPricingTurns)
	}
	if got := status.WarningMessage(); !strings.Contains(got, "Cross-session budget warning") {
		t.Fatalf("WarningMessage() = %q", got)
	}
	if err := status.ExceededError(); err == nil || !strings.Contains(err.Error(), "Cross-session budget reached") {
		t.Fatalf("ExceededError() = %v", err)
	}
}

func TestSessionServiceBudgetStatusUsesReportedCacheDiscountedCost(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	ctx := context.Background()
	workspaceRoot := t.TempDir()
	if _, err := sessions.CreateSession(ctx, CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: workspaceRoot,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := sessions.append(ctx, events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload: events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    1400,
			EstimatedCompletionTokens: 120,
			EstimatedInputCost:        0.0042,
			EstimatedOutputCost:       0.001,
		},
	}); err != nil {
		t.Fatalf("append recorded usage error = %v", err)
	}
	initial, err := sessions.BudgetStatus(ctx, "session-1", SessionConfig{
		TotalBudget: 1,
	})
	if err != nil {
		t.Fatalf("initial BudgetStatus() error = %v", err)
	}
	if math.Abs(initial.TotalCost-0.0052) > 1e-9 {
		t.Fatalf("initial TotalCost = %f, want 0.0052", initial.TotalCost)
	}
	if _, err := sessions.append(ctx, events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeTurnProviderUsageReported,
		Payload: events.TurnProviderUsageReportedPayload{
			Model:                "openai/gpt-5",
			RequestID:            "resp_123",
			Step:                 1,
			Attempt:              1,
			InputTokens:          1400,
			CacheReadInputTokens: 300,
			OutputTokens:         120,
			TotalTokens:          1520,
			EstimatedInputCost:   0.0014125,
			EstimatedOutputCost:  0.0012,
			CachePricingApplied:  true,
		},
	}); err != nil {
		t.Fatalf("append reported usage error = %v", err)
	}

	status, err := sessions.BudgetStatus(ctx, "session-1", SessionConfig{
		Budget:      0.003,
		BudgetWarn:  0.8,
		TotalBudget: 1,
	})
	if err != nil {
		t.Fatalf("BudgetStatus() error = %v", err)
	}

	if math.Abs(status.SessionCost-0.0026125) > 1e-9 {
		t.Fatalf("SessionCost = %f, want 0.0026125", status.SessionCost)
	}
	if math.Abs(status.TotalCost-0.0026125) > 1e-9 {
		t.Fatalf("TotalCost = %f, want 0.0026125", status.TotalCost)
	}
	if status.SessionExceeded {
		t.Fatalf("SessionExceeded = true, want false")
	}
	if status.SessionMissingPricingTurns != 0 {
		t.Fatalf("SessionMissingPricingTurns = %d, want 0", status.SessionMissingPricingTurns)
	}
}

func TestSessionServiceBudgetStatusDoesNotSnapshotColdSessionsForCrossSessionTotals(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	ctx := context.Background()
	workspaceRoot := t.TempDir()
	if _, err := sessions.CreateSession(ctx, CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: workspaceRoot,
	}); err != nil {
		t.Fatalf("CreateSession(session-1) error = %v", err)
	}
	if _, err := sessions.append(ctx, events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload: events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5-mini",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    120,
			EstimatedCompletionTokens: 24,
			EstimatedInputCost:        0.0002,
			EstimatedOutputCost:       0.0001,
		},
	}); err != nil {
		t.Fatalf("append current session usage error = %v", err)
	}

	appendCold := func(draft events.Draft) {
		t.Helper()
		if _, err := store.Append(ctx, draft); err != nil {
			t.Fatalf("store.Append(%s/%s/%s) error = %v", draft.SessionID, draft.TurnID, draft.Type, err)
		}
	}

	appendCold(events.Draft{
		SessionID: "session-2",
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionConfigured,
		Payload: events.SessionConfiguredPayload{
			WorkspaceRoot: workspaceRoot,
		},
	})
	for i := 0; i < 64; i++ {
		appendCold(events.Draft{
			SessionID: "session-2",
			TurnID:    "turn-cold",
			Type:      events.TypeTurnProviderUsageRecorded,
			Payload: events.TurnProviderUsageRecordedPayload{
				Model:                     "openai/gpt-5-mini",
				Step:                      i + 1,
				Attempt:                   1,
				EstimatedRequestTokens:    100,
				EstimatedCompletionTokens: 20,
				EstimatedInputCost:        0.0001,
				EstimatedOutputCost:       0.00005,
			},
		})
	}

	countSnapshots := func(sessionID string) int {
		t.Helper()
		replayed, err := store.Replay(ctx, events.Query{SessionID: sessionID, AfterSequence: -1})
		if err != nil {
			t.Fatalf("Replay(%s) error = %v", sessionID, err)
		}
		count := 0
		for _, event := range replayed {
			if event.Type == events.TypeSessionStateSnapshot {
				count++
			}
		}
		return count
	}

	if got := countSnapshots("session-2"); got != 0 {
		t.Fatalf("session-2 snapshot count before BudgetStatus = %d, want 0", got)
	}

	status, err := sessions.BudgetStatus(ctx, "session-1", SessionConfig{
		TotalBudget: 10,
	})
	if err != nil {
		t.Fatalf("BudgetStatus() error = %v", err)
	}
	if status.TotalCost <= status.SessionCost {
		t.Fatalf("TotalCost = %f, want cross-session total above SessionCost %f", status.TotalCost, status.SessionCost)
	}
	if got := countSnapshots("session-2"); got != 0 {
		t.Fatalf("session-2 snapshot count after BudgetStatus = %d, want 0", got)
	}
}

func TestSessionServiceBudgetStatusDoesNotSnapshotColdCurrentSession(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	ctx := context.Background()
	workspaceRoot := t.TempDir()
	appendCold := func(draft events.Draft) {
		t.Helper()
		if _, err := store.Append(ctx, draft); err != nil {
			t.Fatalf("store.Append(%s/%s/%s) error = %v", draft.SessionID, draft.TurnID, draft.Type, err)
		}
	}

	appendCold(events.Draft{
		SessionID: "session-1",
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionConfigured,
		Payload: events.SessionConfiguredPayload{
			WorkspaceRoot: workspaceRoot,
		},
	})
	appendCold(events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload: events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5-mini",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    120,
			EstimatedCompletionTokens: 24,
			EstimatedInputCost:        0.0002,
			EstimatedOutputCost:       0.0001,
		},
	})

	countSnapshots := func() int {
		t.Helper()
		replayed, err := store.Replay(ctx, events.Query{SessionID: "session-1", AfterSequence: -1})
		if err != nil {
			t.Fatalf("Replay(session-1) error = %v", err)
		}
		count := 0
		for _, event := range replayed {
			if event.Type == events.TypeSessionStateSnapshot {
				count++
			}
		}
		return count
	}

	if got := countSnapshots(); got != 0 {
		t.Fatalf("snapshot count before BudgetStatus = %d, want 0", got)
	}

	status, err := sessions.BudgetStatus(ctx, "session-1", SessionConfig{
		Budget: 10,
	})
	if err != nil {
		t.Fatalf("BudgetStatus() error = %v", err)
	}
	if status.SessionCost <= 0 {
		t.Fatalf("SessionCost = %f, want positive", status.SessionCost)
	}
	if got := countSnapshots(); got != 0 {
		t.Fatalf("snapshot count after BudgetStatus = %d, want 0", got)
	}
}

func TestSessionServiceBudgetStatusReturnsErrorWhenCrossSessionSummaryFails(t *testing.T) {
	store := &failingBudgetStore{
		MemoryStore: events.NewMemoryStore(),
		extraSessions: []events.SessionIndexEntry{
			{SessionID: "broken-session", WorkspaceRoot: t.TempDir()},
		},
		failReplayFor: "broken-session",
	}
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	ctx := context.Background()
	workspaceRoot := t.TempDir()
	if _, err := sessions.CreateSession(ctx, CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: workspaceRoot,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err = sessions.BudgetStatus(ctx, "session-1", SessionConfig{
		TotalBudget: 10,
	})
	if err == nil {
		t.Fatal("BudgetStatus() error = nil, want cross-session summary failure")
	}
	if !strings.Contains(err.Error(), "budget summary for broken-session") {
		t.Fatalf("BudgetStatus() error = %v, want broken-session context", err)
	}
}
