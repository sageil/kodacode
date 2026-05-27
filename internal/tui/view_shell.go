package tui

import (
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
	path := displaySessionPath(state.WorkspaceRoot, pickWorkspace(state.WorkspaceRoot, m.workspace))
	if strings.TrimSpace(path) == "" {
		path = "~"
	}
	branch := ""
	if git := footerGitStatus(m.footerStatus.workspace); git != nil && strings.TrimSpace(git.Branch) != "" {
		branch = strings.TrimSpace(git.Branch)
	}
	leftParts := []string{
		renderShellText(m, path, "subtext", "#9da8ca", false),
	}
	if branch != "" {
		leftParts = append(leftParts, renderShellText(m, branch, "success", "#90e5b4", false))
	}
	leftParts = append(leftParts,
		renderShellText(m, "koda", "primary", "#e6b450", true),
		renderShellText(m, footerAgentLabel(m, state, effectiveFooterTurnID(m, state)), "secondary", "#39bae6", false),
	)
	if modelZone := headerModelZone(m, state, max(width/3, 8)); modelZone != "" {
		leftParts = append(leftParts, modelZone)
	}
	left := strings.Join(compactTextParts(leftParts), " ")
	right := headerContextMetricsZone(m, state)
	row := lipgloss.NewStyle().Width(width).Render(joinBar(truncateEnd(left, max(width-lipgloss.Width(right)-1, 1)), right, width))
	return row + "\n" + renderHeaderDivider(m, width)
}

func renderKodaShellFooter(m Model, state events.SessionState, width int, transcriptStatus string) string {
	width = max(width, 1)
	parts := make([]string, 0, 5)
	if notice := renderFooterNoticeBlock(m, state, width); strings.TrimSpace(notice) != "" {
		parts = append(parts, notice)
	}
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
