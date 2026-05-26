package app

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func TestTurnRunnerRunFailsBeforeProviderCallWhenSessionBudgetExceeded(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-0",
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload: events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    500,
			EstimatedCompletionTokens: 100,
			EstimatedInputCost:        0.3,
			EstimatedOutputCost:       0.2,
		},
	}); err != nil {
		t.Fatalf("append usage error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{Budget: 0.4})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		UserText:     "summarize the files",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{"read"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("Run() status = %q, want failed", result.Status)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want 0", len(client.requests))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusFailed {
		t.Fatalf("turn = %#v, want failed turn", turn)
	}
	if !strings.Contains(turn.Error, "Session budget reached") {
		t.Fatalf("turn error = %q", turn.Error)
	}
}

func TestTurnRunnerRunFailsBeforeProviderCallWhenCrossSessionBudgetExceeded(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	ctx := context.Background()
	for _, sessionID := range []string{"session-1", "session-2"} {
		if _, err := sessions.CreateSession(ctx, CreateSessionInput{
			SessionID:     sessionID,
			WorkspaceRoot: t.TempDir(),
		}); err != nil {
			t.Fatalf("CreateSession(%q) error = %v", sessionID, err)
		}
	}
	if _, err := sessions.append(ctx, events.Draft{
		SessionID: "session-2",
		TurnID:    "turn-0",
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload: events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    600,
			EstimatedCompletionTokens: 120,
			EstimatedInputCost:        0.35,
			EstimatedOutputCost:       0.25,
		},
	}); err != nil {
		t.Fatalf("append usage error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "hello"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{TotalBudget: 0.5})

	result, err := runner.Run(ctx, RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		UserText:     "summarize the files",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{"read"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("Run() status = %q, want failed", result.Status)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want 0", len(client.requests))
	}

	state, err := sessions.Snapshot(ctx, "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Status != events.TurnStatusFailed {
		t.Fatalf("turn = %#v, want failed turn", turn)
	}
	if !strings.Contains(turn.Error, "Cross-session budget reached") {
		t.Fatalf("turn error = %q", turn.Error)
	}
}
