package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	diffRmBg      = lipgloss.Color("#5c1b1b")
	diffRmGutter  = lipgloss.NewStyle().Foreground(lipgloss.Color("#cc6666")).Background(diffRmBg)
	diffRmContent = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffcccc")).Background(diffRmBg)

	diffAddBg      = lipgloss.Color("#1b3d1b")
	diffAddGutter  = lipgloss.NewStyle().Foreground(lipgloss.Color("#66cc66")).Background(diffAddBg)
	diffAddContent = lipgloss.NewStyle().Foreground(lipgloss.Color("#ccffcc")).Background(diffAddBg)

	diffDimGutter = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	diffHunkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6699cc"))
	diffHeader    = lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Bold(true)
)

type diffOpKind int

const (
	diffEqual diffOpKind = iota
	diffDelete
	diffInsert
)

type diffOp struct {
	kind diffOpKind
	text string
}

// diffLines computes an interleaved line diff between old and new using a
// simple LCS (longest common subsequence) algorithm.
func diffLines(old, new []string) []diffOp {
	m, n := len(old), len(new)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if old[i] == new[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < m && j < n {
		if old[i] == new[j] {
			ops = append(ops, diffOp{diffEqual, old[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, diffOp{diffDelete, old[i]})
			i++
		} else {
			ops = append(ops, diffOp{diffInsert, new[j]})
			j++
		}
	}
	for ; i < m; i++ {
		ops = append(ops, diffOp{diffDelete, old[i]})
	}
	for ; j < n; j++ {
		ops = append(ops, diffOp{diffInsert, new[j]})
	}
	return ops
}

// renderDiffOps renders a slice of diffOp into styled lines with a
// line-number gutter and full-width colored backgrounds.
func renderDiffOps(ops []diffOp, textWidth int) []string {
	// Count max line number for gutter width.
	oldCount, newCount := 0, 0
	for _, op := range ops {
		switch op.kind {
		case diffEqual:
			oldCount++
			newCount++
		case diffDelete:
			oldCount++
		case diffInsert:
			newCount++
		}
	}
	maxLineNo := max(oldCount, newCount)
	gutterW := max(len(fmt.Sprintf("%d", maxLineNo)), 2)

	prefixW := gutterW + 3 // gutter + " " + sign + " "
	contentW := max(textWidth-prefixW, 1)

	formatLine := func(lineNo int, sign string, text string, gutterStyle, contentStyle lipgloss.Style) string {
		num := fmt.Sprintf("%*d", gutterW, lineNo)
		content := sign + " " + text
		cw := lipgloss.Width(content)
		if cw > contentW {
			content = ansi.Truncate(content, contentW, "…")
		} else if cw < contentW {
			content += strings.Repeat(" ", contentW-cw)
		}
		return gutterStyle.Render(" "+num+" ") + contentStyle.Render(content)
	}

	var out []string
	oldNo, newNo := 1, 1
	for i, op := range ops {
		switch op.kind {
		case diffEqual:
			// Sentinel: empty-text equal op inserted by trimDiffContext.
			if op.text == "" && i > 0 && i < len(ops)-1 {
				// Advance line counters by counting skipped equal lines.
				// The next non-sentinel op tells us where we are.
				// For now, scan forward to find the next change and compute the gap.
				sep := strings.Repeat(" ", gutterW+1) + " ···"
				if lipgloss.Width(sep) < contentW+prefixW {
					sep += strings.Repeat(" ", contentW+prefixW-lipgloss.Width(sep))
				}
				out = append(out, diffHunkStyle.Render(sep))
				// Re-sync line counters: count remaining ops to compute final line numbers,
				// then subtract to find current position.
				remOld, remNew := 0, 0
				for _, r := range ops[i+1:] {
					switch r.kind {
					case diffEqual:
						if r.text != "" {
							remOld++
							remNew++
						}
					case diffDelete:
						remOld++
					case diffInsert:
						remNew++
					}
				}
				oldNo = oldCount - remOld + 1
				newNo = newCount - remNew + 1
				continue
			}
			num := fmt.Sprintf("%*d", gutterW, newNo)
			line := "  " + op.text
			lw := lipgloss.Width(line)
			if lw > contentW {
				line = ansi.Truncate(line, contentW, "…")
			} else if lw < contentW {
				line += strings.Repeat(" ", contentW-lw)
			}
			out = append(out, diffDimGutter.Render(" "+num+" ")+line)
			oldNo++
			newNo++
		case diffDelete:
			out = append(out, formatLine(oldNo, "-", op.text, diffRmGutter, diffRmContent))
			oldNo++
		case diffInsert:
			out = append(out, formatLine(newNo, "+", op.text, diffAddGutter, diffAddContent))
			newNo++
		}
	}
	return out
}

// trimDiffContext reduces a full-file diff to only changed hunks with up to
// contextLines surrounding equal lines. Hunks are separated by a sentinel
// diffOp (kind=diffEqual, text="") which renderDiffOps shows as "···".
func trimDiffContext(ops []diffOp, contextLines int) []diffOp {
	// Mark which ops should be kept (changed lines + context).
	keep := make([]bool, len(ops))
	for i, op := range ops {
		if op.kind != diffEqual {
			lo := max(i-contextLines, 0)
			hi := min(i+contextLines, len(ops)-1)
			for j := lo; j <= hi; j++ {
				keep[j] = true
			}
		}
	}

	var trimmed []diffOp
	skipping := false
	for i, op := range ops {
		if keep[i] {
			if skipping {
				trimmed = append(trimmed, diffOp{kind: diffEqual, text: ""})
				skipping = false
			}
			trimmed = append(trimmed, op)
		} else {
			skipping = true
		}
	}
	return trimmed
}

// DiffHunk groups a contiguous set of diff operations that contain at least
// one change (delete or insert), plus surrounding context lines.
type DiffHunk struct {
	Ops      []diffOp
	Start    int
	Accepted bool
}

// splitIntoHunks groups diff ops into hunks. Each hunk contains at least one
// non-equal op plus up to contextLines of surrounding equal ops. Adjacent
// hunks that would overlap in context are merged.
func splitIntoHunks(ops []diffOp, contextLines int) []DiffHunk {
	if len(ops) == 0 {
		return nil
	}

	// Find indices of all change ops.
	var changeIdxs []int
	for i, op := range ops {
		if op.kind != diffEqual {
			changeIdxs = append(changeIdxs, i)
		}
	}
	if len(changeIdxs) == 0 {
		return nil // no changes
	}

	// Group changes into hunks by merging overlapping context windows.
	type span struct{ lo, hi int } // inclusive range in ops
	var spans []span
	for _, ci := range changeIdxs {
		lo := max(ci-contextLines, 0)
		hi := min(ci+contextLines, len(ops)-1)
		if len(spans) > 0 && lo <= spans[len(spans)-1].hi+1 {
			// Merge with previous span.
			if hi > spans[len(spans)-1].hi {
				spans[len(spans)-1].hi = hi
			}
		} else {
			spans = append(spans, span{lo, hi})
		}
	}

	hunks := make([]DiffHunk, len(spans))
	for i, sp := range spans {
		hunks[i] = DiffHunk{
			Ops:      append([]diffOp{}, ops[sp.lo:sp.hi+1]...), // copy to avoid aliasing
			Start:    sp.lo,
			Accepted: true,
		}
	}
	return hunks
}

// reconstructContent replays the full diff, using the new version for accepted
// hunks and the old version for rejected hunks. Returns the reconstructed file
// content as a single string.
func reconstructContent(allOps []diffOp, hunks []DiffHunk) string {
	// Build acceptance map: opIndex → accepted. Ops not in any hunk are
	// between hunks (context) and always use the equal/old version.
	hunkOf := make(map[int]int, len(allOps)) // opIndex → hunkIndex, -1 = not in hunk
	for hi, h := range hunks {
		for i := range h.Ops {
			hunkOf[h.Start+i] = hi
		}
	}

	var result []string
	for i, op := range allOps {
		hi, inHunk := hunkOf[i]
		if !inHunk {
			// Between hunks, keep equal lines from the old content.
			if op.kind == diffEqual {
				result = append(result, op.text)
			}
			continue
		}
		acc := hunks[hi].Accepted
		switch op.kind {
		case diffEqual:
			result = append(result, op.text)
		case diffDelete:
			if !acc {
				result = append(result, op.text) // rejected: keep old
			}
		case diffInsert:
			if acc {
				result = append(result, op.text) // accepted: keep new
			}
		}
	}
	return strings.Join(result, "\n")
}

// renderEditDiffLines returns styled diff lines with a line-number gutter
// and full-width colored backgrounds (red for removed, green for added).
func renderEditDiffLines(msg Message, _, _ lipgloss.Style, textWidth int) []string {
	var fields map[string]any
	if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err != nil {
		return []string{msg.ToolOutput}
	}
	oldStr, _ := fields["oldString"].(string)
	newStr, _ := fields["newString"].(string)

	if oldStr == "" && newStr == "" {
		return []string{msg.ToolOutput}
	}

	var oldLines, newLines []string
	if oldStr != "" {
		oldLines = strings.Split(strings.TrimRight(oldStr, "\n"), "\n")
	}
	if newStr != "" {
		newLines = strings.Split(strings.TrimRight(newStr, "\n"), "\n")
	}

	return renderDiffOps(diffLines(oldLines, newLines), textWidth)
}

// isUnifiedDiff returns true if the output looks like a unified diff
// (e.g. from git diff). Checks for the presence of diff headers and hunks.
func isUnifiedDiff(output string) bool {
	hasDiffHeader := strings.Contains(output, "diff --git") ||
		strings.Contains(output, "--- a/") ||
		strings.Contains(output, "--- /")
	hasHunk := strings.Contains(output, "@@ ")
	return hasDiffHeader && hasHunk
}

// renderUnifiedDiffLines renders unified diff output with colored styling:
// red background for removed lines, green for added, blue for hunk headers,
// bold for file headers, dim for context and metadata.
func renderUnifiedDiffLines(output string, textWidth int) []string {
	padToWidth := func(s string, style lipgloss.Style) string {
		w := lipgloss.Width(s)
		if w < textWidth {
			s += strings.Repeat(" ", textWidth-w)
		}
		return style.Render(s)
	}

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git"):
			rendered = append(rendered, diffHeader.Render(line))
		case strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "new file"),
			strings.HasPrefix(line, "deleted file"),
			strings.HasPrefix(line, "similarity"):
			rendered = append(rendered, diffDimGutter.Render(line))
		case strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ "):
			rendered = append(rendered, diffHeader.Render(line))
		case strings.HasPrefix(line, "@@"):
			rendered = append(rendered, diffHunkStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			rendered = append(rendered, padToWidth(line, diffRmContent))
		case strings.HasPrefix(line, "+"):
			rendered = append(rendered, padToWidth(line, diffAddContent))
		default:
			rendered = append(rendered, diffDimGutter.Render(line))
		}
	}
	return rendered
}
