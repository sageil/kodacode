package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

var liveSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m Model) liveTurnSpinnerState(state events.SessionState) (bool, string) {
	turnID := effectiveLiveTurnID(m, state)
	turn := currentTurn(state, turnID)
	if turn != nil && eventsHistoryCompactionUIActive(turn) {
		return true, historySummarizingStatusLabel
	}
	if !m.liveTurn.spinnerArmed {
		return false, ""
	}
	pendingSubmission := m.pendingInteractionSubmissionInFlightForState(state)
	if pendingSubmission {
		return true, "Continuing"
	}
	trackedTurnID := strings.TrimSpace(m.turnID)
	if turnID == "" {
		if trackedTurnID == "" {
			return false, ""
		}
		if turn := currentTurn(state, trackedTurnID); turn != nil && isTurnFinished(turn) {
			return false, ""
		}
	}
	if hasPendingInteractionInState(state, turnID) && !pendingSubmission {
		return false, ""
	}
	if turn == nil {
		return true, "Starting turn"
	}
	if turn.Status != events.TurnStatusRunning {
		return false, ""
	}
	if eventsHistoryCompactionUIActive(turn) {
		return true, historySummarizingStatusLabel
	}
	if retryLabel := liveTurnRetryLabel(turn); retryLabel != "" {
		return true, retryLabel
	}
	if liveTurnHasExecutingTool(turn) {
		return true, "Running tools"
	}
	if strings.TrimSpace(turn.StreamingText) != "" {
		return true, "Streaming"
	}
	if strings.TrimSpace(turn.ReasoningText) != "" {
		return true, "Thinking"
	}
	return true, "Waiting for model"
}

func (m Model) shouldAnimateTranscriptActivity() bool {
	state := m.projector.CurrentState()
	return m.shouldAnimateTranscriptActivityForState(state)
}

func (m Model) shouldAnimateTranscriptActivityForState(state events.SessionState) bool {
	if active, _ := m.liveTurnSpinnerState(state); active {
		return true
	}
	if !m.busy {
		return false
	}
	turnID := strings.TrimSpace(m.turnID)
	if turnID == "" {
		return false
	}
	if turn := currentTurn(state, turnID); turn != nil {
		if turn.Status == events.TurnStatusRunning {
			return true
		}
	} else {
		return true
	}
	for _, ref := range orderedSessionToolCallRefs(state) {
		if turn := currentTurn(state, ref.TurnID); turn == nil || turn.Status != events.TurnStatusRunning {
			continue
		}
		_, call := sessionToolCall(state, ref)
		if call != nil && toolOutcomeShowsSpinner(toolStatus(call)) {
			return true
		}
	}
	return false
}

func eventsHistoryCompactionUIActive(turn *events.TurnState) bool {
	if turn == nil || turn.Status != events.TurnStatusRunning {
		return false
	}
	ui := turn.HistoryCompactionUI
	if ui == nil {
		return false
	}
	if ui.Scope != events.CompactionScopeHistory {
		return false
	}
	if ui.StartedAtSeq <= 0 {
		return false
	}
	return ui.SummaryReadyAtSeq == 0
}

func renderLiveSpinner(m Model) string {
	frame := liveSpinnerFrames[0]
	if len(liveSpinnerFrames) > 0 {
		frame = liveSpinnerFrames[m.animation.frame%len(liveSpinnerFrames)]
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
		Render(frame)
}

func liveTurnRetryLabel(turn *events.TurnState) string {
	if turn == nil || turn.Retry == nil {
		return ""
	}
	wait := time.Until(turn.Retry.RetryAt)
	if wait <= 0 {
		return fmt.Sprintf("Retrying now (%d/%d)", turn.Retry.Attempt, turn.Retry.MaxAttempts)
	}
	return fmt.Sprintf("Retrying in %s (%d/%d)", renderRetryWait(wait), turn.Retry.Attempt, turn.Retry.MaxAttempts)
}

func renderRetryWait(wait time.Duration) string {
	if wait <= 0 {
		return "0s"
	}
	if wait < time.Second {
		return wait.Round(100 * time.Millisecond).String()
	}
	return wait.Round(time.Second).String()
}

func liveTurnHasExecutingTool(turn *events.TurnState) bool {
	if turn == nil {
		return false
	}
	for _, callID := range orderedToolCallIDs(turn) {
		call := turn.ToolCalls[callID]
		if call == nil {
			continue
		}
		if call.Executing || (call.Declared && !call.Completed) {
			return true
		}
	}
	return false
}
