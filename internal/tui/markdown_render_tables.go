package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func isMarkdownTableStart(lines []string, start int) bool {
	if start < 0 || start+1 >= len(lines) {
		return false
	}
	return isMarkdownTableRow(strings.TrimSpace(lines[start])) &&
		isMarkdownTableSeparator(strings.TrimSpace(lines[start+1]))
}

func renderMarkdownTableOnSurface(m Model, lines []string, start, width int, bg string) ([]string, int, bool) {
	headers := parseMarkdownTableCells(lines[start])
	if len(headers) < 2 {
		return nil, start, false
	}

	rows := make([][]string, 0, 4)
	next := start + 2
	for next < len(lines) {
		trimmed := strings.TrimSpace(lines[next])
		if trimmed == "" || !isMarkdownTableRow(trimmed) {
			break
		}
		cells := parseMarkdownTableCells(trimmed)
		if len(cells) < 2 {
			break
		}
		rows = append(rows, alignMarkdownTableCells(cells, len(headers)))
		next++
	}
	if len(rows) == 0 {
		return nil, start, false
	}

	if gridLines, ok := renderMarkdownGridTableOnSurface(m, headers, rows, width, bg); ok {
		return gridLines, next, true
	}

	return renderMarkdownTableFallbackOnSurface(m, headers, rows, width, bg), next, true
}

func renderMarkdownGridTableOnSurface(m Model, headers []string, rows [][]string, width int, bg string) ([]string, bool) {
	colWidths, ok := markdownTableColumnWidths(m, headers, rows, width, bg)
	if !ok {
		return nil, false
	}

	borderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m)))
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
		Bold(true)
	if strings.TrimSpace(bg) != "" {
		bgColor := lipgloss.Color(bg)
		borderStyle = borderStyle.Background(bgColor)
		headerStyle = headerStyle.Background(bgColor)
	}

	horizontal := func(left, middle, right string) string {
		parts := make([]string, 0, len(colWidths)*2+1)
		parts = append(parts, left)
		for idx, colWidth := range colWidths {
			parts = append(parts, strings.Repeat("─", colWidth+2))
			if idx < len(colWidths)-1 {
				parts = append(parts, middle)
			}
		}
		parts = append(parts, right)
		return borderStyle.Render(strings.Join(parts, ""))
	}

	out := make([]string, 0, len(rows)*3+4)
	out = append(out, horizontal("┌", "┬", "┐"))
	out = append(out, renderMarkdownTableRowGridOnSurface(m, headers, colWidths, bg, headerStyle, borderStyle)...)
	out = append(out, horizontal("├", "┼", "┤"))
	for rowIdx, row := range rows {
		out = append(out, renderMarkdownTableRowGridOnSurface(m, row, colWidths, bg, lipgloss.NewStyle(), borderStyle)...)
		if rowIdx < len(rows)-1 {
			out = append(out, horizontal("├", "┼", "┤"))
		}
	}
	out = append(out, horizontal("└", "┴", "┘"))
	return out, true
}

func markdownTableColumnWidths(m Model, headers []string, rows [][]string, width int, bg string) ([]int, bool) {
	if len(headers) == 0 {
		return nil, false
	}
	width = max(width, 1)
	colWidths := make([]int, len(headers))
	minWidths := make([]int, len(headers))
	for idx, header := range headers {
		visible := ansi.StringWidth(renderInlineMarkdownOnSurface(m, header, bg))
		colWidths[idx] = max(visible, 3)
		minWidths[idx] = max(min(visible, 12), 3)
	}
	for _, row := range rows {
		for idx := range headers {
			if idx >= len(row) {
				continue
			}
			visible := ansi.StringWidth(renderInlineMarkdownOnSurface(m, row[idx], bg))
			colWidths[idx] = max(colWidths[idx], visible)
		}
	}
	for idx := range colWidths {
		colWidths[idx] = min(colWidths[idx], max(width/2, 12))
		minWidths[idx] = min(minWidths[idx], colWidths[idx])
	}

	totalWidth := markdownTableRenderedWidth(colWidths)
	for totalWidth > width {
		shrank := false
		for {
			target := -1
			for idx := range colWidths {
				if colWidths[idx] <= minWidths[idx] {
					continue
				}
				if target == -1 || colWidths[idx] > colWidths[target] {
					target = idx
				}
			}
			if target == -1 {
				break
			}
			colWidths[target]--
			totalWidth--
			shrank = true
			if totalWidth <= width {
				break
			}
		}
		if !shrank {
			return nil, false
		}
	}
	return colWidths, true
}

