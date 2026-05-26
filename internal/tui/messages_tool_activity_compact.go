package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderCompactWideTurnToolOutcomeSections(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	rows := deriveTurnToolOutcomeRows(state, refs)
	if len(rows) == 0 {
		return nil
	}

	sections := make([]transcriptSection, 0, len(rows))
	for _, row := range rows {
		_, call := sessionToolCall(state, row.Ref)
		content := ""
		if call != nil && strings.TrimSpace(call.ToolName) == "question" {
			content = strings.TrimSpace(renderQuestionOutcomeTranscriptSection(m, state, row.Ref, call, width))
		}
		if isMCPToolCall(call) {
			content = strings.TrimSpace(renderMCPToolTranscriptSection(m, row.Ref, state, call, width))
		}
		if content == "" && row.Kind == toolOutcomeMutation && shouldRenderMutationOutcomeDetailsInWideTranscript(call) {
			content = strings.TrimSpace(renderToolOutcomeSummarySection(m, state, row, call, width))
		}
		if content == "" {
			content = renderCompactWideToolOutcomeLine(m, row, width)
		}
		sections = append(sections, transcriptSection{
			content:  content,
			toolRefs: []sessionToolCallRef{row.Ref},
		})
	}
	return sections
}

func shouldRenderMutationOutcomeDetailsInWideTranscript(call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	switch strings.TrimSpace(call.ToolName) {
	case "write", "edit", "apply_patch", "bash":
		return true
	default:
		return false
	}
}

func renderCompactWideToolOutcomeLine(m Model, row toolOutcomeRow, width int) string {
	label := strings.TrimSpace(row.Label)
	if label == "" {
		label = "Tool"
	}
	line := label
	if detail := strings.TrimSpace(row.Detail); detail != "" {
		line += " · " + detail
	}

	color := colorFor(m.theme, "text", "#ecf0ff")
	switch normalizeOutcomeStatus(row.Status) {
	case "error":
		color = colorFor(m.theme, "error", "#ff9aa6")
	case "running", "preparing", "building":
		color = colorFor(m.theme, "warning", "#ffd28f")
	}
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(color))
	if selectedToolMatchesSession(m, m.sessionID, row.Ref) {
		style = style.
			Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
			Bold(true)
	}
	return style.Render(truncateEnd(line, max(width, 8)))
}
