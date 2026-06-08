package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
)

func renderMutationToolTimelineSection(m Model, workspaceRoot string, call *events.ToolCallState, width int) string {
	display, ok := mutationDisplayFromCall(workspaceRoot, call)
	if ok {
		return renderMutationTranscriptSection(m, workspaceRoot, call, display, width)
	}
	body := strings.TrimSpace(mutationToolTranscriptBody(workspaceRoot, call))
	if body == "" {
		return ""
	}
	return renderMutationTranscriptSection(m, workspaceRoot, call, mutationDisplay{Summary: body}, width)
}

func renderWideToolDetailTranscriptSection(m Model, ref sessionToolCallRef, state events.SessionState, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	return renderWideToolSection(
		m,
		wideToolDetailTitleForSession(state, call),
		toolStatus(call),
		toolDetailTranscriptMetaLines(call),
		toolDetailTranscriptContent(m, &ref, call, width),
		width,
	)
}

func renderToolDetailTranscriptSection(m Model, ref sessionToolCallRef, state events.SessionState, call *events.ToolCallState, width int, selected bool) string {
	body := toolDetailTranscriptBody(m, ref, call, width)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	accent := colorFor(m.theme, "secondary", "#7dcfff")
	if selected {
		accent = colorFor(m.theme, "primary", "#7cc7ff")
	}
	return renderTranscriptBlock(m, toolDetailTranscriptTitle(state, ref, call), body, width, transcriptBlockStyle{
		accent:        accent,
		errorSections: strings.TrimSpace(toolResultError(m, &ref, call)) != "",
	})
}

func toolDetailTranscriptTitle(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) string {
	if call == nil {
		return "Tool"
	}
	title := strings.TrimSpace(toolDisplayNameForSession(state, call))
	if title == "" {
		title = "tool"
	}
	ordinal := sessionToolTurnOrdinal(state, ref.TurnID)
	if ordinal > 0 {
		return "Tool • turn " + strconv.Itoa(ordinal) + " • " + title + " • " + toolStatus(call)
	}
	return "Tool • " + title + " • " + toolStatus(call)
}

func toolDetailTranscriptBody(m Model, ref sessionToolCallRef, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if details := toolDetailTranscriptDetails(call); details != "" {
		parts = append(parts, details)
	}
	if content := toolDetailTranscriptContent(m, &ref, call, width); content != "" {
		parts = append(parts, content)
	}
	return strings.Join(parts, "\n\n")
}

func toolDetailTranscriptMetaLines(call *events.ToolCallState) []string {
	if isTaskToolCall(call) {
		return nil
	}
	params := toolInspectorParams(call)
	lines := make([]string, 0, len(params)*2)
	for _, param := range params {
		if param.Error {
			continue
		}
		label := strings.TrimSpace(param.Label)
		value := strings.TrimSpace(param.Value)
		if label == "" || value == "" {
			continue
		}
		if strings.Contains(value, "\n") {
			lines = append(lines, label+":")
			lines = append(lines, strings.Split(value, "\n")...)
			continue
		}
		lines = append(lines, label+": "+value)
	}
	return lines
}

func toolDetailTranscriptDetails(call *events.ToolCallState) string {
	lines := toolDetailTranscriptMetaLines(call)
	return strings.Join(lines, "\n")
}

func toolDetailTranscriptContent(m Model, ref *sessionToolCallRef, call *events.ToolCallState, width int) string {
	parts := make([]string, 0, 2)
	if input := toolDetailTranscriptInput(call, width); input != "" {
		parts = append(parts, input)
	}
	if output := toolDetailTranscriptOutput(m, ref, call, width); output != "" {
		parts = append(parts, output)
	}
	return strings.Join(parts, "\n\n")
}

func toolDetailTranscriptInput(call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	if isTaskToolCall(call) {
		if rendered, ok := renderTaskToolInputTranscript(call, width); ok {
			return rendered
		}
	}
	if isMCPToolCall(call) {
		if structured, ok := renderStructuredPayloadTranscript(call.Input, width); ok {
			return structured
		}
	}
	_, arguments, ok := toolDetailArgumentsBlock(call)
	if !ok {
		return ""
	}
	return arguments
}

