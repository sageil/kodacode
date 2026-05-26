package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func renderComposerPopup(m Model, width int) string {
	if m.composerState.popupMode == composerPopupNone {
		return ""
	}

	title, hint := composerPopupHeader(m)
	items := m.composerPopupItems()
	width = max(width, 1)
	boxStyle := toneFillStyle(m.theme, tonePanel).
		Width(width).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(lineTone(m))).
		Padding(0, 1)
	contentWidth := max(width-boxStyle.GetHorizontalFrameSize(), 1)

	rows := make([]string, 0, composerPopupMaxVisible+2)
	rows = append(rows, lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
		Bold(true).
		Render(title))
	if hint != "" {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render(hint))
	}

	switch {
	case m.composerState.popupMode == composerPopupHistory && m.composerState.promptHistoryBusy:
		rows = append(rows, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render("Loading recent prompts…"))
	case m.composerState.popupMode == composerPopupSkills && m.composerState.skillsBusy:
		rows = append(rows, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render("Loading skills…"))
	case m.composerState.popupMode == composerPopupPaths && m.composerState.workspacePathsBusy:
		rows = append(rows, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render("Loading workspace paths…"))
	case len(items) == 0:
		empty := "No matches."
		switch m.composerState.popupMode {
		case composerPopupHistory:
			empty = "No saved prompts."
		case composerPopupSkills:
			empty = "No skills."
		case composerPopupPaths:
			empty = "No paths."
		}
		rows = append(rows, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render(empty))
	default:
		start, end := composerPopupWindow(len(items), m.composerState.popupCursor)
		for idx := start; idx < end; idx++ {
			rows = append(rows, renderComposerPopupRow(m, items[idx], idx == m.composerState.popupCursor, contentWidth))
		}
		if start > 0 {
			rows = append([]string{lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
				Render("↑ more")}, rows...)
		}
		if end < len(items) {
			rows = append(rows, lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
				Render("↓ more"))
		}
	}

	body := strings.Join(rows, "\n")
	return boxStyle.Render(renderToneBlock(m.theme, tonePanel, contentWidth, lipgloss.Height(body), body))
}

func composerPopupHeader(m Model) (string, string) {
	switch m.composerState.popupMode {
	case composerPopupSlash:
		return "Commands", "↑/↓ choose · enter run · esc close"
	case composerPopupHistory:
		return "Recent Prompts", "↑/↓ choose · enter fill composer · esc close"
	case composerPopupSkills:
		return "Skills", "↑/↓ choose · enter insert · esc close"
	case composerPopupPaths:
		return "Files", "↑/↓ choose · enter include · esc close"
	default:
		return "", ""
	}
}

func composerPopupWindow(total, cursor int) (int, int) {
	if total <= composerPopupMaxVisible {
		return 0, total
	}
	start := max(cursor-composerPopupMaxVisible+1, 0)
	end := min(start+composerPopupMaxVisible, total)
	if end-start < composerPopupMaxVisible {
		start = max(end-composerPopupMaxVisible, 0)
	}
	return start, end
}

func renderComposerPopupRow(m Model, item composerPopupItem, selected bool, width int) string {
	prefix := "  "
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff")))
	metaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	if selected {
		prefix = "❯ "
		titleStyle = titleStyle.Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).Bold(true)
	}
	meta := strings.TrimSpace(item.Meta)
	titleWidth := max(width-ansiWidth(prefix), 8)
	row := prefix + titleStyle.Render(truncateEnd(item.Title, titleWidth))
	if meta != "" {
		meta = truncateEnd(meta, max(width/3, 14))
		titleWidth = max(width-ansiWidth(prefix)-ansiWidth(meta)-1, 8)
		row = joinBar(
			prefix+titleStyle.Render(truncateEnd(item.Title, titleWidth)),
			metaStyle.Render(meta),
			width,
		)
	}
	return row
}
