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
	return m.renderCache.splitWideView.renderedFor(splitWideViewCacheKeyForLayout(m, state, layout), func() string {
		return renderSplitWideViewUncached(m, state, layout)
	})
}

func renderSplitWideViewUncached(m Model, state events.SessionState, layout shellLayout) string {
	header := renderSplitWideHeader(m, state, layout.totalWidth)
	footer := renderSplitWideFooter(m, state, layout.totalWidth)
	body := renderSplitWideBody(m, state, layout)
	return renderPresizedToneBlock(m.theme, toneBG, max(m.width, 1), max(m.height, 1), header+"\n"+body+"\n"+footer)
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
	headerHeight := splitWideHeaderHeight()
	footerHeight := splitWideFooterHeight(m, state, layout.totalWidth)
	bodyHeight := max(m.height-headerHeight-footerHeight, 1)
	layout.contentHeight = bodyHeight
	layout.mainHeight = bodyHeight
	layout.transcriptHeight = bodyHeight
	layout.inspectorHeight = bodyHeight
	return layout
}

func splitWideHeaderHeight() int {
	return 2
}

func splitWideFooterHeight(m Model, state events.SessionState, width int) int {
	height := splitWideComposerHeight(m, state)
	if splitWideFooterStatusVisible(m, state) {
		height++
	}
	// renderFooterHintsLine always returns one line, either collapsed or expanded.
	height++
	return max(height, 1)
}

func splitWideComposerHeight(m Model, state events.SessionState) int {
	height := splitWideComposerBodyHeight(m, state)
	if activity, ok := composerActivityStripStateFor(m, state); ok && strings.TrimSpace(activity.Label) != "" {
		height++
	}
	return max(height, 1)
}

func splitWideComposerBodyHeight(m Model, state events.SessionState) int {
	disabledMessage := strings.TrimSpace(m.composerDisabledMessage(state))
	disabled := disabledMessage != "" && !m.hasPendingInteraction()
	switch {
	case m.hasPendingInteraction():
		return max(lipgloss.Height(composerBlockedMessage(m)), 1)
	case disabled:
		return max(lipgloss.Height(disabledMessage), 1)
	default:
		content := strings.TrimRight(m.composer.View(), "\n")
		return max(lipgloss.Height(content), splitComposerMinHeight())
	}
}

func splitWideFooterStatusVisible(m Model, state events.SessionState) bool {
	metricsState, metricsTurnID, _ := effectiveStatusMetricsScope(m, state)
	return len(footerStatusSegments(m, state)) > 0 || footerStatusMeta(metricsState, metricsTurnID) != ""
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
