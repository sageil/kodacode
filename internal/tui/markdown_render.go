package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func renderMarkdownLinesOnSurface(m Model, text string, width int, bg string) []string {
	return renderMarkdownLinesOnSurfaceWithStreamKey(m, text, width, bg, "")
}

func renderMarkdownLinesOnSurfaceWithStreamKey(m Model, text string, width int, bg string, streamKey string) []string {
	return renderMarkdownLinesOnSurfaceWithStreamCache(m, text, width, bg, m.renderCache.transcriptMarkdown, streamKey)
}

func renderMarkdownLinesOnSurfaceWithStreamCache(m Model, text string, width int, bg string, streamCache *streamingMarkdownSurfaceCache, streamKey string) []string {
	raw := normalizeMarkdownSurfaceInput(strings.TrimRight(text, "\n"))
	bg = resolveMarkdownSurfaceBG(m, bg)
	return cachedMarkdownSurfaceLines("markdown_surface", m, raw, width, bg, streamCache, streamKey, func(content string) string {
		return strings.Join(renderMarkdownLinesOnSurfaceUncached(m, content, width, bg), "\n")
	})
}

func renderMarkdownLinesOnSurfaceUncached(m Model, raw string, width int, bg string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{""}
	}

	inputLines := strings.Split(raw, "\n")
	output := make([]string, 0, len(inputLines))
	inCode := false
	codeFenceLen := 0
	codeLanguage := ""
	codeLines := make([]string, 0, 8)

	flushCodeBlock := func() {
		if !inCode {
			return
		}
		output = append(output, renderCodeBlockLinesOnSurface(m, codeLines, codeLanguage, width, bg)...)
		codeLines = codeLines[:0]
	}

	for i := 0; i < len(inputLines); i++ {
		line := inputLines[i]
		trimmed := strings.TrimSpace(line)

		if fenceLen, language, ok := parseMarkdownCodeFence(trimmed); ok {
			if !inCode {
				inCode = true
				codeFenceLen = fenceLen
				codeLanguage = language
				continue
			}
			if isMarkdownCodeFenceClose(trimmed, codeFenceLen) {
				flushCodeBlock()
				inCode = false
				codeFenceLen = 0
				codeLanguage = ""
				continue
			}
		}

		if inCode {
			codeLines = append(codeLines, line)
			continue
		}

		if trimmed == "" {
			output = append(output, "")
			continue
		}

		if isMarkdownThematicBreak(trimmed) {
			output = append(output, renderMarkdownDividerOnSurface(m, width, bg))
			continue
		}

		if isMarkdownTableStart(inputLines, i) {
			tableLines, next, ok := renderMarkdownTableOnSurface(m, inputLines, i, width, bg)
			if ok {
				output = append(output, tableLines...)
				i = next - 1
				continue
			}
		}

		if level, heading := parseMarkdownHeading(trimmed); level > 0 {
			style := lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
				Bold(true)
			if strings.TrimSpace(bg) != "" {
				style = style.Background(lipgloss.Color(bg))
			}
			output = appendWrappedMarkdownSurfaceLine(output, style.Render(heading), width, 0)
			continue
		}

		if quote, ok := parseMarkdownBlockQuote(trimmed); ok {
			output = append(output, renderMarkdownQuoteLinesOnSurface(m, quote, width, bg)...)
			continue
		}

		if (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) && len(trimmed) > 2 {
			bulletStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
			if strings.TrimSpace(bg) != "" {
				bulletStyle = bulletStyle.Background(lipgloss.Color(bg))
			}
			bullet := bulletStyle.Render("•")
			rest := trimmed[2:]
			if codeLines, next, ok := collectMarkdownMultilineCodeSpan(inputLines, i, rest); ok {
				output = appendMarkdownListItemCodeBlockLines(m, output, bullet, codeLines, width, bg)
				i = next - 1
				continue
			}
			styled := renderInlineMarkdownOnSurface(m, rest, bg)
			output = appendMarkdownListItemLines(output, bullet, styled, width)
			continue
		}

		if num, rest := parseNumberedListItem(trimmed); num != "" {
			markerStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
			if strings.TrimSpace(bg) != "" {
				markerStyle = markerStyle.Background(lipgloss.Color(bg))
			}
			marker := markerStyle.Render(num)
			if codeLines, next, ok := collectMarkdownMultilineCodeSpan(inputLines, i, rest); ok {
				output = appendMarkdownListItemCodeBlockLines(m, output, marker, codeLines, width, bg)
				i = next - 1
				continue
			}
			styled := renderInlineMarkdownOnSurface(m, rest, bg)
			output = appendMarkdownListItemLines(output, marker, styled, width)
			continue
		}

		styled := renderInlineMarkdownOnSurface(m, line, bg)
		output = append(output, splitWrappedStyledLines(styled, max(width, 1))...)
	}
	if inCode {
		flushCodeBlock()
	}

	if len(output) == 0 {
		return []string{""}
	}
	return output
}

