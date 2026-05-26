package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func renderMarkdownQuoteLinesOnSurface(m Model, text string, width int, bg string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{""}
	}

	railStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	if strings.TrimSpace(bg) != "" {
		railStyle = railStyle.Background(lipgloss.Color(bg))
	}
	rail := railStyle.Render("│")

	contentWidth := max(width-2, 1)
	quoted := renderInlineMarkdownOnSurface(m, text, bg)
	wrapped := splitWrappedStyledLines(quoted, contentWidth)
	lines := make([]string, 0, len(wrapped))
	for _, line := range wrapped {
		lines = append(lines, rail+" "+line)
	}
	return lines
}

func renderInlineMarkdownOnSurface(m Model, text string, bg string) string {
	textColor := colorFor(m.theme, "text", "#ecf0ff")
	codeColor := colorFor(m.theme, "secondary", "#7dcfff")
	hasBG := strings.TrimSpace(bg) != ""

	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(textColor))
	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(codeColor))
	if hasBG {
		background := lipgloss.Color(bg)
		textStyle = textStyle.Background(background)
		codeStyle = codeStyle.Background(background)
	}
	boldStyle := textStyle.Bold(true)

	var buf strings.Builder
	i := 0
	n := len(text)

	for i < n {
		if text[i] == '`' && i+1 < n {
			if end := strings.IndexByte(text[i+1:], '`'); end >= 0 {
				buf.WriteString(codeStyle.Render(text[i+1 : i+1+end]))
				i = i + 1 + end + 1
				continue
			}
		}

		if i+1 < n && text[i] == '*' && text[i+1] == '*' {
			if end := strings.Index(text[i+2:], "**"); end >= 0 {
				buf.WriteString(boldStyle.Render(text[i+2 : i+2+end]))
				i = i + 2 + end + 2
				continue
			}
		}

		start := i
		i++
		for i < n {
			if text[i] == '`' {
				break
			}
			if i+1 < n && text[i] == '*' && text[i+1] == '*' {
				break
			}
			i++
		}
		buf.WriteString(textStyle.Render(text[start:i]))
	}

	return buf.String()
}

func renderLiteralLineOnSurface(m Model, text string, bg string) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff")))
	if strings.TrimSpace(bg) != "" {
		style = style.Background(lipgloss.Color(bg))
	}
	return style.Render(text)
}

func parseMarkdownCodeFence(line string) (int, string, bool) {
	if !strings.HasPrefix(line, "```") {
		return 0, "", false
	}
	fenceLen := 0
	for fenceLen < len(line) && line[fenceLen] == '`' {
		fenceLen++
	}
	if fenceLen < 3 {
		return 0, "", false
	}
	info := strings.TrimSpace(line[fenceLen:])
	if info == "" {
		return fenceLen, "", true
	}
	language := strings.Fields(info)[0]
	return fenceLen, strings.TrimSpace(language), true
}

func isMarkdownCodeFenceClose(line string, openLen int) bool {
	if openLen < 3 {
		return false
	}
	if !strings.HasPrefix(line, "```") {
		return false
	}
	fenceLen := 0
	for fenceLen < len(line) && line[fenceLen] == '`' {
		fenceLen++
	}
	return fenceLen >= openLen && strings.TrimSpace(line[fenceLen:]) == ""
}

func parseMarkdownHeading(line string) (int, string) {
	level := 0
	for _, c := range line {
		if c == '#' {
			level++
		} else {
			break
		}
	}
	if level == 0 || level > 3 || level >= len(line) || line[level] != ' ' {
		return 0, ""
	}
	rest := strings.TrimSpace(line[level+1:])
	if rest == "" {
		return 0, ""
	}
	return level, rest
}

func parseMarkdownBlockQuote(line string) (string, bool) {
	if strings.HasPrefix(line, "> ") && len(line) > 2 {
		return strings.TrimSpace(line[2:]), true
	}
	if line == ">" {
		return "", true
	}
	return "", false
}

func parseNumberedListItem(line string) (string, string) {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(line) {
		return "", ""
	}
	if line[i] == '.' && line[i+1] == ' ' {
		return line[:i+1], line[i+2:]
	}
	return "", ""
}

func isMarkdownThematicBreak(line string) bool {
	compacted := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if len(compacted) < 3 {
		return false
	}
	switch compacted[0] {
	case '-', '_', '*':
	default:
		return false
	}
	for i := 1; i < len(compacted); i++ {
		if compacted[i] != compacted[0] {
			return false
		}
	}
	return true
}

func isMarkdownTableRow(line string) bool {
	return strings.Count(line, "|") >= 2
}

func isMarkdownTableSeparator(line string) bool {
	if strings.Count(line, "|") < 2 {
		return false
	}
	trimmed := strings.ReplaceAll(line, "|", "")
	trimmed = strings.ReplaceAll(trimmed, ":", "")
	trimmed = strings.ReplaceAll(trimmed, "-", "")
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	return trimmed == ""
}
