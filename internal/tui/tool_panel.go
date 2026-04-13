package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Messages) renderToolCall(msg Message, _ bool, msgIndex int) (string, *diffReviewMeta) {
	s := m.getStyles()
	dimStyle := s.dim
	successStyle := s.success
	errorStyle := s.err
	warnStyle := s.warn

	boxWidth := m.vp.Width()
	if boxWidth < 6 {
		boxWidth = 80
	}

	isError := msg.ToolDone && (msg.ToolError != "" || strings.HasPrefix(strings.TrimSpace(msg.ToolOutput), "error:"))
	accentStyle := s.secondary
	var statusIcon string
	switch {
	case !msg.ToolDone:
		frame := (pulseTick % 10) * 3
		statusIcon = accentStyle.Render(spinnerFrames[frame : frame+3])
	case isError:
		statusIcon = errorStyle.Render("⊘")
	default:
		statusIcon = accentStyle.Render("⦿")
	}

	ts := parseToolSummary(msg.ToolName, msg.ToolInput)

	if msg.ToolName == "task_output" {
		if cmd := extractTaskStatusField(msg.ToolOutput, "command"); cmd != "" {
			ts.summary = truncate(cmd, 50)
		}
	}

	nameStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "primary", lipgloss.Color("62")))
	displayName := toolDisplayName(msg.ToolName)
	if msg.ToolName == "subagent" && ts.summary != "" {
		displayName = formatAgentName(ts.summary)
		ts.summary = ""
	}
	toolName := nameStyle.Render(displayName)
	header := statusIcon + " " + toolName
	if ts.summary != "" {
		header += "  " + dimStyle.Render(ts.summary)
	}
	if ts.command != "" {
		cmd := truncate(ts.command, 60)
		header += "  " + dimStyle.Render(cmd)
	}
	if ts.args != "" {
		header += "  " + dimStyle.Render(ts.args)
	}
	if hint := toolHintText(msg); hint != "" && msg.ToolDone {
		header += "  " + dimStyle.Render(hint)
	}
	if msg.ToolDone && msg.ToolElapsed > 0 {
		header += "  " + dimStyle.Render(formatElapsed(msg.ToolElapsed))
	}
	if msg.ToolName == "subagent" && !msg.ToolDone && len(msg.SubagentActivities) > 0 {
		var actInfo string
		for i := len(msg.SubagentActivities) - 1; i >= 0; i-- {
			act := msg.SubagentActivities[i]
			actName := toolDisplayName(act.Tool)
			info := actName
			if act.Summary != "" {
				info += " " + act.Summary
			} else if act.Args != "" {
				info += " " + act.Args
			}
			if !act.Done {
				actInfo = info
				break
			}
			if actInfo == "" {
				actInfo = info
			}
		}
		if actInfo != "" {
			header += "  " + dimStyle.Render("→") + " " + accentStyle.Render(actInfo)
		}
	} else if !msg.ToolDone && msg.ToolOutput == "" && !isStreamingDiffTool(msg.ToolName) && msg.ToolName != "question" {
		header += "  " + dimStyle.Render("running…")
	}

	innerWidth := max(boxWidth-2, 1)

	if lipgloss.Width(header) > innerWidth {
		header = ansi.Truncate(header, innerWidth, "…")
	}

	var body strings.Builder
	var meta *diffReviewMeta
	body.WriteString(header)

	if msg.Collapsed {
	} else if msg.ToolName == "read" || msg.ToolName == "tree" || msg.ToolName == "task_output" || msg.ToolName == "test" || msg.ToolName == "task" {
		// Header-only for read/tree/task_output/test.
	} else if msg.ToolName == "subagent" && msg.UserExpanded && msg.ToolDone {
		textWidth := max(innerWidth-2, 1)
		m.renderSubagentBody(&body, msg, msgIndex, nameStyle, dimStyle, accentStyle, errorStyle, successStyle, warnStyle, textWidth, innerWidth)
	} else if !msg.ToolDone && isStreamingDiffTool(msg.ToolName) && msg.ToolInput != "" {
		textWidth := max(innerWidth-2, 1)
		if preview := m.renderStreamingDiffPreview(msg, textWidth); preview != "" {
			body.WriteString("\n" + strings.Repeat(" ", innerWidth))
			body.WriteString(preview)
		} else {
			body.WriteString("\n")
		}
	} else if !msg.ToolDone {
	} else if isMCPTool(msg.ToolName) && msg.ToolInput != "" {
		body.WriteString("\n" + strings.Repeat(" ", innerWidth))
		textWidth := max(innerWidth-2, 1)
		m.renderMCPToolBody(&body, msg, dimStyle, textWidth)
	} else if msg.ToolOutput != "" {
		body.WriteString("\n" + strings.Repeat(" ", innerWidth))
		textWidth := max(innerWidth-2, 1)
		meta = m.renderToolPanelContent(&body, msg, msgIndex, dimStyle, errorStyle, successStyle, warnStyle, textWidth)
	}

	panel := lipgloss.NewStyle().
		PaddingLeft(2).
		Width(boxWidth)

	return panel.Render(strings.TrimRight(body.String(), "\n")) + "\n", meta
}