func markdownTableRenderedWidth(colWidths []int) int {
	total := 1
	for _, width := range colWidths {
		total += width + 3
	}
	return total
}

func renderMarkdownTableRowGridOnSurface(
	m Model,
	cells []string,
	colWidths []int,
	bg string,
	cellStyle lipgloss.Style,
	borderStyle lipgloss.Style,
) []string {
	cells = alignMarkdownTableCells(cells, len(colWidths))
	wrapped := make([][]string, len(colWidths))
	rowHeight := 1
	for idx, cell := range cells {
		content := cellStyle.Render(renderInlineMarkdownOnSurface(m, cell, bg))
		lines := splitWrappedStyledLines(content, max(colWidths[idx], 1))
		if len(lines) == 0 {
			lines = []string{""}
		}
		wrapped[idx] = lines
		rowHeight = max(rowHeight, len(lines))
	}

	rowLines := make([]string, 0, rowHeight)
	rail := borderStyle.Render("│")
	for lineIdx := 0; lineIdx < rowHeight; lineIdx++ {
		parts := make([]string, 0, len(colWidths)*2+1)
		parts = append(parts, rail)
		for colIdx, colWidth := range colWidths {
			content := ""
			if lineIdx < len(wrapped[colIdx]) {
				content = wrapped[colIdx][lineIdx]
			}
			parts = append(parts, " "+padMarkdownTableCell(content, colWidth)+" ")
			parts = append(parts, rail)
		}
		rowLines = append(rowLines, strings.Join(parts, ""))
	}
	return rowLines
}

func padMarkdownTableCell(content string, width int) string {
	padding := max(width-ansi.StringWidth(content), 0)
	return content + strings.Repeat(" ", padding)
}

func renderMarkdownTableFallbackOnSurface(m Model, headers []string, rows [][]string, width int, bg string) []string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
		Bold(true)
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Bold(true)
	if strings.TrimSpace(bg) != "" {
		bgColor := lipgloss.Color(bg)
		titleStyle = titleStyle.Background(bgColor)
		labelStyle = labelStyle.Background(bgColor)
	}

	out := make([]string, 0, len(rows)*4)
	for rowIdx, row := range rows {
		if rowIdx > 0 {
			out = append(out, "")
		}
		title := strings.TrimSpace(row[0])
		if title != "" {
			out = appendWrappedMarkdownSurfaceLine(out, titleStyle.Render(renderInlineMarkdownOnSurface(m, title, bg)), width, 0)
		}
		for colIdx, header := range headers {
			if colIdx >= len(row) {
				break
			}
			value := strings.TrimSpace(row[colIdx])
			if value == "" {
				continue
			}
			if colIdx == 0 && title != "" {
				continue
			}
			label := strings.TrimSpace(header)
			if label == "" {
				label = "Field"
			}
			content := labelStyle.Render(label+": ") + renderInlineMarkdownOnSurface(m, value, bg)
			out = appendWrappedMarkdownSurfaceLine(out, content, width, 2)
		}
	}
	return out
}

func parseMarkdownTableCells(line string) []string {
	trimmed := strings.TrimSpace(line)
	if !isMarkdownTableRow(trimmed) {
		return nil
	}
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	if len(parts) < 2 {
		return nil
	}
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}

func alignMarkdownTableCells(cells []string, width int) []string {
	if len(cells) >= width {
		return cells[:width]
	}
	aligned := append([]string(nil), cells...)
	for len(aligned) < width {
		aligned = append(aligned, "")
	}
	return aligned
}

func appendWrappedMarkdownSurfaceLine(lines []string, content string, width, indent int) []string {
	if strings.TrimSpace(ansi.Strip(content)) == "" {
		return lines
	}
	for _, line := range splitWrappedStyledLines(content, max(width-indent, 1)) {
		lines = append(lines, strings.Repeat(" ", indent)+line)
	}
	return lines
}

func renderMarkdownDividerOnSurface(m Model, width int, bg string) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m)))
	if strings.TrimSpace(bg) != "" {
		style = style.Background(lipgloss.Color(bg))
	}
	return style.Render(strings.Repeat("─", max(width, 1)))
}
