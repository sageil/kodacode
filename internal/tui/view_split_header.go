package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderSplitWideHeader(m Model, state events.SessionState, width int) string {
	modeTag := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#1f1726")).
		Background(lipgloss.Color(shellModeColor(m))).
		Bold(true).
		Padding(0, 1).
		Render(shellModeName(m))

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
		center = strings.Join(compactTextParts([]string{modelZone}), " · ")
		prefixWidth := lipgloss.Width(modeTag) + lipgloss.Width(pipe) + lipgloss.Width(brand) + lipgloss.Width(pipe) + lipgloss.Width(titleDivider)
		titleAvail := max(width-prefixWidth-lipgloss.Width(center)-1, 4)
		session := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
			Render(truncateEnd(shellSessionLabel(m, state), titleAvail))
		left = modeTag + pipe + brand + pipe + session + titleDivider
	}

	row := renderHeaderLeftCenterRow(left, center, max(width, 1))
	return row + "\n" + renderHeaderDivider(m, max(width, 1))
}

func renderSplitWideFooter(m Model, state events.SessionState, width int) string {
	composer := renderSplitComposerPane(m, state, width)
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
	return strings.Join(lines, "\n")
}
