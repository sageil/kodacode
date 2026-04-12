package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (m *Messages) renderStreamingDiffPreview(msg Message, textWidth int) string {
	fields := parsePartialToolInput(msg.ToolInput)

	switch msg.ToolName {
	case "edit":
		return m.streamingEditDiff(fields, textWidth)
	case "write":
		return m.streamingWriteDiff(fields, textWidth)
	case "patch":
		return m.streamingPatchDiff(fields, msg.ToolInput, textWidth)
	}
	return ""
}

func (m *Messages) streamingEditDiff(fields map[string]string, textWidth int) string {
	oldStr := fields["oldString"]
	newStr := fields["newString"]
	if oldStr == "" && newStr == "" {
		return ""
	}
	var oldLines, newLines []string
	if oldStr != "" {
		oldLines = strings.Split(strings.TrimRight(oldStr, "\n"), "\n")
	}
	if newStr != "" {
		newLines = strings.Split(strings.TrimRight(newStr, "\n"), "\n")
	}
	ops := diffLines(oldLines, newLines)
	trimmed := trimDiffContext(ops, 3)
	return renderStreamingDiff(trimmed, textWidth)
}

func (m *Messages) streamingWriteDiff(fields map[string]string, textWidth int) string {
	newContent := fields["content"]
	if newContent == "" {
		return ""
	}
	filePath := fields["filePath"]
	newLines := strings.Split(strings.TrimRight(newContent, "\n"), "\n")

	if filePath != "" {
		oldContent := streamDiffOldContent(filePath)
		if oldContent != "" {
			oldLines := strings.Split(strings.TrimRight(oldContent, "\n"), "\n")
			ops := diffLines(oldLines, newLines)
			trimmed := trimDiffContext(ops, 3)
			return renderStreamingDiff(trimmed, textWidth)
		}
	}
	var ops []diffOp
	for _, l := range newLines {
		ops = append(ops, diffOp{kind: diffInsert, text: l})
	}
	return renderStreamingDiff(ops, textWidth)
}

func (m *Messages) streamingPatchDiff(_ map[string]string, rawInput string, textWidth int) string {
	var parsed map[string]any
	for _, suffix := range []string{"", `]}`, `"]}`, `""]}`, `""]}`} {
		if json.Unmarshal([]byte(rawInput+suffix), &parsed) == nil {
			break
		}
		parsed = nil
	}
	if parsed == nil {
		return ""
	}
	edits, ok := parsed["edits"].([]any)
	if !ok || len(edits) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, e := range edits {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		oldStr, _ := em["oldString"].(string)
		newStr, _ := em["newString"].(string)
		if oldStr == "" && newStr == "" {
			continue
		}
		var oldLines, newLines []string
		if oldStr != "" {
			oldLines = strings.Split(strings.TrimRight(oldStr, "\n"), "\n")
		}
		if newStr != "" {
			newLines = strings.Split(strings.TrimRight(newStr, "\n"), "\n")
		}
		ops := diffLines(oldLines, newLines)
		trimmed := trimDiffContext(ops, 3)
		sb.WriteString(renderStreamingDiff(trimmed, textWidth))
	}
	return sb.String()
}

func renderStreamingDiff(ops []diffOp, textWidth int) string {
	lines := renderDiffOps(ops, textWidth)
	if len(lines) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&sb, "  %s\n", line)
	}
	return sb.String()
}
