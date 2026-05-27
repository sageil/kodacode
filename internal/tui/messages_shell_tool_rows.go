package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

const (
	shellToolRowKindWidthWide   = 8
	shellToolRowKindWidthNarrow = 6
)

func renderShellTurnToolOutcomeSections(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	rows := deriveUngroupedToolOutcomeRows(state, refs)
	if len(rows) == 0 {
		return nil
	}
	sections := make([]transcriptSection, 0, len(rows))
	compactLines := make([]string, 0, len(rows))
	compactRefs := make(map[sessionToolCallRef]int, len(rows))
	flushCompact := func() {
		if len(compactLines) == 0 {
			return
		}
		lineRefs := make(map[sessionToolCallRef]int, len(compactRefs))
		for ref, line := range compactRefs {
			lineRefs[ref] = line
		}
		sections = append(sections, transcriptSection{
			content:      strings.Join(compactLines, "\n"),
			toolLineRefs: lineRefs,
		})
		compactLines = compactLines[:0]
		clear(compactRefs)
	}
	for _, row := range rows {
		_, call := sessionToolCall(state, row.Ref)
		if call != nil && strings.TrimSpace(call.ToolName) == "question" {
			flushCompact()
			if content := strings.TrimSpace(renderQuestionOutcomeTranscriptSection(m, state, row.Ref, call, width)); content != "" {
				sections = append(sections, transcriptSection{content: content, toolRefs: []sessionToolCallRef{row.Ref}})
			}
			continue
		}
		if row.Kind == toolOutcomeMutation || isMutationToolCall(call) {
			if content := strings.TrimSpace(renderShellMutationToolTranscriptSection(m, state, row.Ref, call, width)); content != "" {
				if transcriptRenderedLineCount(content) == 1 {
					compactRefs[row.Ref] = len(compactLines)
					compactLines = append(compactLines, content)
					continue
				}
				flushCompact()
				sections = append(sections, transcriptSection{content: content, toolRefs: []sessionToolCallRef{row.Ref}})
			}
			continue
		}
		content := renderShellToolOutcomeLine(m, state, row, call, width, selectedToolMatchesSession(m, state.SessionID, row.Ref))
		if strings.TrimSpace(content) == "" {
			continue
		}
		compactRefs[row.Ref] = len(compactLines)
		compactLines = append(compactLines, content)
	}
	flushCompact()
	return sections
}

