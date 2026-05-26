package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

type mutationSideBySideRow struct {
	oldLine int
	newLine int
	oldText string
	newText string
	oldKind mutationDiffOpKind
	newKind mutationDiffOpKind
	skip    bool
}

type mutationSideBySideLine struct {
	line int
	text string
	kind mutationDiffOpKind
}

func renderMutationSideBySideOpsAt(m Model, ops []mutationDiffOp, textWidth, oldStart, newStart int) []string {
	rows, lineNoWidth := mutationSideBySideRows(ops, oldStart, newStart)
	if len(rows) == 0 {
		return nil
	}

	const divider = " │ "
	leftWidth, rightWidth := mutationSideBySideColumnWidths(rows, textWidth, lipgloss.Width(divider), lineNoWidth)

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
		Bold(true)
	dividerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))

	lines := []string{
		headerStyle.Render(padSideBySideHeader("Old", leftWidth)) +
			dividerStyle.Render(divider) +
			headerStyle.Render(padSideBySideHeader("New", rightWidth)),
	}

	for _, row := range rows {
		if row.skip {
			lines = append(lines, renderMutationSideBySideSkipRow(m, leftWidth, rightWidth))
			continue
		}

		leftSegments := renderMutationSideBySideCell(row.oldLine, sideBySideSignForKind(row.oldKind), row.oldText, lineNoWidth, leftWidth)
		rightSegments := renderMutationSideBySideCell(row.newLine, sideBySideSignForKind(row.newKind), row.newText, lineNoWidth, rightWidth)
		leftStyle := mutationSideBySideCellStyle(m, row.oldKind)
		rightStyle := mutationSideBySideCellStyle(m, row.newKind)

		rowLines := max(len(leftSegments), len(rightSegments))
		for idx := 0; idx < rowLines; idx++ {
			left := blankSideBySideCell(leftWidth)
			right := blankSideBySideCell(rightWidth)
			if idx < len(leftSegments) {
				left = leftSegments[idx]
			}
			if idx < len(rightSegments) {
				right = rightSegments[idx]
			}
			lines = append(lines,
				leftStyle.Render(left)+
					dividerStyle.Render(divider)+
					rightStyle.Render(right),
			)
		}
	}
	return lines
}

func mutationSideBySideColumnWidths(rows []mutationSideBySideRow, textWidth, dividerWidth, lineNoWidth int) (int, int) {
	contentWidth := max(textWidth-dividerWidth, 2)
	leftWidth := max(contentWidth/2, 1)
	rightWidth := max(contentWidth-leftWidth, 1)

	minColumnWidth := max(lineNoWidth+6, mutationSideBySideMinColumnWidth)
	if minColumnWidth*2 > contentWidth {
		return leftWidth, rightWidth
	}

	leftWeight, rightWeight := mutationSideBySideColumnWeights(rows, lineNoWidth)
	totalWeight := leftWeight + rightWeight
	if leftWeight == 0 || rightWeight == 0 || totalWeight == 0 {
		return leftWidth, rightWidth
	}

	leftWidth = contentWidth * leftWeight / totalWeight
	if leftWidth < minColumnWidth {
		leftWidth = minColumnWidth
	}
	maxLeftWidth := contentWidth - minColumnWidth
	if leftWidth > maxLeftWidth {
		leftWidth = maxLeftWidth
	}
	rightWidth = max(contentWidth-leftWidth, 1)
	return leftWidth, rightWidth
}

func mutationSideBySideColumnWeights(rows []mutationSideBySideRow, lineNoWidth int) (int, int) {
	leftWeight, rightWeight := 0, 0
	for _, row := range rows {
		if row.skip {
			leftWeight += 3
			rightWeight += 3
			continue
		}
		leftWeight += mutationSideBySideCellWeight(row.oldLine, row.oldText, lineNoWidth)
		rightWeight += mutationSideBySideCellWeight(row.newLine, row.newText, lineNoWidth)
	}
	if leftWeight == 0 {
		leftWeight = 1
	}
	if rightWeight == 0 {
		rightWeight = 1
	}
	return leftWeight, rightWeight
}

func mutationSideBySideCellWeight(lineNo int, text string, lineNoWidth int) int {
	if lineNo < 1 && text == "" {
		return 0
	}
	return lineNoWidth + 3 + utf8.RuneCountInString(text)
}

