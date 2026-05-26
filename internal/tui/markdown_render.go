package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
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
			styled := renderInlineMarkdownOnSurface(m, trimmed[2:], bg)
			output = append(output, splitWrappedStyledLines(bullet+" "+styled, max(width, 1))...)
			continue
		}

		if num, rest := parseNumberedListItem(trimmed); num != "" {
			markerStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
			if strings.TrimSpace(bg) != "" {
				markerStyle = markerStyle.Background(lipgloss.Color(bg))
			}
			marker := markerStyle.Render(num)
			styled := renderInlineMarkdownOnSurface(m, rest, bg)
			output = append(output, splitWrappedStyledLines(marker+" "+styled, max(width, 1))...)
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
