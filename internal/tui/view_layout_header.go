package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/sessiontitle"
)

func renderHeaderBar(m Model, state events.SessionState, width int) string {
	modeColor := shellModeColor(m)
	modeName := shellModeName(m)
	modeTag := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#1f1726")).
		Background(lipgloss.Color(modeColor)).
		Bold(true).
		Padding(0, 1).
		Render(modeName)

	pipe := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m))).
		Render(" │ ")
	brand := renderHeaderBrand(m)

	modelZone := headerModelZone(m, state, max(width/3, 8))
	titleDivider := ""
	if modelZone != "" {
		titleDivider = pipe
	}

	var left, center string

	if ctxZone := headerContextMetricsZone(m, state); ctxZone != "" {
		center = headerCenterZone(modelZone, pipe, ctxZone)
		prefixWidth := lipgloss.Width(modeTag) + lipgloss.Width(pipe) + lipgloss.Width(brand) + lipgloss.Width(pipe) + lipgloss.Width(titleDivider)
		titleAvail := max(width-prefixWidth-lipgloss.Width(center)-1, 4)
		sessionTitle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "text", "#f8f8f2"))).
			Bold(true).
			Render(truncateEnd(shellSessionLabel(m, state), titleAvail))
		left = modeTag + pipe + brand + pipe + sessionTitle + titleDivider
	} else {
		infoSep := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render(" · ")
		center = joinCompactTextParts(infoSep, modelZone)
		prefixWidth := lipgloss.Width(modeTag) + lipgloss.Width(pipe) + lipgloss.Width(brand) + lipgloss.Width(pipe) + lipgloss.Width(titleDivider)
		titleAvail := max(width-prefixWidth-lipgloss.Width(center)-1, 4)
		sessionTag := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render(truncateEnd(shellSessionLabel(m, state), titleAvail))
		left = modeTag + pipe + brand + pipe + sessionTag + titleDivider
	}

	row := renderHeaderLeftCenterRow(left, center, max(width, 1))
	return row + "\n" + renderHeaderDivider(m, max(width, 1))
}

func renderHeaderBrand(m Model) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#e6b450"))).
		Bold(true).
		Render("KodaCode")
}

func sessionCardTitle(m Model, state events.SessionState) string {
	if title := strings.TrimSpace(state.Title); title != "" {
		return truncateEnd(displaySessionTitle(title), 20)
	}
	return truncateEnd(defaultSessionTitle(state), 20)
}

func headerSessionTitle(m Model, state events.SessionState) string {
	if title := strings.TrimSpace(state.Title); title != "" {
		return displaySessionTitle(title)
	}
	return defaultSessionTitle(state)
}

func displaySessionTitle(raw string) string {
	title := sessiontitle.Normalize(raw)
	if title == "" {
		return defaultSessionTitle(events.SessionState{})
	}
	return title
}

func defaultSessionTitle(state events.SessionState) string {
	return "Workspace session"
}

func sessionCardMeta(m Model, state events.SessionState) string {
	status := renderStatus(m, state)
	lines := []string{compactID(m.turnID) + " • " + status}
	if turnCount := len(orderedSessionTurnIDs(state)); turnCount > 0 {
		lines = append(lines, fmt.Sprintf("%d turns", turnCount))
	}
	if len(m.skillIDs) > 0 {
		lines = append(lines, "skills "+strings.Join(m.skillIDs, ", "))
	}
	lines = append(lines, displaySessionPath(state.WorkspaceRoot, pickWorkspace(state.WorkspaceRoot, m.workspace)))
	return strings.Join(lines, "\n")
}

func sessionHasVisibleTools(state events.SessionState) bool {
	return len(orderedSessionToolCallRefs(state)) > 0
}

func joinBar(left, right string, width int) string {
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := max(width-leftWidth-rightWidth, 1)
	return left + strings.Repeat(" ", gap) + right
}

func renderHeaderDivider(m Model, width int) string {
	width = max(width, 1)
	if !m.shouldAnimateTranscriptActivity() {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(lineTone(m))).
			Render(strings.Repeat("─", width))
	}
	return sweepRowDivider("─", width, m.animation.frame, colorFor(m.theme, "primary", "#7cc7ff"), lineTone(m))
}
