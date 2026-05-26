package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

type mutationDiffOpKind int

const (
	mutationDiffEqual mutationDiffOpKind = iota
	mutationDiffDelete
	mutationDiffInsert
)

type mutationDiffOp struct {
	kind mutationDiffOpKind
	text string
}

type mutationDiffRow struct {
	oldLine   int
	newLine   int
	sign      string
	text      string
	kind      mutationDiffOpKind
	separator bool
}

func mutationDiffLines(oldLines, newLines []string) []mutationDiffOp {
	m, n := len(oldLines), len(newLines)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []mutationDiffOp
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			ops = append(ops, mutationDiffOp{kind: mutationDiffEqual, text: oldLines[i]})
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, mutationDiffOp{kind: mutationDiffDelete, text: oldLines[i]})
			i++
			continue
		}
		ops = append(ops, mutationDiffOp{kind: mutationDiffInsert, text: newLines[j]})
		j++
	}
	for ; i < m; i++ {
		ops = append(ops, mutationDiffOp{kind: mutationDiffDelete, text: oldLines[i]})
	}
	for ; j < n; j++ {
		ops = append(ops, mutationDiffOp{kind: mutationDiffInsert, text: newLines[j]})
	}
	return ops
}

func trimMutationDiffContext(ops []mutationDiffOp, contextLines int) []mutationDiffOp {
	keep := make([]bool, len(ops))
	for i, op := range ops {
		if op.kind == mutationDiffEqual {
			continue
		}
		lo := max(i-contextLines, 0)
		hi := min(i+contextLines, len(ops)-1)
		for j := lo; j <= hi; j++ {
			keep[j] = true
		}
	}

	var trimmed []mutationDiffOp
	skipping := false
	for i, op := range ops {
		if keep[i] {
			if skipping {
				trimmed = append(trimmed, mutationDiffOp{kind: mutationDiffEqual, text: ""})
				skipping = false
			}
			trimmed = append(trimmed, op)
			continue
		}
		skipping = true
	}
	return trimmed
}

func renderMutationDiffOps(m Model, ops []mutationDiffOp, textWidth int) []string {
	return renderMutationDiffOpsAt(m, ops, textWidth, 1, 1)
}

func renderMutationDiffOpsAt(m Model, ops []mutationDiffOp, textWidth, oldStart, newStart int) []string {
	rows, lineNoWidth := mutationDiffRows(ops, oldStart, newStart)
	gutterWidth := mutationDiffGutterWidth(lineNoWidth)
	contentWidth := max(textWidth-gutterWidth, 1)

	addGutterStyle, addContentStyle, deleteGutterStyle, deleteContentStyle, equalGutterStyle, hunkStyle := mutationDiffStyles(m)

	formatLine := func(row mutationDiffRow, gutterStyle, contentStyle lipgloss.Style) []string {
		segments := wrapMutationDiffContent(contentWidth, row.sign, row.text)
		if len(segments) == 0 {
			segments = []string{mutationDiffContent(row.sign, "")}
		}
		lines := make([]string, 0, len(segments))
		for idx, segment := range segments {
			gutter := mutationDiffContinuationGutter(row, lineNoWidth, idx > 0)
			lines = append(lines, gutterStyle.Render(gutter)+contentStyle.Render(padMutationDiffSegment(segment, contentWidth)))
		}
		return lines
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.separator {
			separator := strings.Repeat(" ", gutterWidth) + "···"
			if lipgloss.Width(separator) < textWidth {
				separator += strings.Repeat(" ", textWidth-lipgloss.Width(separator))
			}
			out = append(out, hunkStyle.Render(separator))
			continue
		}
		switch row.kind {
		case mutationDiffEqual:
			for idx, segment := range wrapMutationDiffContent(contentWidth, row.sign, row.text) {
				gutter := mutationDiffContinuationGutter(row, lineNoWidth, idx > 0)
				out = append(out, equalGutterStyle.Render(gutter)+padMutationDiffSegment(segment, contentWidth))
			}
		case mutationDiffDelete:
			out = append(out, formatLine(row, deleteGutterStyle, deleteContentStyle)...)
		case mutationDiffInsert:
			out = append(out, formatLine(row, addGutterStyle, addContentStyle)...)
		}
	}
	return out
}

func mutationDiffRows(ops []mutationDiffOp, oldStart, newStart int) ([]mutationDiffRow, int) {
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
	lineNoWidth := max(max(len(fmt.Sprintf("%d", maxOldLine)), len(fmt.Sprintf("%d", maxNewLine))), 2)

	rows := make([]mutationDiffRow, 0, len(ops))
	oldLine, newLine := oldStart, newStart
	for i, op := range ops {
		switch op.kind {
		case mutationDiffEqual:
			if op.text == "" && i > 0 && i < len(ops)-1 {
				rows = append(rows, mutationDiffRow{separator: true})
				remainingOld, remainingNew := mutationDiffVisibleLineCounts(ops[i+1:])
				if oldKnown {
					oldLine = oldStart + oldCount - remainingOld
				}
				if newKnown {
					newLine = newStart + newCount - remainingNew
				}
				continue
			}
			rows = append(rows, mutationDiffRow{
				oldLine: oldLine,
				newLine: newLine,
				sign:    " ",
				text:    op.text,
				kind:    mutationDiffEqual,
			})
			if oldKnown {
				oldLine++
			}
			if newKnown {
				newLine++
			}
		case mutationDiffDelete:
			rows = append(rows, mutationDiffRow{
				oldLine: oldLine,
				sign:    "-",
				text:    op.text,
				kind:    mutationDiffDelete,
			})
			if oldKnown {
				oldLine++
			}
		case mutationDiffInsert:
			rows = append(rows, mutationDiffRow{
				newLine: newLine,
				sign:    "+",
				text:    op.text,
				kind:    mutationDiffInsert,
			})
			if newKnown {
				newLine++
			}
		}
	}
	return rows, lineNoWidth
}

