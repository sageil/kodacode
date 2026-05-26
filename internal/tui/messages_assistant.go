package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
)

func renderAssistantTranscriptSection(m Model, turn *events.TurnState, text string, width int) string {
	return renderAssistantTranscriptSectionWithStreamKey(m, turn, text, width, "")
}

func renderAssistantTranscriptSectionWithStreamKey(m Model, turn *events.TurnState, text string, width int, streamKey string) string {
	if isLocalShellTurn(turn) {
		return ""
	}
	if turn == nil {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	body := strings.TrimRight(text, "\n")
	return renderAssistantTranscriptCardWithStreamKey(m, body, max(width, 1), streamKey)
}

func renderAssistantPreviewTranscriptSectionWithStreamKey(m Model, turn *events.TurnState, text string, width int, streamKey string) string {
	if turn != nil && turn.Config != nil && turn.Config.HideAssistantPreview {
		return ""
	}
	return renderAssistantTranscriptSectionWithStreamKey(m, turn, text, width, streamKey)
}

func renderReasoningTranscriptSection(m Model, text string, width int, dimmed bool) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	style := transcriptBlockStyle{
		accent: colorFor(m.theme, "secondary", "#7dcfff"),
	}
	if dimmed {
		softColor := colorFor(m.theme, "soft", softTextColor)
		style.accent = softColor
		style.textColor = softColor
	}
	return renderTranscriptBlock(m, "Thinking", strings.TrimRight(text, "\n"), width, style)
}

func isLocalShellTurn(turn *events.TurnState) bool {
	if turn == nil || turn.Config != nil {
		return false
	}
	call := turn.ToolCalls["call-local-shell"]
	return isBashToolCall(call)
}

func renderAssistantTranscriptCard(m Model, body string, width int) string {
	return renderAssistantTranscriptCardWithStreamKey(m, body, width, "")
}

func renderAssistantTranscriptCardWithStreamKey(m Model, body string, width int, streamKey string) string {
	cardBG := toneValue(m.theme, tonePanelAlt)
	return cachedTranscriptRender("assistant_card", m, width, func() string {
		blockWidth := max(width, 1)
		hPadding := 2
		contentWidth := max(blockWidth-hPadding*2, 1)
		contentLines := renderAssistantContentLinesWithStreamKey(m, body, contentWidth, "", streamKey)
		for i, line := range contentLines {
			contentLines[i] = fillBackground(contentWidth, cardBG, line)
		}
		content := strings.Join(contentLines, "\n")
		return lipgloss.NewStyle().
			Width(blockWidth).
			Padding(1, hPadding).
			Background(lipgloss.Color(cardBG)).
			Render(persistBackgroundANSI(content, cardBG))
	}, strings.TrimSpace(cardBG), body)
}

func renderCompactionSummaryTranscriptCard(m Model, body string, width int) string {
	cardBG := toneValue(m.theme, tonePanel)
	accent := colorFor(m.theme, "warning", "#ffd28f")
	return cachedTranscriptRender("compaction_card", m, width, func() string {
		blockWidth := max(width, 1)
		hPadding := 2
		contentWidth := max(blockWidth-hPadding*2, 1)

		contentLines := []string{
			fillBackground(contentWidth, cardBG, renderTranscriptRuleTitleLine(historyCompactionCardTitle, contentWidth, accent, cardBG)),
			fillBackground(contentWidth, cardBG, ""),
		}
		for _, line := range renderMarkdownBlockOnSurface(m, body, contentWidth, cardBG) {
			contentLines = append(contentLines, fillBackground(contentWidth, cardBG, line))
		}

		content := strings.Join(contentLines, "\n")
		return lipgloss.NewStyle().
			Width(blockWidth).
			Padding(1, hPadding).
			Background(lipgloss.Color(cardBG)).
			Render(persistBackgroundANSI(content, cardBG))
	}, strings.TrimSpace(cardBG), accent, body)
}

func renderTranscriptRuleTitleLine(title string, width int, accent, bg string) string {
	width = max(width, 1)
	title = strings.TrimSpace(title)

	ruleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accent))
	titleStyle := ruleStyle.Bold(true)
	if strings.TrimSpace(bg) != "" {
		bgColor := lipgloss.Color(bg)
		ruleStyle = ruleStyle.Background(bgColor)
		titleStyle = titleStyle.Background(bgColor)
	}

	if title == "" {
		return ruleStyle.Render(strings.Repeat("─", width))
	}
	if width <= ansi.StringWidth(title) {
		return truncateEnd(titleStyle.Render(title), width)
	}

	separatorWidth := width - ansi.StringWidth(title) - 2
	if separatorWidth <= 0 {
		return truncateEnd(titleStyle.Render(title), width)
	}
	leftWidth := separatorWidth / 2
	rightWidth := separatorWidth - leftWidth

	left := ""
	right := ""
	if leftWidth > 0 {
		left = ruleStyle.Render(strings.Repeat("─", leftWidth)) + " "
	}
	if rightWidth > 0 {
		right = " " + ruleStyle.Render(strings.Repeat("─", rightWidth))
	}
	return left + titleStyle.Render(title) + right
}

func assistantTranscriptSelectionLines(m Model, body string, width int) []transcriptSelectionLine {
	return assistantTranscriptSelectionLinesWithStreamKey(m, body, width, "")
}

func assistantTranscriptSelectionLinesWithStreamKey(m Model, body string, width int, streamKey string) []transcriptSelectionLine {
	blockWidth := max(width, 1)
	hPadding := 2
	contentWidth := max(blockWidth-hPadding*2, 1)
	contentLines := renderAssistantContentLinesWithStreamKey(m, body, contentWidth, "", streamKey)
	lines := make([]transcriptSelectionLine, 0, len(contentLines)+2)
	lines = append(lines, transcriptSelectionLine{})
	for _, line := range contentLines {
		lines = append(lines, newTranscriptSelectionLine(ansi.Strip(line), hPadding))
	}
	lines = append(lines, transcriptSelectionLine{})
	return lines
}