func mutationSideBySideRows(ops []mutationDiffOp, oldStart, newStart int) ([]mutationSideBySideRow, int) {
	oldKnown := oldStart > 0
	newKnown := newStart > 0
	if !oldKnown {
		oldStart = 0
	}
	if !newKnown {
		newStart = 0
	}

	oldCount, newCount := mutationDiffVisibleLineCounts(ops)
	maxOldLine := oldStart
	if oldKnown && oldCount > 0 {
		maxOldLine = oldStart + oldCount - 1
	}
	maxNewLine := newStart
	if newKnown && newCount > 0 {
		maxNewLine = newStart + newCount - 1
	}
	lineNoWidth := max(max(lenInt(maxOldLine), lenInt(maxNewLine)), 2)

	oldLine, newLine := oldStart, newStart
	rows := make([]mutationSideBySideRow, 0, len(ops))
	for i := 0; i < len(ops); {
		op := ops[i]
		if op.kind == mutationDiffEqual {
			if op.text == "" {
				rows = append(rows, mutationSideBySideRow{skip: true})
				i++
				continue
			}
			rows = append(rows, mutationSideBySideRow{
				oldLine: oldLine,
				newLine: newLine,
				oldText: op.text,
				newText: op.text,
				oldKind: mutationDiffEqual,
				newKind: mutationDiffEqual,
			})
			if oldKnown {
				oldLine++
			}
			if newKnown {
				newLine++
			}
			i++
			continue
		}

		deletes := make([]mutationSideBySideLine, 0, 2)
		inserts := make([]mutationSideBySideLine, 0, 2)
		for i < len(ops) && ops[i].kind != mutationDiffEqual {
			switch ops[i].kind {
			case mutationDiffDelete:
				deletes = append(deletes, mutationSideBySideLine{line: oldLine, text: ops[i].text, kind: mutationDiffDelete})
				if oldKnown {
					oldLine++
				}
			case mutationDiffInsert:
				inserts = append(inserts, mutationSideBySideLine{line: newLine, text: ops[i].text, kind: mutationDiffInsert})
				if newKnown {
					newLine++
				}
			}
			i++
		}

		blockLen := max(len(deletes), len(inserts))
		for idx := 0; idx < blockLen; idx++ {
			row := mutationSideBySideRow{}
			if idx < len(deletes) {
				row.oldLine = deletes[idx].line
				row.oldText = deletes[idx].text
				row.oldKind = deletes[idx].kind
			}
			if idx < len(inserts) {
				row.newLine = inserts[idx].line
				row.newText = inserts[idx].text
				row.newKind = inserts[idx].kind
			}
			rows = append(rows, row)
		}
	}
	return rows, lineNoWidth
}

func renderMutationSideBySideCell(lineNo int, sign, text string, lineNoWidth, cellWidth int) []string {
	if lineNo < 1 && text == "" {
		return []string{blankSideBySideCell(cellWidth)}
	}

	prefix := mutationSideBySideCellPrefix(lineNo, sign, lineNoWidth, false)
	continuation := mutationSideBySideCellPrefix(0, "", lineNoWidth, true)
	if lipgloss.Width(prefix) >= cellWidth {
		return []string{padOrTrimSideBySideCell(prefix, cellWidth)}
	}

	available := max(cellWidth-lipgloss.Width(prefix), 1)
	head, tail := hardWrapMutationText(text, available)
	lines := []string{padOrTrimSideBySideCell(prefix+head, cellWidth)}
	for tail != "" {
		available = max(cellWidth-lipgloss.Width(continuation), 1)
		head, tail = hardWrapMutationText(tail, available)
		lines = append(lines, padOrTrimSideBySideCell(continuation+head, cellWidth))
	}
	return lines
}

func mutationSideBySideCellPrefix(lineNo int, sign string, lineNoWidth int, continuation bool) string {
	if continuation {
		return strings.Repeat(" ", lineNoWidth+3)
	}
	if lineNo < 1 {
		return strings.Repeat(" ", lineNoWidth+3)
	}
	if sign == "" {
		sign = " "
	}
	return lipgloss.PlaceHorizontal(lineNoWidth, lipgloss.Right, strconv.Itoa(lineNo)) + " " + sign + " "
}

func mutationSideBySideCellStyle(m Model, kind mutationDiffOpKind) lipgloss.Style {
	_, addContentStyle, _, deleteContentStyle, _, _ := mutationDiffStyles(m)
	switch kind {
	case mutationDiffDelete:
		return deleteContentStyle
	case mutationDiffInsert:
		return addContentStyle
	default:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "text", "#cdd6f4")))
	}
}

func renderMutationSideBySideSkipRow(m Model, leftWidth, rightWidth int) string {
	markerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff")))
	dividerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	return markerStyle.Render(padSideBySideHeader("···", leftWidth)) +
		dividerStyle.Render(" │ ") +
		markerStyle.Render(padSideBySideHeader("···", rightWidth))
}

func padSideBySideHeader(text string, width int) string {
	return lipgloss.PlaceHorizontal(width, lipgloss.Left, truncateEnd(text, width))
}

func blankSideBySideCell(width int) string {
	return strings.Repeat(" ", max(width, 0))
}

func padOrTrimSideBySideCell(text string, width int) string {
	text = truncateEnd(text, max(width, 1))
	if pad := width - lipgloss.Width(text); pad > 0 {
		return text + strings.Repeat(" ", pad)
	}
	return text
}

func sideBySideSignForKind(kind mutationDiffOpKind) string {
	switch kind {
	case mutationDiffDelete:
		return "-"
	case mutationDiffInsert:
		return "+"
	default:
		return " "
	}
}

func lenInt(value int) int {
	if value < 1 {
		return 1
	}
	return len(strconv.Itoa(value))
}