func renderShellToolTranscriptSection(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int) string {
	if call != nil && strings.TrimSpace(call.ToolName) == "question" {
		return renderQuestionOutcomeTranscriptSection(m, state, ref, call, width)
	}
	rows := deriveUngroupedToolOutcomeRows(state, []sessionToolCallRef{ref})
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		_, rowCall := sessionToolCall(state, row.Ref)
		if rowCall == nil {
			rowCall = call
		}
		if row.Kind == toolOutcomeMutation || isMutationToolCall(rowCall) {
			if content := strings.TrimSpace(renderShellMutationToolTranscriptSection(m, state, row.Ref, rowCall, width)); content != "" {
				lines = append(lines, content)
			}
			continue
		}
		if line := strings.TrimSpace(renderShellToolOutcomeLine(m, state, row, rowCall, width, selectedToolMatchesSession(m, state.SessionID, row.Ref))); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func renderShellMutationToolTranscriptSection(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	if display, ok := mutationDisplayFromCall(state.WorkspaceRoot, call); ok && display.Failure != nil {
		return strings.Join(renderMutationFailureSummaryLines(m, display, width), "\n")
	}
	lines := renderShellMutationSuccessLines(m, state, ref, call, width)
	if len(lines) == 0 {
		return renderMutationToolTimelineSection(m, state.WorkspaceRoot, call, width)
	}
	return strings.Join(lines, "\n")
}

func renderShellMutationSuccessLines(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int) []string {
	switch strings.TrimSpace(call.ToolName) {
	case "write":
		return renderWriteMutationLinesWithDiffStyle(m, state.SessionID, ref, state.WorkspaceRoot, call, width, mutationToolDetailDiffStyleSplit)
	case "apply_patch":
		return renderApplyPatchMutationLinesWithDiffStyle(m, state.WorkspaceRoot, call, width, mutationToolDetailDiffStyleSplit)
	case "bash":
		return renderBashMutationLinesWithDiffStyle(m, state.WorkspaceRoot, call, width, mutationToolDetailDiffStyleSplit)
	default:
		return renderMutationSuccessLines(m, state.WorkspaceRoot, call, width)
	}
}

func renderShellToolOutcomeLine(m Model, state events.SessionState, row toolOutcomeRow, call *events.ToolCallState, width int, selected bool) string {
	width = max(width, 1)
	status := normalizeOutcomeStatus(row.Status)
	if status == "" {
		status = normalizeOutcomeStatus(toolStatus(call))
	}
	label := shellToolRowLabel(state, row, call)
	if label == "" {
		label = shellToolPrimaryLabel(state, call)
	}
	if label == "" {
		label = shellToolKind(call)
	}
	kind := shellToolRowKind(row, call)
	right := shellToolRowRight(state, row.Ref, status, selected)
	left := shellToolRowLeft(m, kind, label, status, selected, max(width-lipgloss.Width(right)-1, 1))
	if right == "" {
		return left
	}
	return joinBar(left, right, width)
}

func shellToolRowLabel(state events.SessionState, row toolOutcomeRow, call *events.ToolCallState) string {
	label := strings.TrimSpace(row.Label)
	if shellLayoutToolRowShouldUseConciseLabel(row, call) {
		label = shellToolConciseLabel(state, call, label)
	}
	if detail := strings.TrimSpace(row.Detail); detail != "" {
		if label == "" {
			label = detail
		} else if !strings.Contains(label, detail) {
			label += " · " + detail
		}
	}
	return singleLineToolText(label)
}

func shellLayoutToolRowShouldUseConciseLabel(row toolOutcomeRow, call *events.ToolCallState) bool {
	return call != nil && row.Kind == toolOutcomeExploration
}

func shellToolConciseLabel(state events.SessionState, call *events.ToolCallState, fallback string) string {
	label := strings.TrimSpace(fallback)
	switch strings.TrimSpace(call.ToolName) {
	case "read":
		if input, ok := parseReadToolViewInput(call.Input); ok && len(input.Paths) == 1 {
			label = displayToolBaseName(state.WorkspaceRoot, input.Paths[0])
		} else {
			label = strings.TrimPrefix(label, "Read ")
			label = strings.TrimPrefix(label, "Reading ")
		}
	case "locate":
		if input, ok := parseLocateToolViewInput(call.Input); ok {
			path := displayToolPath(state.WorkspaceRoot, input.Path)
			query := strings.TrimSpace(input.Query)
			switch {
			case path != "." && path != "" && query != "":
				label = query + " under " + path
			case query != "":
				label = query
			case path != "." && path != "":
				label = path
			default:
				label = "workspace"
			}
		} else {
			label = strings.TrimPrefix(label, "Locate ")
			label = strings.TrimPrefix(label, "Locating ")
		}
	case "search":
		label = strings.TrimPrefix(label, "Search ")
		label = strings.TrimPrefix(label, "Searching ")
	case "bash":
		label = strings.TrimPrefix(label, "Shell: ")
		label = strings.TrimPrefix(label, "Shell")
	default:
		display := strings.TrimSpace(toolDisplayNameForSession(state, call))
		for _, prefix := range []string{display + " ", titleCaseASCII(display) + " "} {
			label = strings.TrimPrefix(label, prefix)
		}
	}
	if label == "" {
		label = strings.TrimSpace(fallback)
	}
	return label
}

func titleCaseASCII(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	first := value[0]
	if first >= 'a' && first <= 'z' {
		first -= 'a' - 'A'
	}
	return string(first) + value[1:]
}

func shellToolRowKind(row toolOutcomeRow, call *events.ToolCallState) string {
	switch row.Kind {
	case toolOutcomeExploration:
		if call != nil {
			return shellToolKind(call)
		}
		return "scan"
	case toolOutcomeMutation:
		if call != nil {
			return shellToolKind(call)
		}
		return "write"
	case toolOutcomeCommand:
		if call != nil {
			return shellToolKind(call)
		}
		return "cmd"
	default:
		if call != nil {
			return shellToolKind(call)
		}
		return "tool"
	}
}

func shellToolRowRight(state events.SessionState, ref sessionToolCallRef, status string, selected bool) string {
	parts := make([]string, 0, 2)
	if ordinal := sessionToolTurnOrdinal(state, ref.TurnID); ordinal > 0 {
		parts = append(parts, fmt.Sprintf("t%d", ordinal))
	}
	switch {
	case selected:
		parts = append(parts, "enter")
	case normalizeOutcomeStatus(status) != "done":
		parts = append(parts, shellToolStatusLabel(status))
	}
	return strings.Join(parts, " · ")
}

func shellToolRowLeft(m Model, kind, label, status string, selected bool, width int) string {
	width = max(width, 1)
	marker := " "
	if selected {
		marker = ">"
	}
	kindWidth := shellToolRowKindWidth(width)
	icon := toolStatusSymbol(status)
	prefixPlain := marker + " " + icon + " "
	if kindWidth > 0 {
		prefixPlain += padRight(truncateEnd(kind, kindWidth), kindWidth) + " "
	}
	labelWidth := max(width-lipgloss.Width(prefixPlain), 1)
	text := truncateEnd(label, labelWidth)

	markerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	if selected {
		markerStyle = markerStyle.Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).Bold(true)
	}
	iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(toolStatusColorHex(m.theme, status)))
	kindStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(shellToolLabelColor(m.theme, status)))
	if selected && normalizeOutcomeStatus(status) != "error" {
		labelStyle = labelStyle.Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).Bold(true)
	}

	parts := []string{
		markerStyle.Render(marker),
		" ",
		iconStyle.Render(icon),
		" ",
	}
	if kindWidth > 0 {
		parts = append(parts, kindStyle.Render(padRight(truncateEnd(kind, kindWidth), kindWidth)), " ")
	}
	parts = append(parts, labelStyle.Render(text))
	return strings.Join(parts, "")
}

func shellToolRowKindWidth(width int) int {
	switch {
	case width >= 56:
		return shellToolRowKindWidthWide
	case width >= 38:
		return shellToolRowKindWidthNarrow
	default:
		return 0
	}
}