func toolDetailTranscriptOutput(m Model, ref *sessionToolCallRef, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	output := strings.TrimSpace(toolResultOutput(m, ref, call))
	errorText := strings.TrimSpace(toolResultError(m, ref, call))
	if errorText != "" && isTaskToolCall(call) {
		errorText = strings.TrimSpace(taskToolErrorDisplayText(call, errorText))
	}
	notice := ""
	if !toolResultBodyLoaded(m, ref) {
		notice = toolResultPreviewNotice(call)
	}
	switch {
	case output != "" && errorText != "":
		renderedOutput := output
		if isTaskToolCall(call) {
			if structured, ok := renderTaskToolOutputTranscript(output, width); ok {
				renderedOutput = structured
			}
		} else if isMCPToolCall(call) {
			if structured, ok := renderStructuredPayloadTranscriptDelta(output, call.Input, width); ok {
				renderedOutput = structured
			} else if structured, ok := renderStructuredPayloadTranscript(output, width); ok {
				renderedOutput = structured
			}
		}
		body := strings.TrimSpace(renderedOutput)
		if body != "" {
			body += "\n\n"
		}
		body += "Error:\n" + errorText
		if notice != "" {
			body += "\n\n(" + notice + ")"
		}
		return body
	case output != "":
		if isTaskToolCall(call) {
			if structured, ok := renderTaskToolOutputTranscript(output, width); ok {
				if notice != "" {
					return structured + "\n\n(" + notice + ")"
				}
				return structured
			}
		}
		if isMCPToolCall(call) {
			if structured, ok := renderStructuredPayloadTranscriptDelta(output, call.Input, width); ok {
				if notice != "" {
					return structured + "\n\n(" + notice + ")"
				}
				return structured
			}
			if structured, ok := renderStructuredPayloadTranscript(output, width); ok {
				if notice != "" {
					return structured + "\n\n(" + notice + ")"
				}
				return structured
			}
		}
		if notice != "" {
			return output + "\n\n(" + notice + ")"
		}
		return output
	case errorText != "":
		body := "Error:\n" + errorText
		if notice != "" {
			body += "\n\n(" + notice + ")"
		}
		return body
	case call.Executing:
		return "(running...)"
	case strings.TrimSpace(toolResultHydrationPlaceholder(m, ref, call)) != "":
		return "(" + strings.TrimSpace(toolResultHydrationPlaceholder(m, ref, call)) + ")"
	default:
		return ""
	}
}

func renderSearchTranscriptSection(m Model, ref sessionToolCallRef, workspaceRoot string, call *events.ToolCallState, width int) string {
	if strings.TrimSpace(call.ToolName) == "locate" {
		input, ok := parseLocateToolViewInput(call.Input)
		if !ok {
			return renderWideToolDetailTranscriptSection(m, ref, events.SessionState{WorkspaceRoot: workspaceRoot}, call, width)
		}

		meta := []string{
			"Hidden: " + onOffLabel(input.IncludeHidden),
			"Max Matches: " + strconv.Itoa(input.MaxMatches),
		}
		return renderWideToolSection(
			m,
			wideToolDetailTitle(workspaceRoot, call),
			toolStatus(call),
			meta,
			toolDetailTranscriptOutput(m, &ref, call, width),
			width,
		)
	}

	input, ok := parseSearchToolViewInput(call.Input)
	if !ok {
		return renderWideToolDetailTranscriptSection(m, ref, events.SessionState{WorkspaceRoot: workspaceRoot}, call, width)
	}

	meta := []string{
		"Mode: " + input.Mode,
		"Regex: " + onOffLabel(input.Regex),
		"Case: " + onOffLabel(input.CaseSensitive),
		"Max Matches: " + strconv.Itoa(input.MaxMatches),
	}
	if strings.TrimSpace(input.Glob) != "" {
		meta = append(meta, "Filter: "+input.Glob)
	}
	return renderWideToolSection(
		m,
		wideToolDetailTitle(workspaceRoot, call),
		toolStatus(call),
		meta,
		toolDetailTranscriptOutput(m, &ref, call, width),
		width,
	)
}

func renderReadTranscriptSection(m Model, ref sessionToolCallRef, workspaceRoot string, call *events.ToolCallState, width int) string {
	return renderReadTranscriptSectionForSession(m, "", ref, workspaceRoot, call, width)
}

func renderReadTranscriptSectionForSession(m Model, sessionID string, ref sessionToolCallRef, workspaceRoot string, call *events.ToolCallState, width int) string {
	input, ok := parseReadToolViewInput(call.Input)
	if !ok {
		return renderWideToolDetailTranscriptSection(m, ref, events.SessionState{WorkspaceRoot: workspaceRoot}, call, width)
	}
	meta := make([]string, 0, 2)
	if input.HasOffset {
		meta = append(meta, "Offset: "+strconv.Itoa(input.Offset))
	}
	if input.HasLimit {
		meta = append(meta, "Limit: "+strconv.Itoa(input.Limit))
	}
	return renderWideToolSectionWithBodyLines(
		m,
		wideToolDetailTitle(workspaceRoot, call),
		toolStatus(call),
		meta,
		readTranscriptBodyLines(m, sessionID, ref, call, width),
		width,
	)
}

func renderWideToolSection(m Model, title, status string, meta []string, body string, width int) string {
	return renderWideToolSectionWithBodyLines(m, title, status, meta, renderWideToolBodyLines(m, body, width), width)
}

