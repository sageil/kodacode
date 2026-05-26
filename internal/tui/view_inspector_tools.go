package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
)

func renderToolsListInspector(m Model, state events.SessionState, width int) string {
	if isWideShell(m) {
		return renderOutcomeToolsListInspector(m, state, width)
	}
	refs := orderedSessionToolCallRefs(state)
	if len(refs) == 0 {
		return renderInspectorBlock(m, "Tools", "No tool calls in this session.", width)
	}
	rows := make([]string, 0, len(refs))
	for idx, ref := range refs {
		_, call := sessionToolCall(state, ref)
		if call == nil {
			continue
		}
		status := toolStatus(call)
		label := fmt.Sprintf("%d. %s", idx+1, sessionToolDisplayName(state, ref, call))
		labelColor := colorFor(m.theme, "text", "#ecf0ff")
		if selectedToolMatchesSession(m, state.SessionID, ref) {
			labelColor = colorFor(m.theme, "primary", "#7cc7ff")
		}
		statusText := lipgloss.NewStyle().
			Foreground(toolStatusColor(m.theme, status)).
			Render(status)
		rows = append(rows, joinBar(
			lipgloss.NewStyle().
				Foreground(lipgloss.Color(labelColor)).
				Render(truncateEnd(label, max(width-12, 8))),
			statusText,
			max(width, 1),
		))
	}
	return strings.Join(rows, "\n")
}

func renderOutcomeToolsListInspector(m Model, state events.SessionState, width int) string {
	rows := deriveSessionToolOutcomeRows(state)
	if len(rows) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render("No tool outcomes in this session.")
	}

	lines := make([]string, 0, len(rows)*2)
	for idx, row := range rows {
		labelColor := colorFor(m.theme, "text", "#ecf0ff")
		if selectedToolMatchesSession(m, state.SessionID, row.Ref) {
			labelColor = colorFor(m.theme, "primary", "#7cc7ff")
		}
		prefix := fmt.Sprintf("%d. ", idx+1)
		body := strings.TrimSpace(row.Label)
		if detail := strings.TrimSpace(row.Detail); detail != "" {
			body += " · " + detail
		}
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color(labelColor)).
			Render(singleLineInspectorEntry(prefix, body, max(width, 8))))
	}
	return strings.Join(lines, "\n")
}

func singleLineInspectorEntry(prefix, body string, width int) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" {
		prefix += " "
	}
	body = singleLineToolText(body)
	width = max(width, 1)

	if body == "" {
		return truncateEnd(prefix, width)
	}
	prefixWidth := ansi.StringWidth(prefix)
	if prefixWidth <= 0 || prefixWidth >= width {
		return truncateEnd(strings.TrimSpace(prefix+body), width)
	}
	return prefix + truncateEnd(body, max(width-prefixWidth, 1))
}

func singleLineToolText(text string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(text, "\r\n", "\n")), " ")
}

func sessionToolDisplayName(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) string {
	name := strings.TrimSpace(toolDisplayNameForSession(state, call))
	if strings.TrimSpace(call.ToolName) == "question" {
		if prompt := strings.TrimSpace(questionToolPrompt(call)); prompt != "" {
			name = prompt
		}
	}
	if name == "" {
		name = "tool"
	}
	if len(state.TurnOrder) <= 1 {
		return name
	}
	ordinal := sessionToolTurnOrdinal(state, ref.TurnID)
	if ordinal <= 0 {
		return name
	}
	return fmt.Sprintf("t%d %s", ordinal, name)
}