func mutationDiffVisibleLineCounts(ops []mutationDiffOp) (int, int) {
	oldCount, newCount := 0, 0
	for _, op := range ops {
		switch op.kind {
		case mutationDiffEqual:
			if op.text != "" {
				oldCount++
				newCount++
			}
		case mutationDiffDelete:
			oldCount++
		case mutationDiffInsert:
			newCount++
		}
	}
	return oldCount, newCount
}

func mutationDiffGutterWidth(lineNoWidth int) int {
	return (lineNoWidth * 2) + 5
}

func mutationDiffGutter(row mutationDiffRow, width int) string {
	return " " + mutationDiffLineNo(row.oldLine, width) + " | " + mutationDiffLineNo(row.newLine, width) + " "
}

func mutationDiffLineNo(lineNo, width int) string {
	if lineNo < 1 {
		return strings.Repeat(" ", width)
	}
	return fmt.Sprintf("%*d", width, lineNo)
}

func mutationDiffContent(sign, text string) string {
	if sign == "" || sign == " " {
		return "  " + text
	}
	return sign + " " + text
}

func mutationDiffContinuationGutter(row mutationDiffRow, width int, continuation bool) string {
	if !continuation {
		return mutationDiffGutter(row, width)
	}
	return strings.Repeat(" ", mutationDiffGutterWidth(width))
}

func wrapMutationDiffContent(width int, sign, text string) []string {
	content := mutationDiffContent(sign, text)
	if width <= 0 {
		return []string{content}
	}
	if lipgloss.Width(content) <= width {
		return []string{content}
	}

	segments := make([]string, 0, 2)
	remaining := text
	prefix := mutationDiffContent(sign, "")
	continuationPrefix := mutationDiffContent("", "")
	currentPrefix := prefix

	for {
		available := max(width-lipgloss.Width(currentPrefix), 1)
		head, tail := hardWrapMutationText(remaining, available)
		segments = append(segments, currentPrefix+head)
		if tail == "" {
			return segments
		}
		remaining = tail
		currentPrefix = continuationPrefix
	}
}

func hardWrapMutationText(text string, width int) (string, string) {
	if text == "" || width <= 0 {
		return "", text
	}
	consumed := 0
	used := 0
	for consumed < len(text) {
		r, size := utf8.DecodeRuneInString(text[consumed:])
		if r == utf8.RuneError && size == 1 {
			size = 1
		}
		runeWidth := lipgloss.Width(text[consumed : consumed+size])
		if used+runeWidth > width && consumed > 0 {
			break
		}
		consumed += size
		used += runeWidth
		if used >= width {
			break
		}
	}
	if consumed <= 0 {
		return text, ""
	}
	return text[:consumed], text[consumed:]
}

func padMutationDiffSegment(segment string, width int) string {
	if width <= 0 {
		return segment
	}
	switch segment {
	case "", "  ", "+ ", "- ":
		return segment
	}
	if pad := width - lipgloss.Width(segment); pad > 0 {
		return segment + strings.Repeat(" ", pad)
	}
	return segment
}

func mutationDiffStyles(m Model) (lipgloss.Style, lipgloss.Style, lipgloss.Style, lipgloss.Style, lipgloss.Style, lipgloss.Style) {
	addBG := "#153a24"
	deleteBG := "#41191f"
	addFG := colorFor(m.theme, "success", "#90e5b4")
	deleteFG := colorFor(m.theme, "error", "#ff9aa6")
	contextFG := colorFor(m.theme, "subtext", "#9da8ca")
	hunkFG := colorFor(m.theme, "primary", "#7cc7ff")

	addGutter := lipgloss.NewStyle().
		Foreground(lipgloss.Color(addFG)).
		Background(lipgloss.Color(addBG))
	addContent := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#e8fff0"))).
		Background(lipgloss.Color(addBG))
	deleteGutter := lipgloss.NewStyle().
		Foreground(lipgloss.Color(deleteFG)).
		Background(lipgloss.Color(deleteBG))
	deleteContent := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ffe7eb"))).
		Background(lipgloss.Color(deleteBG))
	equalGutter := lipgloss.NewStyle().
		Foreground(lipgloss.Color(contextFG))
	hunk := lipgloss.NewStyle().
		Foreground(lipgloss.Color(hunkFG))

	return addGutter, addContent, deleteGutter, deleteContent, equalGutter, hunk
}
