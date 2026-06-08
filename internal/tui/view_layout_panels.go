package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderSidePanel(m Model, state events.SessionState, width int) string {
	return renderSessionsPanel(m, state, width)
}

func renderSessionsPanel(m Model, state events.SessionState, width int) string {
	header := renderSessionsPanelHeader(m, state, width)
	body := ""
	if m.sessionsBody.Width() > 0 {
		body = m.sessionsBody.View()
	} else {
		body = renderSessionsPanelBody(m, state, width)
	}
	return header + "\n\n" + body
}

func renderSessionsPanelHeader(m Model, state events.SessionState, width int) string {
	activeCount := 1
	focused := false
	tag := renderPanelTag(m, "Sessions", focused)
	countLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color(panelContentColor(m, focused))).
		Render(fmt.Sprintf("%d active", activeCount))
	return tag + " " + countLabel
}

func renderSessionsPanelBody(m Model, state events.SessionState, width int) string {
	sections := []string{
		renderSessionRailCard(m, width, true, sessionCardTitle(m, state), "now", sessionCardMeta(m, state)),
		renderSectionLabel(m, "Shortcuts", "focus", width, ""),
		renderShortcutList(m, state, currentTurn(state, m.turnID), width),
	}
	return strings.Join(sections, "\n\n")
}

func renderFooterBar(m Model, state events.SessionState, width int) string {
	composer := renderComposerBar(m, state, width)
	statusLine := renderFooterStatusBar(m, state, width)
	hintsLine := renderFooterHintsLine(m, state, width)
	lines := make([]string, 0, 3)
	lines = append(lines, composer)
	if strings.TrimSpace(statusLine) != "" {
		lines = append(lines, statusLine)
	}
	if strings.TrimSpace(hintsLine) != "" {
		lines = append(lines, hintsLine)
	}
	return lipgloss.NewStyle().
		Width(max(width, 1)).
		Render(strings.Join(lines, "\n"))
}

func renderShortcutList(m Model, state events.SessionState, turn *events.TurnState, width int) string {
	_ = turn
	innerWidth := max(width-4, 1)
	hasTools := sessionHasVisibleTools(state)
	rows := []string{
		renderShortcutRow(m, "Sessions", "1", innerWidth, false),
		renderShortcutRow(m, "Transcript", "2", innerWidth, false),
		renderShortcutRow(m, "Inspector", "3", innerWidth, false),
		renderShortcutRow(m, "Composer", "4", innerWidth, false),
		renderShortcutRow(m, "Agent", "Tab", innerWidth, false),
		renderShortcutRow(m, "Timeline", "/timeline", innerWidth, false),
		renderShortcutRow(m, "Model", "^M", innerWidth, false),
		renderShortcutRow(m, "Theme", "^T", innerWidth, false),
		renderShortcutRow(m, "Connect", "^P", innerWidth, false),
	}
	if hasTools {
		rows = append(rows, renderShortcutRow(m, "Tools", "J K", innerWidth, true))
	} else {
		rows[len(rows)-1] = renderShortcutRow(m, "Connect", "^P", innerWidth, true)
	}
	return lipgloss.NewStyle().
		Width(max(width, 1)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(lineTone(m))).
		Padding(0, 1).
		Render(strings.Join(rows, "\n"))
}

func renderShortcutRow(m Model, label, key string, width int, last bool) string {
	left := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "soft", softTextColor))).
		Render(label)
	row := joinBar(left, renderKeycap(m, key), width)
	if last {
		return row
	}
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m))).
		Render(strings.Repeat("─", max(width, 1)))
	return row + "\n" + sep
}
