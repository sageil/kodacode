package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderSplitLeftColumn(m Model, layout shellLayout, width, height int) string {
	return renderSplitTranscriptPane(m, width, height)
}

func renderSplitTranscriptPane(m Model, width, height int) string {
	state := m.projector.CurrentState()
	layout := resolveShellLayout(m, state)
	if isWideShell(m) {
		layout = normalizeWideShellLayout(m, state, layout)
	}
	borderless := splitTranscriptPaneBorderless(m, layout)
	if m.renderCache.splitTranscriptPane == nil {
		return renderSplitTranscriptPaneUncached(m, state, width, height, borderless)
	}
	return m.renderCache.splitTranscriptPane.renderedFor(splitTranscriptPaneCacheKey(m, state, width, height, borderless), func() string {
		return renderSplitTranscriptPaneUncached(m, state, width, height, borderless)
	})
}

func splitTranscriptPaneBorderless(m Model, _ shellLayout) bool {
	return isWideShell(m)
}

func renderSplitTranscriptPaneUncached(m Model, state events.SessionState, width, height int, borderless bool) string {
	contentWidth := max(width-4, 1)
	if borderless {
		contentWidth = max(width, 1)
	}
	body, activity := renderTranscriptPaneSectionsWithOptions(m, state, contentWidth, transcriptPaneRenderOptions{showScrollbar: !borderless})
	body = strings.TrimRight(body, "\n")
	transcriptTone := toneBG
	if borderless {
		body = renderSplitTranscriptContent(max(height, 1), body, activity)
		return renderToneBlock(m.theme, transcriptTone, max(width, 1), max(height, 1), body)
	}
	body = renderSplitTranscriptContent(max(height-2, 1), body, activity)
	return renderSplitPane(m, "", m.messages.ScrollSummary(), body, width, height, transcriptTone, m.chrome.focus == focusTranscript)
}

func renderSplitTranscriptContent(height int, body, activity string) string {
	height = max(height, 1)
	body = strings.TrimRight(body, "\n")
	activity = strings.TrimSpace(activity)
	if activity == "" {
		return fitSplitTranscriptBlockHeight(body, height, true)
	}
	footerHeight := lipgloss.Height(activity)
	if footerHeight >= height {
		return fitSplitTranscriptBlockHeight(activity, height, false)
	}
	bodyHeight := max(height-footerHeight, 1)
	return fitSplitTranscriptBlockHeight(body, bodyHeight, true) + "\n" + fitSplitTranscriptBlockHeight(activity, footerHeight, false)
}

func fitSplitTranscriptBlockHeight(content string, height int, alignBottom bool) string {
	height = max(height, 1)
	content = cropBlockHeight(strings.TrimRight(content, "\n"), height)
	lines := strings.Split(content, "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = lines[:0]
	}
	if len(lines) >= height {
		return strings.Join(lines, "\n")
	}
	padding := make([]string, height-len(lines))
	if alignBottom {
		lines = append(padding, lines...)
	} else {
		lines = append(lines, padding...)
	}
	return strings.Join(lines, "\n")
}

func renderSplitRightColumn(m Model, state events.SessionState, width, height int) string {
	_ = state
	body := renderSplitTabsComponent(m, width, height)
	return placeBlock(max(width, 1), max(height, 1), "", body)
}

func renderSplitPane(m Model, title, meta, body string, width, height int, surfaceTone string, focused bool) string {
	borderColor := lineTone(m)
	if focused {
		borderColor = colorFor(m.theme, "primary", "#7cc7ff")
	}
	style := toneFillStyle(m.theme, surfaceTone).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 1)

	contentWidth := max(width-style.GetHorizontalFrameSize(), 1)
	contentHeight := max(height-style.GetVerticalFrameSize(), 1)

	header := strings.TrimSpace(renderSplitPaneHeader(m, title, meta, contentWidth))
	content := ""
	if header != "" {
		content = header
	}
	if strings.TrimSpace(body) != "" {
		if content != "" {
			content += "\n"
		}
		content += body
	}
	return style.Render(renderToneBlock(m.theme, surfaceTone, contentWidth, contentHeight, content))
}

func renderSplitPaneHeader(m Model, title, meta string, width int) string {
	if strings.TrimSpace(title) == "" {
		return ""
	}
	left := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Bold(true).
		Render(strings.ToUpper(strings.TrimSpace(title)))
	if strings.TrimSpace(meta) == "" {
		return left
	}
	right := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render(truncateEnd(meta, max(width/3, 4)))
	return joinBar(left, right, max(width, 1))
}
