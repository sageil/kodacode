package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderSplitComposerPane(m Model, state events.SessionState, width int) string {
	disabledMessage := strings.TrimSpace(m.composerDisabledMessage(state))
	disabled := disabledMessage != "" && !m.hasPendingInteraction()
	border := lineTone(m)
	if m.chrome.focus == focusComposer {
		border = colorFor(m.theme, "primary", "#7cc7ff")
	}
	if m.hasPendingInteraction() {
		border = colorFor(m.theme, "warning", "#ffd28f")
	} else if disabled {
		border = colorFor(m.theme, "warning", "#ffd28f")
	} else if strings.TrimSpace(m.composerState.err) != "" {
		border = colorFor(m.theme, "error", "#ff9aa6")
	}
	divider := renderComposerActivityStrip(m, state, width, border)

	content := strings.TrimRight(m.composer.View(), "\n")
	bodyHeight := max(lipgloss.Height(content), splitComposerMinHeight())
	if m.hasPendingInteraction() {
		content = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "warning", "#ffd28f"))).
			Render(composerBlockedMessage(m))
		bodyHeight = max(lipgloss.Height(content), 1)
	} else if disabled {
		content = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "warning", "#ffd28f"))).
			Render(disabledMessage)
		bodyHeight = max(lipgloss.Height(content), 1)
	}
	body := placeBlock(max(width, 1), max(bodyHeight, 1), "", content)
	parts := make([]string, 0, 2)
	if strings.TrimSpace(divider) != "" {
		parts = append(parts, divider)
	}
	parts = append(parts, body)
	return strings.Join(parts, "\n")
}
