package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

const footerNoticeMaxLines = 3

func renderFooterNoticeBlock(m Model, state events.SessionState, width int) string {
	text, tone := footerNoticeText(m, state)
	if strings.TrimSpace(text) == "" {
		return ""
	}

	accentColor := colorFor(m.theme, "warning", "#ffd28f")
	if tone == footerActivityToneInfo {
		accentColor = colorFor(m.theme, "primary", "#7cc7ff")
	}
	if tone == footerActivityToneError {
		accentColor = colorFor(m.theme, "error", "#ff9aa6")
	}
	textColor := colorFor(m.theme, "subtext", "#9da8ca")

	prefix := " ! "
	bodyWidth := max(width-len(prefix), 1)
	lines := clampFooterNoticeLines(wrapTranscriptText(strings.TrimSpace(text), bodyWidth), bodyWidth)
	if len(lines) == 0 {
		return ""
	}

	rendered := make([]string, 0, len(lines))
	accent := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentColor)).
		Render("!")
	for i, line := range lines {
		content := lipgloss.NewStyle().
			Foreground(lipgloss.Color(textColor)).
			Render(line)
		if i == 0 {
			rendered = append(rendered, " "+accent+" "+content)
			continue
		}
		rendered = append(rendered, "   "+content)
	}
	return lipgloss.NewStyle().
		Width(max(width, 1)).
		Render(strings.Join(rendered, "\n"))
}

func footerNoticeText(m Model, state events.SessionState) (string, footerActivityTone) {
	if text := strings.TrimSpace(m.footerNotice.err); text != "" {
		return text, footerActivityToneError
	}
	if text := strings.TrimSpace(m.composerState.err); text != "" {
		return text, footerActivityToneError
	}
	if text := footerTurnFailureNoticeText(m, state); text != "" {
		return text, footerActivityToneError
	}
	return footerActivityText(m)
}

func footerTurnFailureNoticeText(m Model, state events.SessionState) string {
	turn := currentTurn(state, m.turnID)
	if turn == nil || turn.Status != events.TurnStatusFailed {
		return ""
	}
	label := "Failed"
	switch {
	case turn.ErrorRetryable:
		label = "Temporary error"
	case isTurnStalled(turn):
		label = "Stalled"
	case turn.ErrorCode == events.TurnFailureCodeBudgetExceeded:
		label = "Budget exceeded"
	}
	if detail := strings.TrimSpace(turn.Error); detail != "" {
		return label + " " + detail
	}
	return label
}

func footerActivityText(m Model) (string, footerActivityTone) {
	if m.footerNotice.activity == nil || strings.TrimSpace(m.footerNotice.activity.text) == "" {
		return "", ""
	}
	return strings.TrimSpace(m.footerNotice.activity.text), m.footerNotice.activity.tone
}

func clampFooterNoticeLines(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	if len(lines) <= footerNoticeMaxLines {
		return lines
	}
	clamped := append([]string(nil), lines[:footerNoticeMaxLines]...)
	clamped[len(clamped)-1] = truncateEnd(clamped[len(clamped)-1], max(width, 1))
	return clamped
}
