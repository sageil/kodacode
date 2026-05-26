package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func renderCodeBlockLinesOnSurface(m Model, lines []string, language string, width int, bg string) []string {
	border := renderCodeBlockSurfaceBorder(m, bg)
	contentWidth := max(width-2, 1)
	if wrapLanguage, ok := codeBlockWrapLanguage(language); ok {
		return renderWrappedCodeBlockLinesOnSurface(m, lines, wrapLanguage, border, contentWidth, bg)
	}
	if len(lines) == 0 {
		return []string{renderCodeBlockLineWithBorder(border, "", contentWidth)}
	}
	if sections, ok := splitMultiFileCodeSections(lines); ok {
		return renderMultiFileCodeBlockLinesOnSurface(m, sections, border, contentWidth, bg)
	}

	lexer := syntaxHighlightLexer(language, strings.Join(lines, "\n"))
	style := syntaxHighlightStyle(m.theme)
	output := make([]string, 0, len(lines))
	for _, line := range lines {
		output = append(output, renderCodeBlockLineWithBorder(
			border,
			syntaxHighlightCodeLine(line, lexer, style, colorFor(m.theme, "soft", softTextColor), bg),
			contentWidth,
		))
	}
	return output
}

func codeBlockWrapLanguage(language string) (string, bool) {
	language = strings.TrimSpace(strings.ToLower(language))
	switch language {
	case "text-wrap":
		return "", true
	default:
		wrapSuffix := "-wrap"
		if strings.HasSuffix(language, wrapSuffix) {
			return strings.TrimSuffix(language, wrapSuffix), true
		}
		return "", false
	}
}

func renderWrappedCodeBlockLinesOnSurface(m Model, lines []string, language string, border string, contentWidth int, bg string) []string {
	if len(lines) == 0 {
		return []string{renderCodeBlockLineWithBorder(border, "", contentWidth)}
	}
	var lexer = syntaxHighlightLexer(language, strings.Join(lines, "\n"))
	var style = syntaxHighlightStyle(m.theme)
	defaultFG := colorFor(m.theme, "soft", softTextColor)
	output := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			output = append(output, renderCodeBlockLineWithBorder(border, "", contentWidth))
			continue
		}
		wrapped := strings.Split(strings.TrimRight(ansi.Wrap(line, contentWidth, ""), "\n"), "\n")
		for _, segment := range wrapped {
			if strings.TrimSpace(language) != "" {
				segment = syntaxHighlightCodeLine(segment, lexer, style, defaultFG, bg)
			}
			output = append(output, renderCodeBlockLineWithBorder(border, segment, contentWidth))
		}
	}
	if len(output) == 0 {
		return []string{renderCodeBlockLineWithBorder(border, "", contentWidth)}
	}
	return output
}

func renderMultiFileCodeBlockLinesOnSurface(m Model, sections []codeBlockSection, border string, contentWidth int, bg string) []string {
	output := make([]string, 0, len(sections)*4)
	style := syntaxHighlightStyle(m.theme)
	defaultFG := colorFor(m.theme, "soft", softTextColor)
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff")))
	noticeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))

	for _, section := range sections {
		if strings.TrimSpace(section.header) != "" {
			output = append(output, renderCodeBlockLineWithBorder(border, headerStyle.Render(section.header), contentWidth))
		}

		language := toolDetailLanguageForPath(section.path)
		lexer := syntaxHighlightLexer(language, strings.Join(section.lines, "\n"))
		for _, line := range section.lines {
			content := ""
			if matches := readToolResultFooterPattern.FindStringSubmatch(strings.TrimSpace(line)); len(matches) == 4 {
				content = noticeStyle.Render(line)
			} else {
				content = syntaxHighlightCodeLine(line, lexer, style, defaultFG, bg)
			}
			output = append(output, renderCodeBlockLineWithBorder(border, content, contentWidth))
		}
	}

	if len(output) == 0 {
		return []string{renderCodeBlockLineWithBorder(border, "", contentWidth)}
	}
	return output
}

func renderCodeBlockSurfaceBorder(m Model, bg string) string {
	borderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m)))
	if strings.TrimSpace(bg) != "" {
		borderStyle = borderStyle.Background(lipgloss.Color(bg))
	}
	return borderStyle.Render("│")
}

func renderCodeBlockLineWithBorder(border string, line string, contentWidth int) string {
	if ansi.StringWidth(line) > contentWidth {
		line = ansi.Truncate(line, contentWidth, "…")
	}
	return border + " " + line
}
