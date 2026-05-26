package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestRuntimeReuseDelegatedResultPersistsAndReplaysToNextParentTurn(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "follow up"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "planner",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	delegated, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}

	reused, err := runtime.ReuseDelegatedResult(context.Background(), ReuseDelegatedResultInput{
		ParentSessionID: sessionID,
		HandoffID:       delegated.HandoffID,
	})
	if err != nil {
		t.Fatalf("ReuseDelegatedResult() error = %v", err)
	}
	if reused.Content == "" {
		t.Fatal("reused content = empty")
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	handoff := parentState.Turns["turn-1"].Handoffs[delegated.HandoffID]
	if handoff == nil {
		t.Fatal("handoff state missing")
	}
	if !handoff.Reused || handoff.ReusedContent != reused.Content {
		t.Fatalf("handoff reuse state = %#v", handoff)
	}

	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-2",
		UserText:  "follow up",
		AgentID:   "planner",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(turn-2) error = %v", err)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	next := client.requests[2]
	found := false
	for _, input := range next.Inputs {
		if input.Kind == provider.InputKindAssistantMessage && input.Content == reused.Content {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("next turn inputs = %#v, want reused result content", next.Inputs)
	}
}

func TestRuntimeReuseDelegatedResultRejectsDuplicateReuse(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "planner",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	delegated, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}

	if _, err := runtime.ReuseDelegatedResult(context.Background(), ReuseDelegatedResultInput{
		ParentSessionID: sessionID,
		HandoffID:       delegated.HandoffID,
	}); err != nil {
		t.Fatalf("ReuseDelegatedResult(first) error = %v", err)
	}
	if _, err := runtime.ReuseDelegatedResult(context.Background(), ReuseDelegatedResultInput{
		ParentSessionID: sessionID,
		HandoffID:       delegated.HandoffID,
	}); err != ErrHandoffResultAlreadyReused {
		t.Fatalf("ReuseDelegatedResult(second) error = %v, want %v", err, ErrHandoffResultAlreadyReused)
	}
}
