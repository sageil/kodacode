package textdiff

import "strings"

type OpKind string

const (
	OpContext OpKind = "context"
	OpDelete  OpKind = "delete"
	OpInsert  OpKind = "insert"
	OpSkip    OpKind = "skip"
)

type PreviewOp struct {
	Kind OpKind `json:"kind,omitempty"`
	Text string `json:"text,omitempty"`
}

type Preview struct {
	OldStartLine int         `json:"old_start_line,omitempty"`
	NewStartLine int         `json:"new_start_line,omitempty"`
	Ops          []PreviewOp `json:"ops,omitempty"`
}

func (p Preview) Valid() bool {
	if p.OldStartLine < 0 || p.NewStartLine < 0 {
		return false
	}
	if len(p.Ops) == 0 {
		return true
	}
	if p.OldStartLine == 0 && p.NewStartLine == 0 {
		return false
	}
	for _, op := range p.Ops {
		switch op.Kind {
		case OpContext, OpDelete, OpInsert, OpSkip:
		default:
			return false
		}
	}
	return true
}

func BuildPreview(before, after string, contextLines int) Preview {
	full := diffLines(splitNormalizedLines(before), splitNormalizedLines(after))
	if !hasChangeOps(full) {
		return Preview{}
	}
	if contextLines < 0 {
		contextLines = 0
	}
	keep := buildKeepMask(full, contextLines)
	preview := Preview{}
	oldLine, newLine := 1, 1
	started := false
	skipping := false

	for i, op := range full {
		if keep[i] {
			if !started {
				preview.OldStartLine = oldLine
				preview.NewStartLine = newLine
				started = true
			} else if skipping {
				preview.Ops = append(preview.Ops, PreviewOp{Kind: OpSkip})
				skipping = false
			}
			preview.Ops = append(preview.Ops, op)
		} else if started {
			skipping = true
		}
		switch op.Kind {
		case OpContext:
			oldLine++
			newLine++
		case OpDelete:
			oldLine++
		case OpInsert:
			newLine++
		}
	}

	return preview
}

func HasChanges(preview Preview) bool {
	for _, op := range preview.Ops {
		if op.Kind == OpDelete || op.Kind == OpInsert {
			return true
		}
	}
	return false
}

func LineStats(preview Preview) (added, removed int) {
	for _, op := range preview.Ops {
		switch op.Kind {
		case OpDelete:
			removed++
		case OpInsert:
			added++
		}
	}
	return added, removed
}

func diffLines(oldLines, newLines []string) []PreviewOp {
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

	var ops []PreviewOp
	i, j := 0, 0
	for i < m && j < n {
		if oldLines[i] == newLines[j] {
			ops = append(ops, PreviewOp{Kind: OpContext, Text: oldLines[i]})
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, PreviewOp{Kind: OpDelete, Text: oldLines[i]})
			i++
			continue
		}
		ops = append(ops, PreviewOp{Kind: OpInsert, Text: newLines[j]})
		j++
	}
	for ; i < m; i++ {
		ops = append(ops, PreviewOp{Kind: OpDelete, Text: oldLines[i]})
	}
	for ; j < n; j++ {
		ops = append(ops, PreviewOp{Kind: OpInsert, Text: newLines[j]})
	}
	return ops
}

func buildKeepMask(ops []PreviewOp, contextLines int) []bool {
	keep := make([]bool, len(ops))
	for i, op := range ops {
		if op.Kind == OpContext {
			continue
		}
		lo := max(i-contextLines, 0)
		hi := min(i+contextLines, len(ops)-1)
		for j := lo; j <= hi; j++ {
			keep[j] = true
		}
	}
	return keep
}

func hasChangeOps(ops []PreviewOp) bool {
	for _, op := range ops {
		if op.Kind == OpDelete || op.Kind == OpInsert {
			return true
		}
	}
	return false
}

func splitNormalizedLines(text string) []string {
	if text == "" {
		return nil
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(strings.TrimRight(normalized, "\n"), "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
