package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
)

func renderUserSection(m Model, width int, body string) string {
	return renderUserPromptBlock(m, width, body)
}

func renderUserPromptBlock(m Model, width int, body string) string {
	return renderTranscriptRailBlock(
		"user_prompt",
		m,
		width,
		body,
		colorFor(m.theme, "primary", "#7cc7ff"),
		colorFor(m.theme, "text", "#ecf0ff"),
	)
}

var (
	userPromptContentPrefixValue         = userPromptRailGlyph + userPromptInnerPadding
	userPromptContentPrefixGraphemeCount = transcriptGraphemeCount(userPromptContentPrefixValue)
	asciiUserPromptContentPrefixValue    = asciiUserPromptRailGlyph + userPromptInnerPadding
)

func transcriptRailSelectionLines(m Model, body string, width int) []transcriptSelectionLine {
	bodyLines := wrapTranscriptText(body, max(width-ansi.StringWidth(m.userPromptContentPrefix()), 1))
	lines := make([]transcriptSelectionLine, 0, len(bodyLines))
	for _, line := range bodyLines {
		lines = append(lines, newTranscriptSelectionLine(line, m.userPromptContentPrefixGraphemeCount()))
	}
	return lines
}

func userPromptContentPrefix() string {
	return userPromptContentPrefixValue
}

func (m Model) userPromptRailGlyph() string {
	return m.terminalIcon(terminalIconPromptRail)
}

func (m Model) userPromptContentPrefix() string {
	return m.userPromptRailGlyph() + userPromptInnerPadding
}

func (m Model) userPromptContentPrefixGraphemeCount() int {
	return transcriptGraphemeCount(m.userPromptContentPrefix())
}

func renderTranscriptRailBlock(kind string, m Model, width int, body, accent, textColor string) string {
	return cachedTranscriptRender(kind, m, width, func() string {
		prefix := m.userPromptContentPrefix()
		bodyLines := wrapTranscriptText(body, max(width-ansi.StringWidth(prefix), 1))
		content := make([]string, 0, len(bodyLines))
		rail := lipgloss.NewStyle().
			Foreground(lipgloss.Color(accent)).
			Render(m.userPromptRailGlyph())
		textStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(textColor))
		for _, line := range bodyLines {
			content = append(content, rail+userPromptInnerPadding+textStyle.Render(line))
		}
		return strings.Join(content, "\n")
	}, body, accent, textColor)
}

func renderSystemSection(m Model, title, body string, width int) string {
	if isWideShell(m) {
		lines := make([]string, 0, 2)
		if strings.TrimSpace(title) != "" {
			lines = append(lines, lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
				Bold(true).
				Render(title))
		}
		if strings.TrimSpace(body) != "" {
			lines = append(lines, lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
				Render(strings.Join(wrapTranscriptText(body, width), "\n")))
		}
		return strings.Join(lines, "\n")
	}
	return renderTranscriptBlock(m, title, body, width, transcriptBlockStyle{
		accent: colorFor(m.theme, "warning", "#ffd28f"),
	})
}

func shouldRenderDelegationRowInTranscript(handoff *events.AgentHandoffState, selectedID string) bool {
	if handoff == nil {
		return false
	}
	if strings.TrimSpace(handoff.HandoffID) != "" && strings.TrimSpace(handoff.HandoffID) == strings.TrimSpace(selectedID) {
		return true
	}
	if handoff.PreviewActive {
		return true
	}
	switch handoff.Status {
	case "":
		return true
	case events.AgentResultStatusPendingPermission, events.AgentResultStatusPendingQuestion, events.AgentResultStatusFailed:
		return true
	default:
		return false
	}
}

