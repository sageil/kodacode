package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

type transcriptStatusSegment struct {
	Text  string
	Color string
	Bold  bool
}

func transcriptStatusMeta(m Model, state events.SessionState) (string, string) {
	turnID := effectiveFooterTurnID(m, state)
	turn := currentTurn(state, turnID)
	parts := make([]string, 0, 3)
	if elapsed := m.liveTurnElapsed(); elapsed > 0 {
		parts = append(parts, renderActivityElapsed(elapsed))
	}
	if turn != nil && turn.Retry != nil {
		wait := time.Until(turn.Retry.RetryAt)
		if wait > 0 {
			parts = append(parts, "retry in "+renderRetryWait(wait))
		}
	}
	if m.turnCancellationAvailable() {
		parts = append(parts, "esc to interrupt")
	}
	return formatStatusMeta(parts), colorFor(m.theme, "subtext", "#9da8ca")
}

func formatStatusMeta(parts []string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return "(" + strings.Join(filtered, " • ") + ")"
}

func renderTranscriptStatusSegments(m Model, segments []transcriptStatusSegment, width int) string {
	width = max(width, 1)
	fitted := fitTranscriptStatusSegments(segments, width)
	if len(fitted) == 0 {
		return ""
	}

	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#6b7190"))).
		Render(" · ")

	parts := make([]string, 0, len(fitted)*2-1)
	for idx, segment := range fitted {
		if idx > 0 {
			parts = append(parts, separator)
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(segment.Color))
		if segment.Bold {
			style = style.Bold(true)
		}
		parts = append(parts, style.Render(segment.Text))
	}
	return strings.Join(parts, "")
}

func fitTranscriptStatusSegments(segments []transcriptStatusSegment, width int) []transcriptStatusSegment {
	if len(segments) == 0 || width <= 0 {
		return nil
	}
	const separatorWidth = 3
	out := make([]transcriptStatusSegment, 0, len(segments))
	used := 0
	for _, segment := range segments {
		text := strings.TrimSpace(segment.Text)
		if text == "" {
			continue
		}
		needed := lipgloss.Width(text)
		if len(out) > 0 {
			needed += separatorWidth
		}
		if used+needed <= width {
			out = append(out, transcriptStatusSegment{
				Text:  text,
				Color: segment.Color,
				Bold:  segment.Bold,
			})
			used += needed
			continue
		}
		if len(out) == 0 {
			out = append(out, transcriptStatusSegment{
				Text:  truncateEnd(text, width),
				Color: segment.Color,
				Bold:  segment.Bold,
			})
		}
		break
	}
	return out
}

func currentTurnActiveToolCount(turn *events.TurnState) int {
	if turn == nil || isTurnFinished(turn) {
		return 0
	}
	count := 0
	for _, callID := range orderedToolCallIDs(turn) {
		call := turn.ToolCalls[callID]
		if call == nil {
			continue
		}
		if call.Executing || (call.Declared && !call.Completed) {
			count++
		}
	}
	return count
}

func currentTurnPruningLabel(turn *events.TurnState) string {
	if turn == nil || turn.Pruning == nil {
		return ""
	}
	if turn.Pruning.OmittedPriorTurns > 0 {
		return historyPrunedSignal
	}
	return ""
}

func isTurnStalled(turn *events.TurnState) bool {
	if turn == nil {
		return false
	}
	switch turn.ErrorCode {
	case events.TurnFailureCodeProviderRequestLimit, events.TurnFailureCodeNoProgress:
		return true
	default:
		return false
	}
}

func (m Model) liveTurnElapsed() time.Duration {
	if m.liveTurn.startedAt.IsZero() {
		return 0
	}
	elapsed := time.Since(m.liveTurn.startedAt)
	if elapsed < 0 {
		return 0
	}
	return elapsed
}
