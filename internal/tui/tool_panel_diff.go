package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type patchDiffInput struct {
	FilePath string          `json:"filePath"`
	Edits    []patchDiffEdit `json:"edits"`
}

type patchDiffEdit struct {
	OldString string `json:"oldString"`
	NewString string `json:"newString"`
}

type patchDiffGroup struct {
	OldString string
	NewString string
	Count     int
}

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
	parsed, ok := parsePatchDiffInput(rawInput, true)
	if !ok {
		return ""
	}
	return renderPatchDiffPreview(parsed.Edits, textWidth)
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

func parsePatchDiffInput(rawInput string, allowPartial bool) (patchDiffInput, bool) {
	var parsed patchDiffInput
	if !allowPartial {
		if err := json.Unmarshal([]byte(rawInput), &parsed); err != nil {
			return patchDiffInput{}, false
		}
		return parsed, len(parsed.Edits) > 0
	}

	for _, suffix := range []string{"", `]}`, `"]}`, `""]}`, `""]}`} {
		if err := json.Unmarshal([]byte(rawInput+suffix), &parsed); err == nil {
			return parsed, len(parsed.Edits) > 0
		}
		parsed = patchDiffInput{}
	}
	return patchDiffInput{}, false
}

func patchEditCount(rawInput string) int {
	parsed, ok := parsePatchDiffInput(rawInput, false)
	if !ok {
		return 0
	}
	return len(parsed.Edits)
}

func renderPatchDiffPreview(edits []patchDiffEdit, textWidth int) string {
	groups := groupPatchDiffEdits(edits)
	if len(groups) == 0 {
		return ""
	}

	var sb strings.Builder
	if len(edits) > 1 {
		summary := fmt.Sprintf("%d edits", len(edits))
		if len(groups) != len(edits) {
			label := "changes"
			if len(groups) == 1 {
				label = "change"
			}
			summary += fmt.Sprintf(", %d unique %s", len(groups), label)
		}
		fmt.Fprintf(&sb, "  %s\n", truncate(summary, textWidth))
	}

	for i, group := range groups {
		label := patchDiffGroupLabel(i, len(groups), group.Count)
		if label != "" {
			fmt.Fprintf(&sb, "  %s\n", truncate(label, textWidth))
		}

		var oldLines, newLines []string
		if group.OldString != "" {
			oldLines = strings.Split(strings.TrimRight(group.OldString, "\n"), "\n")
		}
		if group.NewString != "" {
			newLines = strings.Split(strings.TrimRight(group.NewString, "\n"), "\n")
		}
		ops := diffLines(oldLines, newLines)
		trimmed := trimDiffContext(ops, 2)
		for _, line := range renderDiffOps(trimmed, textWidth) {
			fmt.Fprintf(&sb, "  %s\n", line)
		}
		if i < len(groups)-1 {
			sb.WriteString("\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func groupPatchDiffEdits(edits []patchDiffEdit) []patchDiffGroup {
	groups := make([]patchDiffGroup, 0, len(edits))
	indexByKey := make(map[string]int, len(edits))
	for _, edit := range edits {
		if edit.OldString == "" && edit.NewString == "" {
			continue
		}
		key := edit.OldString + "\x00" + edit.NewString
		if idx, ok := indexByKey[key]; ok {
			groups[idx].Count++
			continue
		}
		indexByKey[key] = len(groups)
		groups = append(groups, patchDiffGroup{
			OldString: edit.OldString,
			NewString: edit.NewString,
			Count:     1,
		})
	}
	return groups
}

func patchDiffGroupLabel(groupIndex, groupCount, count int) string {
	switch {
	case groupCount == 1 && count > 1:
		return fmt.Sprintf("Repeated change [%d matches]", count)
	case groupCount > 1 && count > 1:
		return fmt.Sprintf("Change %d/%d [%d matches]", groupIndex+1, groupCount, count)
	case groupCount > 1:
		return fmt.Sprintf("Change %d/%d", groupIndex+1, groupCount)
	default:
		return ""
	}
}

func completedDiffSignature(msg Message) (string, bool) {
	if msg.Role != "tool_call" || !msg.ToolDone || msg.ToolError != "" || isStructuredToolResultError(msg.ToolOutput) {
		return "", false
	}

	switch msg.ToolName {
	case "edit":
		var fields struct {
			FilePath  string `json:"filePath"`
			OldString string `json:"oldString"`
			NewString string `json:"newString"`
		}
		if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err != nil {
			return "", false
		}
		return canonicalDiffSignature(fields.FilePath, fields.OldString, fields.NewString)

	case "patch":
		parsed, ok := parsePatchDiffInput(msg.ToolInput, false)
		if !ok || len(parsed.Edits) != 1 {
			return "", false
		}
		edit := parsed.Edits[0]
		return canonicalDiffSignature(parsed.FilePath, edit.OldString, edit.NewString)
	}

	return "", false
}

func canonicalDiffSignature(filePath, oldString, newString string) (string, bool) {
	if strings.TrimSpace(oldString) == "" && strings.TrimSpace(newString) == "" {
		return "", false
	}
	filePath = strings.TrimSpace(filePath)
	if filePath != "" {
		filePath = filepath.ToSlash(filepath.Clean(filePath))
	}
	return filePath + "\x00" + oldString + "\x00" + newString, true
}
