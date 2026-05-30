package app

import (
	"context"
	"errors"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func TestRuntimeGenerateBranchSummaryUsesUtilityModelAndCachesBySequence(t *testing.T) {
	ctx := context.Background()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "- branch adjusted cache flow"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "- branch added retry guard"}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {
				{ID: "gpt-5-mini", ContextSize: 128000, MaxInputTokens: 128000, MaxOutputTokens: 16384, CostInput: 0.25, CostOutput: 2},
				{ID: "gpt-5", ContextSize: 128000, MaxInputTokens: 128000, MaxOutputTokens: 16384, CostInput: 1.25, CostOutput: 10},
			},
		},
	}

	sourceSessionID, err := runtime.CreateSession(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	appendTimelineTurnForTest(t, runtime.Sessions, sourceSessionID, "turn-1", "change cache", "cache changed")
	branch, err := runtime.BranchSessionFromTurn(ctx, BranchSessionFromTurnInput{
		SourceSessionID: sourceSessionID,
		SourceTurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("BranchSessionFromTurn() error = %v", err)
	}

	first, err := runtime.GenerateBranchSummary(ctx, GenerateBranchSummaryInput{SessionID: branch.SessionID})
	if err != nil {
		t.Fatalf("GenerateBranchSummary(first) error = %v", err)
	}
	if first.Cached || first.Summary != "- branch adjusted cache flow" {
		t.Fatalf("first summary = %#v", first)
	}
	if len(client.requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("request model = %q, want utility model", got)
	}

	second, err := runtime.GenerateBranchSummary(ctx, GenerateBranchSummaryInput{SessionID: branch.SessionID})
	if err != nil {
		t.Fatalf("GenerateBranchSummary(second) error = %v", err)
	}
	if !second.Cached || second.Summary != first.Summary {
		t.Fatalf("second summary = %#v, want cached %#v", second, first)
	}
	if len(client.requests) != 1 {
		t.Fatalf("cached request count = %d, want 1", len(client.requests))
	}

	appendTimelineTurnForTest(t, runtime.Sessions, branch.SessionID, "turn-2", "add retry guard", "retry guard added")
	third, err := runtime.GenerateBranchSummary(ctx, GenerateBranchSummaryInput{SessionID: branch.SessionID})
	if err != nil {
		t.Fatalf("GenerateBranchSummary(third) error = %v", err)
	}
	if third.Cached || third.Summary != "- branch added retry guard" {
		t.Fatalf("third summary = %#v", third)
	}
	if len(client.requests) != 2 {
		t.Fatalf("regenerated request count = %d, want 2", len(client.requests))
	}

	state, err := runtime.Sessions.Snapshot(ctx, branch.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	var found bool
	for _, attempt := range state.Turns["turn-2"].ProviderAttempts {
		if attempt.Kind == string(events.TurnProviderUsageKindUtilityBranchSummary) {
			found = true
		}
	}
	if !found {
		t.Fatalf("turn-2 provider attempts = %#v, want branch summary usage", state.Turns["turn-2"].ProviderAttempts)
	}
}

func TestRuntimeGenerateBranchSummaryDoesNotFallbackToPrimaryModel(t *testing.T) {
	ctx := context.Background()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "should not run"}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sourceSessionID, err := runtime.CreateSession(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	appendTimelineTurnForTest(t, runtime.Sessions, sourceSessionID, "turn-1", "change cache", "cache changed")
	branch, err := runtime.BranchSessionFromTurn(ctx, BranchSessionFromTurnInput{
		SourceSessionID: sourceSessionID,
		SourceTurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("BranchSessionFromTurn() error = %v", err)
	}

	_, err = runtime.GenerateBranchSummary(ctx, GenerateBranchSummaryInput{SessionID: branch.SessionID})
	if !errors.Is(err, provider.ErrProviderNotConfigured) {
		t.Fatalf("GenerateBranchSummary() error = %v, want provider not configured", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("request count = %d, want no primary-model fallback", len(client.requests))
	}
}
