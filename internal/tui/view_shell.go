package tui

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderKodaShellView(m Model, state events.SessionState, layout shellLayout) string {
	width := max(layout.totalWidth, 1)
	header := renderKodaShellHeader(m, state, width)
	body := renderKodaShellTranscriptBody(m, state, width)
	status := renderKodaShellTranscriptStatus(m, state, width)
	footer := renderKodaShellFooter(m, state, width, status)
	return renderPresizedToneBlock(m.theme, toneBG, max(m.width, 1), max(m.height, 1), header+"\n"+body+"\n"+footer)
}

func renderKodaShellTranscriptBody(m Model, state events.SessionState, width int) string {
	width = max(width, 1)
	body := renderTranscriptViewportWithOptions(m, width, transcriptPaneRenderOptions{showScrollbar: false})
	if panel := renderInlinePermissionPrompt(m, state, width); panel != "" {
		body = panel + "\n\n" + body
	}
	if panel := renderInlineQuestionPrompt(m, width); panel != "" {
		body = panel + "\n\n" + body
	}
	return body
}

func renderKodaShellTranscriptStatus(m Model, state events.SessionState, width int) string {
	return strings.TrimSpace(renderTranscriptStatusBar(m, state, max(width, 1)))
}

func renderKodaShellHeader(m Model, state events.SessionState, width int) string {
	width = max(width, 1)
	baseParts := []string{
		renderShellText(m, "KodaCode", "primary", "#e6b450", true),
		renderShellText(m, shellWorkspaceHandle(state.WorkspaceRoot, m.workspace), "subtext", "#9da8ca", false),
	}
	if branch := shellHeaderBranchLabel(m, state); branch != "" {
		baseParts = append(baseParts, renderShellText(m, "on", "subtext", "#9da8ca", false))
		baseParts = append(baseParts, renderShellText(m, branch, "success", "#90e5b4", false))
	}

	modelZone := headerModelZone(m, state, max(width/3, 8))
	metricsZone := headerContextMetricsZone(m, state)
	content := renderKodaShellHeaderContent(m, state, baseParts, modelZone, metricsZone, width)
	row := renderCenteredShellHeaderRow(content, width)
	return row + "\n" + renderHeaderDividerForState(m, state, width)
}

func renderKodaShellHeaderContent(m Model, state events.SessionState, baseParts []string, modelZone, metricsZone string, width int) string {
	modelMetrics := headerCenterZone(modelZone, " "+renderShellSeparator(m)+" ", metricsZone)
	title := shellSessionLabel(m, state)
	titleWidth := len([]rune(title))
	for titleWidth > 0 {
		candidate := buildKodaShellHeaderContent(m, baseParts, truncateEnd(title, titleWidth), modelMetrics)
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
		titleWidth--
	}
	if candidate := buildKodaShellHeaderContent(m, baseParts, "", modelMetrics); lipgloss.Width(candidate) <= width {
		return candidate
	}
	if strings.TrimSpace(metricsZone) != "" && strings.TrimSpace(modelZone) != "" {
		for modelWidth := lipgloss.Width(modelZone) - 1; modelWidth > 0; modelWidth-- {
			shrunkModelZone := headerModelZone(m, state, modelWidth)
			candidate := buildKodaShellHeaderContent(m, baseParts, "", headerCenterZone(shrunkModelZone, " "+renderShellSeparator(m)+" ", metricsZone))
			if lipgloss.Width(candidate) <= width {
				return candidate
			}
		}
	}
	return truncateVisibleEnd(buildKodaShellHeaderContent(m, baseParts, "", modelMetrics), width)
}

func buildKodaShellHeaderContent(m Model, baseParts []string, title, modelMetrics string) string {
	parts := append([]string(nil), baseParts...)
	if strings.TrimSpace(title) != "" {
		parts = append(parts, renderShellSeparator(m), renderShellText(m, title, "text", "#f8f8f2", false))
	}
	if strings.TrimSpace(modelMetrics) != "" {
		parts = append(parts, renderShellSeparator(m), modelMetrics)
	}
	return strings.Join(compactTextParts(parts), " ")
}

func renderCenteredShellHeaderRow(content string, width int) string {
	width = max(width, 1)
	contentWidth := lipgloss.Width(content)
	if contentWidth >= width {
		return lipgloss.NewStyle().Width(width).Render(content)
	}
	left := centeredZoneStart(width, contentWidth)
	return strings.Repeat(" ", left) + content + strings.Repeat(" ", max(width-left-contentWidth, 0))
}

