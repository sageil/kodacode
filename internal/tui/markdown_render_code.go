package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func renderCodeBlockLinesOnSurface(m Model, lines []string, language string, width int, bg string) []string {
	border := renderCodeBlockSurfaceBorder(m, bg)
	contentWidth := max(width-4, 1)
	if wrapLanguage, ok := codeBlockWrapLanguage(language); ok {
		body := renderWrappedCodeBlockLinesOnSurface(m, lines, wrapLanguage, border, contentWidth, bg)
		return frameCodeBlockLinesOnSurface(m, body, wrapLanguage, contentWidth, bg)
	}
	var body []string
	if len(lines) == 0 {
		body = []string{renderCodeBlockLineWithBorder(border, "", contentWidth)}
	} else if sections, ok := splitMultiFileCodeSections(lines); ok {
		body = renderMultiFileCodeBlockLinesOnSurface(m, sections, border, contentWidth, bg)
	} else {
		lexer := syntaxHighlightLexer(language, strings.Join(lines, "\n"))
		style := syntaxHighlightStyle(m.theme)
		body = make([]string, 0, len(lines))
		for _, line := range lines {
			body = append(body, renderCodeBlockLineWithBorder(
				border,
				syntaxHighlightCodeLine(line, lexer, style, colorFor(m.theme, "soft", softTextColor), bg),
				contentWidth,
			))
		}
	}
	return frameCodeBlockLinesOnSurface(m, body, language, contentWidth, bg)
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
			output = append(output, renderWrappedCodeBlockLineWithBorder(border, "", contentWidth))
			continue
		}
		wrapped := strings.Split(strings.TrimRight(ansi.Wrap(line, contentWidth, ""), "\n"), "\n")
		for _, segment := range wrapped {
			if strings.TrimSpace(language) != "" {
				segment = syntaxHighlightCodeLine(segment, lexer, style, defaultFG, bg)
			}
			output = append(output, renderWrappedCodeBlockLineWithBorder(border, segment, contentWidth))
		}
	}
	if len(output) == 0 {
		return []string{renderWrappedCodeBlockLineWithBorder(border, "", contentWidth)}
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
	return renderCodeBlockSurfaceGlyph(m, bg, "│")
}

func renderCodeBlockSurfaceGlyph(m Model, bg string, glyph string) string {
	borderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m)))
	if strings.TrimSpace(bg) != "" {
		borderStyle = borderStyle.Background(lipgloss.Color(bg))
	}
	return borderStyle.Render(glyph)
}

func renderCodeBlockLineWithBorder(border string, line string, contentWidth int) string {
	if ansi.StringWidth(line) > contentWidth {
		line = ansi.Truncate(line, contentWidth, "…")
	}
	padding := max(contentWidth-ansi.StringWidth(line), 0)
	return border + " " + line + strings.Repeat(" ", padding) + " " + border
}

func renderWrappedCodeBlockLineWithBorder(border string, line string, contentWidth int) string {
	return border + " " + line
}

func frameCodeBlockLinesOnSurface(m Model, body []string, language string, contentWidth int, bg string) []string {
	if len(body) == 0 {
		body = []string{renderCodeBlockLineWithBorder(renderCodeBlockSurfaceBorder(m, bg), "", contentWidth)}
	}
	output := make([]string, 0, len(body)+2)
	output = append(output, renderCodeBlockFrameLineOnSurface(m, "top", language, contentWidth, bg))
	output = append(output, body...)
	output = append(output, renderCodeBlockFrameLineOnSurface(m, "bottom", "", contentWidth, bg))
	return output
}

func renderCodeBlockFrameLineOnSurface(m Model, position string, language string, contentWidth int, bg string) string {
	left, right := "├", "┤"
	switch position {
	case "top":
		left, right = "┌", "┐"
	case "bottom":
		left, right = "└", "┘"
	}

	innerWidth := max(contentWidth+2, 1)
	label := strings.TrimSpace(language)
	if label != "" && position == "top" {
		label = " " + label + " "
	}
	if label == "" || ansi.StringWidth(label) >= innerWidth {
		label = ""
	}

	ruleWidth := innerWidth - ansi.StringWidth(label)
	if label != "" {
		ruleWidth = max(ruleWidth, 0)
	}
	line := left + label + strings.Repeat("─", max(ruleWidth, 0)) + right
	return renderCodeBlockSurfaceGlyph(m, bg, line)
}
