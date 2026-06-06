package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
		linePrefix := rail + userPromptInnerPadding
		for _, line := range bodyLines {
			content = append(content, linePrefix+textStyle.Render(line))
		}
		return strings.Join(content, "\n")
	}, body, accent, textColor)
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
