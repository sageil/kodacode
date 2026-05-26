package tui

import "github.com/sageil/kodacode/internal/events"

func testHistoryContinuationPayload(summary, activity, reason, frontier string, consolidated, newly int) events.SessionHistoryContinuationUpdatedPayload {
	if reason == "" {
		reason = events.HistoryContinuationUpdateReasonTokenPressure
	}
	if consolidated > 0 && frontier == "" {
		frontier = "turn-1"
	}
	return events.SessionHistoryContinuationUpdatedPayload{
		UpdateReason:               reason,
		ActivityText:               activity,
		RenderedSummary:            summary,
		FrontierTurnID:             frontier,
		ConsolidatedTurnCount:      consolidated,
		NewlyConsolidatedTurnCount: newly,
	}
}

func testHistoryContinuationState(summary, activity, reason, frontier string, consolidated, newly int) *events.HistoryContinuationState {
	payload := testHistoryContinuationPayload(summary, activity, reason, frontier, consolidated, newly)
	return &payload
}