func collectMarkdownMultilineCodeSpan(lines []string, start int, first string) ([]string, int, bool) {
	first = strings.TrimSpace(first)
	if !strings.HasPrefix(first, "`") || strings.Count(first, "`") != 1 {
		return nil, start, false
	}

	codeLines := []string{strings.TrimSpace(strings.TrimPrefix(first, "`"))}
	for i := start + 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if end := strings.IndexByte(line, '`'); end >= 0 {
			codeLines = append(codeLines, strings.TrimSpace(line[:end]))
			return trimEmptyCodeSpanLines(codeLines), i + 1, true
		}
		codeLines = append(codeLines, strings.TrimSpace(line))
	}
	return nil, start, false
}

func trimEmptyCodeSpanLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func appendMarkdownListItemLines(output []string, marker, content string, width int) []string {
	width = max(width, 1)
	prefix := marker + " "
	prefixWidth := ansi.StringWidth(prefix)
	if prefixWidth <= 0 || prefixWidth >= width {
		return append(output, splitWrappedStyledLines(prefix+content, width)...)
	}

	wrapped := splitWrappedStyledLines(content, max(width-prefixWidth, 1))
	continuationPrefix := strings.Repeat(" ", prefixWidth)
	for idx, line := range wrapped {
		if idx == 0 {
			output = append(output, prefix+line)
			continue
		}
		output = append(output, continuationPrefix+line)
	}
	return output
}

func appendMarkdownListItemCodeBlockLines(m Model, output []string, marker string, codeLines []string, width int, bg string) []string {
	width = max(width, 1)
	prefix := marker + " "
	prefixWidth := ansi.StringWidth(prefix)
	if prefixWidth <= 0 || prefixWidth >= width {
		return append(output, renderCodeBlockLinesOnSurface(m, codeLines, "text-wrap", width, bg)...)
	}

	blockLines := renderCodeBlockLinesOnSurface(m, codeLines, "text-wrap", max(width-prefixWidth, 1), bg)
	continuationPrefix := strings.Repeat(" ", prefixWidth)
	for idx, line := range blockLines {
		if idx == 0 {
			output = append(output, prefix+line)
			continue
		}
		output = append(output, continuationPrefix+line)
	}
	return output
}

func renderLiteralLinesOnSurface(m Model, text string, width int, bg string) []string {
	raw := strings.TrimRight(text, "\n")
	bg = resolveMarkdownSurfaceBG(m, bg)
	return cachedMarkdownSurfaceLines("literal_surface", m, raw, width, bg, nil, "", func(content string) string {
		return strings.Join(renderLiteralLinesOnSurfaceUncached(m, content, width, bg), "\n")
	})
}

func renderLiteralLinesOnSurfaceUncached(m Model, raw string, width int, bg string) []string {
	if raw == "" {
		return []string{""}
	}
	inputLines := strings.Split(raw, "\n")
	output := make([]string, 0, len(inputLines))
	for _, line := range inputLines {
		if strings.TrimSpace(line) == "" {
			output = append(output, "")
			continue
		}
		styled := renderLiteralLineOnSurface(m, line, bg)
		output = append(output, splitWrappedStyledLines(styled, max(width, 1))...)
	}
	if len(output) == 0 {
		return []string{""}
	}
	return output
}

func normalizeMarkdownSurfaceInput(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "<br />", "  \n")
	text = strings.ReplaceAll(text, "<br/>", "  \n")
	text = strings.ReplaceAll(text, "<br>", "  \n")
	return text
}

func resolveMarkdownSurfaceBG(m Model, bg string) string {
	bg = strings.TrimSpace(bg)
	if bg == "" {
		return ""
	}
	if strings.HasPrefix(bg, "#") {
		return bg
	}
	if m.theme != nil {
		if resolved := m.theme.ToneToken(bg); resolved != "" {
			return resolved
		}
		if resolved := m.theme.PaletteToken(bg); resolved != "" {
			return resolved
		}
	}
	switch bg {
	case toneBG, toneBGAlt, tonePanel, tonePanelAlt, toneLine, toneLineStrong, toneSoft:
		return toneValue(m.theme, bg)
	default:
		return bg
	}
}
