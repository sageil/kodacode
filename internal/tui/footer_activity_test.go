package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func testHistoryCompactionPayload() events.SessionHistoryContinuationUpdatedPayload {
	return testHistoryContinuationPayload(
		"Compaction Summary:\n## Critical Context\n- turn-1",
		"History summary updated: 1 turn 3.6k->1.8k",
		events.HistoryContinuationUpdateReasonTokenPressure,
		"turn-1",
		1,
		1,
	)
}

func TestHandleWatchEventsShowsTransientHistoryFooterActivity(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 1

	updated, _ := model.handleWatchEvents(1, []events.Event{
		draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
			WorkspaceRoot: "/repo",
		}),
		draftEvent(1, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", testHistoryCompactionPayload()),
		draftEvent(2, events.TypeContextPruned, "session-1", "turn-1", events.ContextPrunedPayload{
			PriorTurns:        4,
			RawPriorTurns:     2,
			OmittedPriorTurns: 1,
		}),
	}, false)
	model = updated.(Model)

	rendered := ansi.Strip(renderFooterNoticeBlock(model, model.projector.Snapshot(), 140))
	for _, want := range []string{
		"History summary updated: 1 turn",
		"History pruned: 1 prior turn",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("footer activity missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestHandleWatchEventsClearsTransientHistoryFooterActivityOnProgress(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 2

	updated, _ := model.handleWatchEvents(2, []events.Event{
		draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
			WorkspaceRoot: "/repo",
		}),
		draftEvent(1, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", testHistoryCompactionPayload()),
	}, false)
	model = updated.(Model)
	if model.footerNotice.activity == nil {
		t.Fatal("footerActivity = nil, want active transient notice")
	}

	updated, _ = model.handleWatchEvents(2, []events.Event{
		draftEvent(2, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
			Content: "continuing",
		}),
	}, false)
	model = updated.(Model)

	if model.footerNotice.activity != nil {
		t.Fatalf("footerActivity = %#v, want cleared after progress", model.footerNotice.activity)
	}
	rendered := ansi.Strip(renderFooterNoticeBlock(model, model.projector.Snapshot(), 140))
	if strings.Contains(rendered, "History summary updated") || strings.Contains(rendered, "History pruned") {
		t.Fatalf("footer still shows cleared history activity\nrendered:\n%s", rendered)
	}
}

func TestHandleWatchEventsMergesAdjacentHistoryFooterNotices(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 4

	updated, _ := model.handleWatchEvents(4, []events.Event{
		draftEvent(0, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", testHistoryCompactionPayload()),
	}, false)
	model = updated.(Model)

	updated, _ = model.handleWatchEvents(4, []events.Event{
		draftEvent(1, events.TypeContextPruned, "session-1", "turn-1", events.ContextPrunedPayload{
			PriorTurns:        3,
			RawPriorTurns:     2,
			OmittedPriorTurns: 1,
		}),
	}, false)
	model = updated.(Model)

	rendered := ansi.Strip(renderFooterNoticeBlock(model, model.projector.Snapshot(), 140))
	for _, want := range []string{
		"History summary updated: 1 turn",
		"History pruned: 1 prior turn",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("merged footer activity missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestHandleWatchEventsShowsHistoryFooterActivityWhenLaterProgressIsInSameBatch(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 5

	updated, _ := model.handleWatchEvents(5, []events.Event{
		draftEvent(0, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", testHistoryCompactionPayload()),
		draftEvent(1, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
			Content: "continuing",
		}),
	}, false)
	model = updated.(Model)

	if model.footerNotice.activity == nil {
		t.Fatal("footerActivity = nil, want transient notice even when later progress shares the batch")
	}
	rendered := ansi.Strip(renderFooterNoticeBlock(model, model.projector.Snapshot(), 140))
	if !strings.Contains(rendered, "History summary updated: 1 turn") {
		t.Fatalf("footer missing same-batch history activity\nrendered:\n%s", rendered)
	}

	updated, _ = model.handleWatchEvents(5, []events.Event{
		draftEvent(2, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
			Content: "more progress",
		}),
	}, false)
	model = updated.(Model)
	if model.footerNotice.activity != nil {
		t.Fatalf("footerActivity = %#v, want cleared by later progress in the next batch", model.footerNotice.activity)
	}
}

func TestFooterActivityExpires(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 6

	updated, _ := model.handleWatchEvents(6, []events.Event{
		draftEvent(0, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", testHistoryCompactionPayload()),
	}, false)
	model = updated.(Model)
	if model.footerNotice.activity == nil {
		t.Fatal("footerActivity = nil, want active transient notice")
	}

	updatedModel, _ := model.Update(footerActivityExpiredMsg{id: model.footerNotice.activity.id})
	model = updatedModel.(Model)
	if model.footerNotice.activity != nil {
		t.Fatalf("footerActivity = %#v, want cleared after expiry", model.footerNotice.activity)
	}
}

func TestFooterActivityDoesNotRelayoutTranscriptHeightOnShowAndExpire(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = modelIface.(Model)
	model.watchID = 7

	baseHeight := model.messages.height

	updated, _ := model.handleWatchEvents(7, []events.Event{
		draftEvent(0, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", testHistoryCompactionPayload()),
	}, false)
	model = updated.(Model)

	if model.messages.height != baseHeight {
		t.Fatalf("messages height = %d, want stable %d after footer activity", model.messages.height, baseHeight)
	}

	expireID := model.footerNotice.activity.id
	updatedModel, _ := model.Update(footerActivityExpiredMsg{id: expireID})
	model = updatedModel.(Model)

	if model.messages.height != baseHeight {
		t.Fatalf("messages height = %d, want stable %d after footer activity expiry", model.messages.height, baseHeight)
	}
}

func TestHistoryCompactionActivityTextIncludesTotalAndNewCounts(t *testing.T) {
	rendered := historyCompactionActivityText(events.SessionHistoryContinuationUpdatedPayload{
		ActivityText: "History summary updated: 5 turns total (2 turns new) 104.0k->69.7k",
	})

	for _, want := range []string{
		"History summary updated: 5 turns total (2 turns new)",
		"104.0k->69.7k",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("history compaction activity = %q, want substring %q", rendered, want)
		}
	}
}

func TestHandleWatchEventsShowsHistoryCompactionFailedFooterActivity(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 9

	updated, _ := model.handleWatchEvents(9, []events.Event{
		draftEvent(0, events.TypeContextCompactionFailed, "session-1", "turn-1", events.ContextCompactionFailedPayload{
			Scope:                  events.CompactionScopeHistory,
			Reason:                 "artifact_generation_failed",
			Detail:                 "session store append failed",
			InputLimitTokens:       3072,
			TriggerTokens:          2560,
			TargetTokens:           2048,
			EstimatedRequestTokens: 3600,
		}),
	}, false)
	model = updated.(Model)

	if model.footerNotice.activity == nil {
		t.Fatal("footerActivity = nil, want history-compaction failure activity")
	}
	rendered := ansi.Strip(renderFooterNoticeBlock(model, model.projector.Snapshot(), 140))
	if !strings.Contains(rendered, historySummaryFailureLabel) {
		t.Fatalf("footer activity missing %q\nrendered:\n%s", historySummaryFailureLabel, rendered)
	}
}