func (m *Messages) renderToolPanelContent(body *strings.Builder, msg Message, msgIndex int,
	dimStyle, errorStyle, successStyle, warnStyle lipgloss.Style, textWidth int) *diffReviewMeta {

	if msg.ToolName == "subagent" || msg.ToolName == "web_fetch" {
		rendered := m.cachedMarkdown(msg.ToolOutput)
		if msg.ToolName == "subagent" {
			rendered = m.cachedMarkdownPreserveSoftBreaks(msg.ToolOutput)
		}
		for line := range strings.SplitSeq(strings.TrimRight(rendered, "\n"), "\n") {
			fmt.Fprintf(body, "  %s\n", line)
		}
		return nil
	}

	if msg.ToolError != "" {
		for line := range strings.SplitSeq(strings.TrimRight(msg.ToolOutput, "\n"), "\n") {
			fmt.Fprintf(body, "  %s\n", errorStyle.Render(line))
		}
		return nil
	}

	if msg.ToolName == "task" && isTaskListOutput(msg.ToolOutput) {
		renderTaskList(body, msg.ToolOutput, successStyle, dimStyle)
		return nil
	}

	output := msg.ToolOutput
	if msg.ToolName == "read" {
		output = stripXMLContent(output)
	}

	if msg.ToolName == "tree" {
		m.renderTree(body, output, textWidth)
		return nil
	}

	if msg.ToolName == "git" && isGitStatusOutput(output) {
		nameStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "text", lipgloss.Color("250")))
		renderGitStatus(body, output, successStyle, errorStyle, warnStyle, dimStyle, nameStyle)
		return nil
	}

	if msg.ToolName == "glob" {
		nameStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "text", lipgloss.Color("250")))
		renderGlobResults(body, output, dimStyle, nameStyle)
		return nil
	}

	if msg.ToolName == "grep" {
		m.renderGrepResults(body, output, textWidth)
		return nil
	}

	if msg.ToolName == "search" {
		m.renderSearchResults(body, output, textWidth)
		return nil
	}

	if msg.ToolName == "read_files" && strings.Contains(output, "── ") {
		m.renderBulkRead(body, output, textWidth)
		return nil
	}

	if msg.ToolName == "lsp" {
		subtextStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("241")))
		for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
			line = ansi.Truncate(line, textWidth, "…")
			fmt.Fprintf(body, "  %s\n", colorizeOutputLine(line, errorStyle, warnStyle, subtextStyle))
		}
		return nil
	}

	if msg.ToolName == "write" {
		var fields map[string]any
		if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err == nil {
			newContent, _ := fields["content"].(string)
			filePath, _ := fields["filePath"].(string)
			if newContent != "" && output != "" && output != "Wrote file successfully." {
				oldLines := strings.Split(strings.TrimRight(output, "\n"), "\n")
				newLines := strings.Split(strings.TrimRight(newContent, "\n"), "\n")
				allOps := diffLines(oldLines, newLines)
				return m.renderDiffWithHunks(body, msgIndex, allOps, oldLines, filePath, "", false, dimStyle, textWidth)
			}
			if newContent != "" {
				newLines := strings.Split(strings.TrimRight(newContent, "\n"), "\n")
				var ops []diffOp
				for _, l := range newLines {
					ops = append(ops, diffOp{kind: diffInsert, text: l})
				}
				for _, line := range renderDiffOps(ops, textWidth) {
					fmt.Fprintf(body, "  %s\n", line)
				}
				return nil
			}
		}
	}

	isBash := msg.ToolName == "bash"

	if isBash && msg.ToolInput != "" {
		cmd := extractBashCommand(msg.ToolInput)
		if cmd != "" {
			if idx := strings.IndexByte(cmd, '\n'); idx >= 0 {
				cmd = cmd[:idx]
			}
			cmd = ansi.Truncate(cmd, textWidth-2, "…")
			accentSt := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("62")))
			fmt.Fprintf(body, "  %s %s\n", accentSt.Render("$"), dimStyle.Render(cmd))
		}
	}

	if msg.ToolName == "edit" {
		var fields map[string]any
		if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err == nil {
			oldStr, _ := fields["oldString"].(string)
			newStr, _ := fields["newString"].(string)
			filePath, _ := fields["filePath"].(string)
			if oldStr != "" || newStr != "" {
				var oldLines, newLines []string
				if oldStr != "" {
					oldLines = strings.Split(strings.TrimRight(oldStr, "\n"), "\n")
				}
				if newStr != "" {
					newLines = strings.Split(strings.TrimRight(newStr, "\n"), "\n")
				}
				allOps := diffLines(oldLines, newLines)
				return m.renderDiffWithHunks(body, msgIndex, allOps, oldLines, filePath, newStr, true, dimStyle, textWidth)
			}
		}
		for _, line := range renderEditDiffLines(msg, errorStyle, successStyle, textWidth) {
			fmt.Fprintf(body, "  %s\n", line)
		}
		return nil
	}

	if msg.ToolName == "patch" {
		var fields map[string]any
		if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err == nil {
			filePath, _ := fields["filePath"].(string)
			if edits, ok := fields["edits"].([]any); ok && len(edits) > 0 {
				var meta *diffReviewMeta
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
					allOps := diffLines(oldLines, newLines)
					editMeta := m.renderDiffWithHunks(body, msgIndex, allOps, oldLines, filePath, newStr, true, dimStyle, textWidth)
					if editMeta != nil {
						meta = editMeta
					}
				}
				return meta
			}
		}
		for line := range strings.SplitSeq(strings.TrimRight(output, "\n"), "\n") {
			fmt.Fprintf(body, "  %s\n", dimStyle.Render(line))
		}
		return nil
	}

	if isUnifiedDiff(output) {
		for _, line := range renderUnifiedDiffLines(output, textWidth) {
			fmt.Fprintf(body, "  %s\n", line)
		}
		return nil
	}

	lines := collapseBlankLines(strings.Split(strings.TrimRight(output, "\n"), "\n"))
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	lang := ""
	if msg.ToolName == "read" || msg.ToolName == "write" {
		var fields map[string]any
		if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err == nil {
			if fp, ok := fields["filePath"].(string); ok {
				lang = detectLanguage(fp)
			}
		}
	}

	subtextStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("241")))
	if lang != "" && !isBash {
		lineNumStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("241")))
		m.highlightWithLineNumbers(body, lines, lang, lineNumStyle, subtextStyle, textWidth)
	} else {
		for _, line := range lines {
			if isBash {
				tLine := ansi.Truncate(line, textWidth, "…")
				colorized := colorizeOutputLine(tLine, errorStyle, warnStyle, subtextStyle)
				fmt.Fprintf(body, "  %s\n", colorized)
			} else {
				fmt.Fprintf(body, "  %s\n", subtextStyle.Render(line))
			}
		}
	}
	return nil
}
