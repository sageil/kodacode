package tui

import (
	"testing"

	"github.com/sageil/kodacode/internal/events"
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
