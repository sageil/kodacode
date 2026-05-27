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
	rows := deriveTurnToolOutcomeRows(state, refs)
	if len(rows) == 0 {
		return nil
	}
	sections := make([]transcriptSection, 0, len(rows))
	for _, row := range rows {
		_, call := sessionToolCall(state, row.Ref)
		if call != nil && strings.TrimSpace(call.ToolName) == "question" {
			if content := strings.TrimSpace(renderQuestionOutcomeTranscriptSection(m, state, row.Ref, call, width)); content != "" {
				sections = append(sections, transcriptSection{content: content, toolRefs: []sessionToolCallRef{row.Ref}})
			}
			continue
		}
		if row.Kind == toolOutcomeMutation || isMutationToolCall(call) {
			if content := strings.TrimSpace(renderShellMutationToolTranscriptSection(m, state, row.Ref, call, width)); content != "" {
				sections = append(sections, transcriptSection{content: content, toolRefs: []sessionToolCallRef{row.Ref}})
			}
			continue
		}
		content := renderShellToolOutcomeLine(m, state, row, call, width, selectedToolMatchesSession(m, state.SessionID, row.Ref))
		if strings.TrimSpace(content) == "" {
			continue
		}
		sections = append(sections, transcriptSection{content: content, toolRefs: []sessionToolCallRef{row.Ref}})
	}
	return sections
}

func renderShellToolTranscriptSection(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int) string {
	if call != nil && strings.TrimSpace(call.ToolName) == "question" {
		return renderQuestionOutcomeTranscriptSection(m, state, ref, call, width)
	}
	rows := deriveTurnToolOutcomeRows(state, []sessionToolCallRef{ref})
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
	singularExploration := shellToolRowSingularExploration(row)
	if row.Kind == toolOutcomeExploration && singularExploration && call != nil {
		if callLabel := strings.TrimSpace(groupedToolItemLabel(state.WorkspaceRoot, call)); callLabel != "" {
			label = callLabel
		}
	}
	if detail := strings.TrimSpace(row.Detail); detail != "" {
		if singularExploration {
			return singleLineToolText(label)
		}
		if label == "" {
			label = detail
		} else if !strings.Contains(label, detail) {
			label += " · " + detail
		}
	}
	return singleLineToolText(label)
}

func shellToolRowKind(row toolOutcomeRow, call *events.ToolCallState) string {
	switch row.Kind {
	case toolOutcomeExploration:
		if shellToolRowSingularExploration(row) && call != nil {
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

func shellToolRowSingularExploration(row toolOutcomeRow) bool {
	if row.Kind != toolOutcomeExploration {
		return false
	}
	detail := strings.TrimSpace(row.Detail)
	return strings.HasPrefix(detail, "1 ") && !strings.Contains(detail, " · ")
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