func renderWideToolSectionWithBodyLines(m Model, title, status string, meta []string, bodyLines []string, width int) string {
	return cachedTranscriptRender("wide_tool_section", m, width, func() string {
		lines := []string{renderTranscriptWideToolHeader(m, title, status, width)}
		if len(bodyLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, bodyLines...)
		}
		return strings.Join(lines, "\n")
	}, title, status, strings.Join(meta, "\n"), strings.Join(bodyLines, "\n"))
}

func renderWideToolHeader(m Model, title, status string, width int) string {
	return renderWideToolHeaderWithSpinner(m, title, status, width, true)
}

func renderTranscriptWideToolHeader(m Model, title, status string, width int) string {
	return renderWideToolHeaderWithSpinner(m, title, status, width, false)
}

func renderWideToolHeaderWithSpinner(m Model, title, status string, width int, showSpinner bool) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "tool"
	}
	titleColor := colorFor(m.theme, "primary", "#7cc7ff")
	switch normalizeOutcomeStatus(status) {
	case "error":
		titleColor = colorFor(m.theme, "error", "#ff9aa6")
	case "partial":
		titleColor = colorFor(m.theme, "subtext", "#9da8ca")
	}
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(titleColor)).
		Bold(true)
	prefix := ""
	if showSpinner && toolOutcomeShowsSpinner(status) {
		prefix = renderLiveSpinner(m) + " "
	}
	titleWidth := max(width-ansi.StringWidth(ansi.Strip(prefix)), 8)
	return prefix + titleStyle.Render(truncateEnd(title, titleWidth))
}

func renderWideToolBodyLines(m Model, body string, width int) []string {
	rawLines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	inErrorSection := false
	for _, line := range rawLines {
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			inErrorSection = false
			continue
		}
		lineColor := colorFor(m.theme, "text", "#ecf0ff")
		if strings.TrimSpace(line) == "Error:" {
			inErrorSection = true
		}
		if inErrorSection {
			lineColor = colorFor(m.theme, "error", "#ff9aa6")
		}
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color(lineColor)).
			Render(truncateEnd(line, max(width, 1))))
	}
	return lines
}

func readTranscriptBodyLines(m Model, sessionID string, ref sessionToolCallRef, call *events.ToolCallState, width int) []string {
	if call == nil {
		return nil
	}
	output := strings.TrimSpace(toolResultOutput(m, &ref, call))
	errorText := strings.TrimSpace(toolResultError(m, &ref, call))
	if output != "" && errorText == "" && toolDetailOutputLanguage(call, output) == "markdown" {
		lines := renderMarkdownBlockOnSurfaceWithStreamKey(m, output, max(width, 1), "", toolMarkdownStreamKey(sessionID, ref, "tool-transcript"))
		if !toolResultBodyLoaded(m, &ref) {
			if notice := strings.TrimSpace(toolResultPreviewNotice(call)); notice != "" {
				lines = append(lines, "")
				lines = append(lines, lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
					Render("("+notice+")"))
			}
		}
		return lines
	}
	return renderWideToolBodyLines(m, toolDetailTranscriptOutput(m, &ref, call, width), width)
}

func splitToolMetaLines(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func mutationToolTranscriptBody(workspaceRoot string, call *events.ToolCallState) string {
	switch call.ToolName {
	case "mkdir":
		return mkdirToolTranscriptBody(workspaceRoot, call)
	default:
		return mutationToolFallbackBody(call)
	}
}

func mkdirToolTranscriptBody(workspaceRoot string, call *events.ToolCallState) string {
	input, ok := parseMkdirToolViewInput(call.Input)
	if !ok {
		return mutationToolFallbackBody(call)
	}

	lines := []string{
		"Change: " + toolPrimaryListSummary(call),
		"Path: " + displayToolPath(workspaceRoot, input.Path),
	}
	if output := strings.TrimSpace(call.Output); output != "" {
		lines = append(lines, "Result: "+summarizeInlineValue(output))
	}
	return strings.Join(lines, "\n")
}

func mutationToolFallbackBody(call *events.ToolCallState) string {
	if isApplyPatchNoop(call) {
		return ""
	}

	lines := make([]string, 0, 2)
	if summary := strings.TrimSpace(toolPrimaryListSummary(call)); summary != "" {
		lines = append(lines, "Change: "+summary)
	}
	if output := summarizeBlock(call.Output); output != "" {
		if len(lines) == 0 || normalizedToolSummary(output) != normalizedToolSummary(strings.TrimPrefix(lines[0], "Change: ")) {
			lines = append(lines, "Result: "+output)
		}
	}
	return strings.Join(lines, "\n")
}

func normalizedToolSummary(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
