package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestShouldBatchWatchEventBatchesRapidVisibleTurnEvents(t *testing.T) {
	batched := []events.Type{
		events.TypeAssistantPreviewDelta,
		events.TypeReasoningDelta,
		events.TypeToolCallDeclared,
		events.TypeToolExecStart,
		events.TypeToolExecEnd,
		events.TypeExecutionDeclared,
		events.TypeExecutionStarted,
		events.TypeExecutionBackgroundObserved,
		events.TypeAgentHandoffPreview,
		events.TypeTurnWorkStateUpdated,
	}
	for _, eventType := range batched {
		if !shouldBatchWatchEvent(events.Event{Type: eventType}) {
			t.Fatalf("shouldBatchWatchEvent(%s) = false, want true", eventType)
		}
	}
}

func TestShouldBatchWatchEventKeepsInteractionAndTerminalEventsImmediate(t *testing.T) {
	immediate := []events.Type{
		events.TypeAssistantCommit,
		events.TypePermissionRequested,
		events.TypeQuestionRequested,
		events.TypeTurnDone,
		events.TypeTurnError,
		events.TypeTurnCanceled,
	}
	for _, eventType := range immediate {
		if shouldBatchWatchEvent(events.Event{Type: eventType}) {
			t.Fatalf("shouldBatchWatchEvent(%s) = true, want false", eventType)
		}
	}
}

func TestTurnCompletionFocusesComposerWhenTranscriptScrolledAwayFromBottom(t *testing.T) {
	for _, layout := range []string{tuiLayoutClassic, tuiLayoutShell} {
		t.Run(layout, func(t *testing.T) {
			defaultTheme := theme.StaticDefault()
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			model := NewModel(&fakeController{}, ModelConfig{
				Context:       ctx,
				Theme:         &defaultTheme,
				SessionID:     "session-1",
				TurnID:        "turn-1",
				WorkspaceRoot: "/repo",
				UserText:      "say hello",
				Layout:        layout,
			})
			model.watchID = 7
			model.busy = true
			model.chrome.focus = focusTranscript
			model.messages.SetSize(80, 3)
			model.messages.Sync(strings.Repeat("transcript line\n", 20), false)
			model.messages.GotoTop()
			if model.messages.AtBottom() {
				t.Fatal("test setup kept transcript at bottom")
			}

			updated, _ := model.handleWatchEvents(model.watchID, []events.Event{
				draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
					WorkspaceRoot: "/repo",
				}),
				draftEvent(1, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
					Content: "say hello",
				}),
				draftEvent(2, events.TypeTurnConfigured, "session-1", "turn-1", events.TurnConfiguredPayload{
					AgentID: "builder",
					Model:   "openai/gpt-5",
				}),
				draftEvent(3, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}),
			}, false)
			next := updated.(Model)
			if next.chrome.focus != focusComposer {
				t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
			}
		})
	}
}