func renderDelegationRow(m Model, handoff *events.AgentHandoffState, width int, selected bool) string {
	prefix := "•"
	if selected {
		prefix = "›"
	}
	status := lipgloss.NewStyle().
		Foreground(handoffDisplayStatusColor(m, handoff)).
		Render("[" + handoffDisplayStatusLabel(handoff) + "]")
	head := prefix + " " + lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
		Render(handoff.ChildAgentID) + " " + status
	lines := []string{head}

	for _, line := range wrapTranscriptText(handoff.Task, max(width-2, 1)) {
		lines = append(lines, "  "+lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render(line))
	}

	switch {
	case handoff.PreviewActive:
		if activity := renderHandoffPreviewActivityLine(m, handoff); activity != "" {
			lines = append(lines, activity)
		}
		if strings.TrimSpace(handoff.PreviewAssistantText) != "" {
			lines = append(lines, "  "+lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
				Render("preview: "+summarizeInlineValue(handoff.PreviewAssistantText)))
		}
	case handoff.Status == events.AgentResultStatusPendingPermission:
		target := handoff.PermissionPath
		if handoff.PermissionKind == events.PermissionRequestKindExecution {
			target = handoff.PermissionDir
		}
		lines = append(lines, "  "+lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "warning", "#ffd28f"))).
			Render("permission: "+handoff.PermissionToolName+" → "+target))
	case handoff.Status == events.AgentResultStatusPendingQuestion:
		lines = append(lines, "  "+lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "warning", "#ffd28f"))).
			Render("question: "+handoff.QuestionToolName+" → "+summarizeInlineValue(handoff.QuestionText)))
	case handoff.Reused:
		lines = append(lines, "  "+lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "success", "#90e5b4"))).
			Render("reused in parent context"))
	case strings.TrimSpace(handoff.Error) != "":
		lines = append(lines, "  "+lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "error", "#ff9aa6"))).
			Render(summarizeInlineValue(handoff.Error)))
	case strings.TrimSpace(handoff.AssistantText) != "":
		lines = append(lines, "  "+lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render(summarizeInlineValue(handoff.AssistantText)))
	}
	return strings.Join(lines, "\n")
}

func renderHandoffPreviewActivityLine(m Model, handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if strings.TrimSpace(handoff.PreviewAction) != "" {
		parts = append(parts, handoff.PreviewAction)
	}
	if strings.TrimSpace(handoff.PreviewToolName) != "" {
		parts = append(parts, "tool: "+handoff.PreviewToolName)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#8be9fd"))).
		Render("activity: "+strings.Join(parts, " · "))
}

type transcriptBlockStyle struct {
	alignRight    bool
	accent        string
	textColor     string
	errorSections bool
}

func renderTranscriptBlock(m Model, title, body string, width int, style transcriptBlockStyle) string {
	alignKey := "0"
	if style.alignRight {
		alignKey = "1"
	}
	errorSectionsKey := "0"
	if style.errorSections {
		errorSectionsKey = "1"
	}
	return cachedTranscriptRender("transcript_block", m, width, func() string {
		blockWidth := max(width, 1)
		if style.alignRight {
			blockWidth = min(max(width*82/100, 36), width)
		}
		bodyLines := wrapTranscriptText(body, blockWidth)
		lines := make([]string, 0, len(bodyLines)+1)
		if strings.TrimSpace(title) != "" {
			accent := lineTone(m)
			if style.accent != "" {
				accent = style.accent
			}
			header := lipgloss.NewStyle().
				Foreground(lipgloss.Color(accent)).
				Bold(true).
				Render(truncateEnd(strings.ToUpper(title), blockWidth))
			lines = append(lines, header)
		}
		bodyColor := colorFor(m.theme, "text", "#ecf0ff")
		if style.textColor != "" {
			bodyColor = style.textColor
		}
		inErrorSection := false
		for _, line := range bodyLines {
			lineColor := bodyColor
			if style.errorSections {
				trimmed := strings.TrimSpace(ansi.Strip(line))
				switch trimmed {
				case "Error:":
					inErrorSection = true
				case "":
					inErrorSection = false
				}
				if inErrorSection {
					lineColor = colorFor(m.theme, "error", "#ff9aa6")
				}
			}
			lines = append(lines, lipgloss.NewStyle().
				Foreground(lipgloss.Color(lineColor)).
				Render(line))
		}
		if !style.alignRight {
			return strings.Join(lines, "\n")
		}
		for i, line := range lines {
			lines[i] = rightAlignTranscriptLine(line, width)
		}
		return strings.Join(lines, "\n")
	}, title, body, style.accent, style.textColor, alignKey, errorSectionsKey)
}

func renderTurnSeparator(m Model, width int) string {
	if isWideShell(m) {
		sepWidth := max(width/4, 8)
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(lineTone(m))).
			Render(strings.Repeat("─", sepWidth))
	}
	sepWidth := max(width/3, 4)
	pad := max((width-sepWidth)/2, 0)
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m))).
		Render(strings.Repeat("─", sepWidth))
	return strings.Repeat(" ", pad) + sep
}

func wrapTranscriptText(text string, width int) []string {
	lines := make([]string, 0, 8)
	for _, paragraph := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if strings.TrimSpace(paragraph) == "" {
			lines = append(lines, "")
			continue
		}
		wrapped := ansi.Wrap(paragraph, width, "")
		lines = append(lines, strings.Split(strings.TrimRight(wrapped, "\n"), "\n")...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func rightAlignTranscriptLine(line string, width int) string {
	pad := max(width-ansi.StringWidth(ansi.Strip(line)), 0)
	return strings.Repeat(" ", pad) + line
}