func shellWorkspaceHandle(workspaceRoot, fallback string) string {
	name := strings.TrimSpace(shellWorkspaceName(workspaceRoot, fallback))
	if name == "" || name == "~" {
		return "@workspace"
	}
	return "@" + strings.ToLower(name)
}

func shellWorkspaceName(workspaceRoot, fallback string) string {
	workspace := strings.TrimSpace(pickWorkspace(workspaceRoot, fallback))
	if workspace == "" {
		return "~"
	}
	base := filepath.Base(filepath.Clean(workspace))
	switch base {
	case "", ".", string(filepath.Separator):
		return workspace
	default:
		return base
	}
}

func shellHeaderBranchLabel(m Model, _ events.SessionState) string {
	if git := footerGitStatus(m.footerStatus.workspace); git != nil {
		if branch := strings.TrimSpace(git.Branch); branch != "" {
			return m.terminalIcon(terminalIconGitBranch) + " " + branch
		}
	}
	return ""
}

func renderShellSeparator(m Model) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m))).
		Render("|")
}

func renderKodaShellFooter(m Model, state events.SessionState, width int, transcriptStatus string) string {
	width = max(width, 1)
	parts := make([]string, 0, 4)
	if divider := renderComposerActivityStrip(m, state, width, composerBorderColor(m, state)); strings.TrimSpace(divider) != "" {
		parts = append(parts, divider)
	}
	content := renderKodaShellComposer(m, state, width)
	if strings.TrimSpace(content) != "" {
		parts = append(parts, content)
	}
	if strings.TrimSpace(transcriptStatus) != "" {
		parts = append(parts, transcriptStatus)
	}
	parts = append(parts, renderKodaShellStatusLine(m, state, width))
	return strings.Join(parts, "\n")
}

func renderKodaShellComposer(m Model, state events.SessionState, width int) string {
	disabledMessage := strings.TrimSpace(m.composerDisabledMessage(state))
	disabled := disabledMessage != "" && !m.hasPendingInteraction()
	switch {
	case m.hasPendingInteraction():
		return renderShellText(m, composerBlockedMessage(m), "warning", "#ffd28f", false)
	case disabled:
		return renderShellText(m, truncateEnd(disabledMessage, width), "warning", "#ffd28f", false)
	default:
		return lipgloss.NewStyle().Width(width).Render(strings.TrimRight(m.composer.View(), "\n"))
	}
}

func renderKodaShellStatusLine(m Model, state events.SessionState, width int) string {
	width = max(width, 1)
	status := renderStatus(m, state)
	if status == "" {
		status = "idle"
	}
	left := renderShellText(m, status, shellStatusTone(status), "#9da8ca", true)
	for _, segment := range footerStatusSegments(m, state) {
		text := strings.TrimSpace(segment.Text)
		if text == "" || text == status {
			continue
		}
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(segment.Color))
		if segment.Bold {
			style = style.Bold(true)
		}
		left += renderShellText(m, " · ", "subtext", "#9da8ca", false) + style.Render(text)
	}
	hints := shellStatusHints(m, state)
	right := renderShellText(m, truncateEnd(hints, max(width/2, 8)), "subtext", "#9da8ca", false)
	return lipgloss.NewStyle().Width(width).Render(joinBar(truncateEnd(left, max(width-lipgloss.Width(right)-1, 1)), right, width))
}

func shellStatusTone(status string) string {
	switch strings.TrimSpace(status) {
	case "running", "starting", "resuming":
		return "warning"
	case "completed":
		return "success"
	case "failed", "stalled", "budget exceeded":
		return "error"
	default:
		return "primary"
	}
}

func renderShellText(m Model, text, token, fallback string, bold bool) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(m.theme, token, fallback)))
	if bold {
		style = style.Bold(true)
	}
	return style.Render(text)
}

func kodaShellFooterHeight(m Model, state events.SessionState, width int) int {
	return max(lipgloss.Height(renderKodaShellFooter(m, state, width, renderKodaShellTranscriptStatus(m, state, width))), 1)
}

func kodaShellHeaderHeight(m Model, state events.SessionState, width int) int {
	return max(lipgloss.Height(renderKodaShellHeader(m, state, width)), 1)
}
