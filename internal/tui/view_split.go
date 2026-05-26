package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderSplitWideView(m Model, state events.SessionState, layout shellLayout) string {
	layout = normalizeWideShellLayout(m, state, layout)
	if m.renderCache.splitWideView == nil {
		return renderSplitWideViewUncached(m, state, layout)
	}
	return m.renderCache.splitWideView.renderedFor(splitWideViewCacheKey(m, state, layout), func() string {
		return renderSplitWideViewUncached(m, state, layout)
	})
}

func renderSplitWideViewUncached(m Model, state events.SessionState, layout shellLayout) string {
	header := renderSplitWideHeader(m, state, layout.totalWidth)
	footer := renderSplitWideFooter(m, state, layout.totalWidth)
	body := renderSplitWideBody(m, state, layout)
	return renderToneBlock(m.theme, toneBG, max(m.width, 1), max(m.height, 1), header+"\n"+body+"\n"+footer)
}

func renderSplitWideBody(m Model, state events.SessionState, layout shellLayout) string {
	leftWidth := layout.centerWidth
	rightWidth := layout.rightWidth
	panelHeight := splitWidePanelHeight(layout)
	if !layout.showInspector {
		leftWidth = layout.totalWidth
	}

	left := renderSplitLeftColumn(m, layout, leftWidth, panelHeight)
	if !layout.showInspector {
		return left
	}

	right := renderSplitRightColumn(m, state, rightWidth, panelHeight)
	spacer := placeBlock(1, panelHeight, "", "")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, right)
}

func splitWidePanelHeight(layout shellLayout) int {
	if !layout.showInspector {
		return max(layout.contentHeight, 1)
	}
	// The split drawer stack renders one row shorter than the raw shell body.
	// Keep the transcript pane and footer layout aligned to that visible height.
	return max(layout.contentHeight-1, 1)
}

func splitComposerMinHeight() int {
	return composerMinHeight + 1
}

func normalizeWideShellLayout(m Model, state events.SessionState, layout shellLayout) shellLayout {
	if !isWideShell(m) {
		return layout
	}
	headerHeight := lipgloss.Height(renderSplitWideHeader(m, state, layout.totalWidth))
	footerHeight := lipgloss.Height(renderSplitWideFooter(m, state, layout.totalWidth))
	bodyHeight := max(m.height-headerHeight-footerHeight, 1)
	layout.contentHeight = bodyHeight
	layout.mainHeight = bodyHeight
	layout.transcriptHeight = bodyHeight
	layout.inspectorHeight = bodyHeight
	return layout
}

func compactTextParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "unknown" || part == "model unavailable" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func joinCompactTextParts(separator string, parts ...string) string {
	return strings.Join(compactTextParts(parts), separator)
}

func centeredZoneStart(width, centerWidth int) int {
	width = max(width, 1)
	centerWidth = max(centerWidth, 0)
	if centerWidth >= width {
		return 0
	}
	return (width - centerWidth) / 2
}

func renderHeaderLeftCenterRow(left, center string, width int) string {
	width = max(width, 1)
	leftWidth := lipgloss.Width(left)
	centerWidth := lipgloss.Width(center)
	if centerWidth == 0 {
		return lipgloss.NewStyle().Width(width).Render(left)
	}
	if leftWidth+1+centerWidth >= width {
		return lipgloss.NewStyle().Width(width).Render(joinBar(left, center, width))
	}
	start := max(centeredZoneStart(width, centerWidth), leftWidth+1)
	if start+centerWidth > width {
		start = max(width-centerWidth, leftWidth+1)
	}
	gapLeft := max(start-leftWidth, 0)
	if leftWidth > 0 && gapLeft == 0 {
		gapLeft = 1
	}
	gapRight := max(width-leftWidth-gapLeft-centerWidth, 0)
	return left + strings.Repeat(" ", gapLeft) + center + strings.Repeat(" ", gapRight)
}
