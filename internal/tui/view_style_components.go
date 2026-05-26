package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func renderSectionLabel(m Model, left, right string, width int, bg string) string {
	left = truncateEnd(left, max(width/2, 4))
	right = truncateEnd(right, max(width-len(left)-1, 4))
	leftStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Bold(true)
	rightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	row := joinBar(leftStyle.Render(strings.ToUpper(left)), rightStyle.Render(strings.ToUpper(right)), max(width, 1))
	if strings.TrimSpace(bg) == "" {
		return row
	}
	return fillBackground(max(width, 1), bg, row)
}

func renderKeycap(m Model, key string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
		Background(lipgloss.Color(toneValue(m.theme, tonePanelAlt))).
		Bold(true).
		Padding(0, 1).
		Render(key)
}

func renderInspectorCard(m Model, title, body string, width int, accent string) string {
	border := lineTone(m)
	if accent != "" {
		border = accent
	}
	style := lipgloss.NewStyle().
		Width(max(width, 1)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(border)).
		Padding(0, 1)
	contentWidth := max(style.GetWidth()-style.GetHorizontalFrameSize(), 1)
	bodyText := lipgloss.NewStyle().
		Width(contentWidth).
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render(body)
	content := bodyText
	if strings.TrimSpace(title) != "" {
		titleText := lipgloss.NewStyle().
			Width(contentWidth).
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Bold(true).
			Render(title)
		content = titleText + "\n" + bodyText
	}
	return style.Render(content)
}

func renderInspectorCardExternalTitle(m Model, title, body string, width int, accent string) string {
	titleText := lipgloss.NewStyle().
		Width(max(width, 1)).
		Align(lipgloss.Center).
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
		Bold(true).
		Render(title)
	return titleText + "\n\n" + renderInspectorCard(m, "", body, width, accent)
}

type inspectorParam struct {
	Label string
	Value string
	Error bool
}

func renderSessionRailCard(m Model, width int, active bool, title, stamp, meta string) string {
	border := lineTone(m)
	if active {
		border = colorFor(m.theme, "primary", "#7cc7ff")
	}
	card := lipgloss.NewStyle().
		Width(max(width, 1)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(border)).
		Padding(0, 1)
	contentWidth := max(card.GetWidth()-card.GetHorizontalFrameSize(), 1)
	stamp = truncateEnd(stamp, max(contentWidth/3, 4))
	header := joinBar(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
			Bold(true).
			Render(truncateEnd(title, max(contentWidth-len(stamp)-2, 8))),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(border)).
			Bold(true).
			Render(stamp),
		max(contentWidth, 12),
	)
	body := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render(meta)
	return card.Render(header + "\n" + body)
}

func renderPanelTag(m Model, label string, focused bool) string {
	if focused {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
			Bold(true).
			Padding(0, 1).
			Render(strings.ToUpper(label))
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "soft", softTextColor))).
		Bold(true).
		Render(strings.ToUpper(label))
}

func panelContentColor(m Model, focused bool) string {
	if focused {
		return colorFor(m.theme, "text", "#ecf0ff")
	}
	return colorFor(m.theme, "soft", softTextColor)
}
