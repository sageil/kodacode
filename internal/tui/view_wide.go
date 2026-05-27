package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func isWideShell(m Model) bool {
	return terminalWidth(m) >= 126
}

func renderWidePaneTitle(m Model, label, right string, width int, accent string) string {
	titleColor := colorFor(m.theme, "subtext", "#9da8ca")
	if strings.TrimSpace(accent) != "" {
		titleColor = lerpHexColor(titleColor, accent, 0.35)
	}
	left := lipgloss.NewStyle().
		Foreground(lipgloss.Color(titleColor)).
		Render(strings.ToUpper(strings.TrimSpace(label)))
	right = strings.TrimSpace(right)
	if right == "" {
		return lipgloss.NewStyle().Width(max(width, 1)).Render(left)
	}
	meta := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render(truncateEnd(right, max(width/3, 4)))
	return lipgloss.NewStyle().
		Width(max(width, 1)).
		Render(joinBar(left, meta, max(width, 1)))
}

func shellSessionLabel(m Model, state events.SessionState) string {
	if base := strings.TrimSpace(headerSessionTitle(m, state)); base != "" {
		return base
	}
	if compact := compactID(strings.TrimSpace(m.sessionID)); compact != "" {
		return compact
	}
	return "session"
}

func (m Model) windowTitle() string {
	label := strings.TrimSpace(shellSessionLabel(m, m.projector.CurrentState()))
	if label == "" {
		label = "Workspace session"
	}
	return "KC | " + label
}

func latestTerminalCommandCall(state events.SessionState) *events.ToolCallState {
	refs := orderedSessionToolCallRefs(state)
	for i := len(refs) - 1; i >= 0; i-- {
		_, call := sessionToolCall(state, refs[i])
		if isCommandToolCall(call) {
			return call
		}
	}
	return nil
}

func showWideToolTimelineRow(call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	if showMutationToolInTranscript(call) {
		return false
	}
	switch strings.TrimSpace(call.ToolName) {
	case "read", "search", "locate", "web_fetch", "list", "tree", "test":
		return true
	case "write", "mkdir":
		return !call.Completed
	default:
		return false
	}
}

func renderWideToolTimelineRow(m Model, state events.SessionState, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	label, target := wideToolDescriptor(state, call)
	status := lipgloss.NewStyle().
		Foreground(lipgloss.Color(wideToolTone(m, call))).
		Render(wideToolStatusSummary(m, call))
	leftWidth := max(width-lipgloss.Width(status)-1, 8)
	targetWidth := leftWidth - len(label) - 1
	if targetWidth < 0 {
		targetWidth = 0
	}
	if target != "" {
		target = truncateEnd(target, targetWidth)
	}
	leftParts := []string{
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "soft", softTextColor))).
			Render(m.toolStatusSymbol(toolStatus(call))),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
			Render(label),
	}
	if target != "" {
		leftParts = append(leftParts, lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render(target))
	}
	left := strings.Join(leftParts, " ")
	return joinBar(left, status, max(width, 1))
}

func wideToolDescriptor(state events.SessionState, call *events.ToolCallState) (string, string) {
	if call == nil {
		return "Tool", ""
	}
	if isCommandToolCall(call) {
		target := strings.TrimSpace(commandToolDisplayName(state.WorkspaceRoot, call))
		if target == "" || target == strings.TrimSpace(call.ToolName) {
			return prettyToolName(strings.TrimSpace(call.ToolName)), ""
		}
		return prettyToolName(strings.TrimSpace(call.ToolName)), target
	}
	if display := strings.TrimSpace(mcpToolDisplayName(state, call)); display != "" {
		return "MCP", display
	}
	name := prettyToolName(strings.TrimSpace(call.ToolName))
	display := strings.TrimSpace(toolDisplayNameForWorkspace(state.WorkspaceRoot, call))
	prefix := strings.TrimSpace(call.ToolName)
	target := ""
	if display != "" && prefix != "" && strings.HasPrefix(display, prefix+" ") {
		target = strings.TrimSpace(display[len(prefix):])
	} else if display != "" && display != prefix {
		target = display
	}
	return name, target
}

func prettyToolName(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "_", " "))
	if name == "" {
		return "Tool"
	}
	parts := strings.Fields(name)
	for i, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(string(runes[0])) + strings.ToLower(string(runes[1:]))
	}
	return strings.Join(parts, " ")
}

func wideToolStatusSummary(m Model, call *events.ToolCallState) string {
	if call != nil && call.Execution != nil && call.Execution.Background != nil {
		background := call.Execution.Background
		prefix := m.toolStatusSymbol(toolStatus(call))
		switch background.Status {
		case events.ExecutionBackgroundStatusStarting:
			return prefix + " starting"
		case events.ExecutionBackgroundStatusRunning:
			return prefix + " running"
		case events.ExecutionBackgroundStatusReady:
			return prefix + " ready"
		case events.ExecutionBackgroundStatusSupervisionLost:
			return prefix + " supervision lost"
		case events.ExecutionBackgroundStatusExited:
			if call.Execution != nil && call.Execution.DurationMS > 0 && strings.TrimSpace(background.Error) == "" {
				return prefix + " done · " + formatDurationMS(call.Execution.DurationMS)
			}
			if strings.TrimSpace(background.Error) != "" {
				return prefix + " error"
			}
			return prefix + " done"
		}
	}
	status := toolStatus(call)
	prefix := m.toolStatusSymbol(status)
	var label string
	switch status {
	case "done":
		label = "done"
	case "running":
		label = "running"
	case "error":
		label = "error"
	case "preparing", "building", "declared", "waiting":
		label = "pending"
	default:
		label = status
	}
	if call != nil && call.Execution != nil && call.Completed && call.Execution.DurationMS > 0 && label == "done" {
		label += " · " + formatDurationMS(call.Execution.DurationMS)
	}
	return prefix + " " + label
}

func wideToolTone(m Model, call *events.ToolCallState) string {
	if call == nil {
		return colorFor(m.theme, "subtext", "#9da8ca")
	}
	switch toolStatus(call) {
	case "done":
		return colorFor(m.theme, "success", "#90e5b4")
	case "running":
		return colorFor(m.theme, "warning", "#ffd28f")
	case "error":
		return colorFor(m.theme, "error", "#ff9aa6")
	default:
		return colorFor(m.theme, "subtext", "#9da8ca")
	}
}

func formatDurationMS(ms int64) string {
	if ms <= 0 {
		return ""
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	seconds := float64(ms) / 1000
	if seconds < 10 {
		return fmt.Sprintf("%.1fs", seconds)
	}
	return fmt.Sprintf("%.0fs", seconds)
}

type wideAgentRow struct {
	label  string
	status string
	depth  int
	active bool
}

func orderedWideAgentRows(m Model, state events.SessionState) []wideAgentRow {
	turn := currentTurn(state, m.turnID)
	if turn == nil {
		return nil
	}
	rootLabel := transcriptAgentLabel(m, state, m.turnID)
	rows := []wideAgentRow{{
		label:  rootLabel,
		status: turnTaskStatus(turn),
		depth:  0,
		active: strings.TrimSpace(m.selection.handoffID) == "",
	}}
	for _, selection := range orderedAgentContextSelections(turn) {
		handoff := turn.Handoffs[selection.HandoffID]
		if handoff == nil {
			continue
		}
		label := strings.TrimSpace(selection.AgentID)
		if label == "" {
			label = "delegated"
		}
		rows = append(rows, wideAgentRow{
			label:  label,
			status: handoffTaskStatus(handoff),
			depth:  1,
			active: selection.HandoffID == strings.TrimSpace(m.selection.handoffID),
		})
	}
	return rows
}

func renderWideAgentsList(m Model, state events.SessionState, width int) string {
	rows := orderedWideAgentRows(m, state)
	if len(rows) <= 1 {
		return ""
	}
	header := renderWidePaneTitle(m, "Agents", "", width, colorFor(m.theme, "thinking", "#bd93f9"))
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		prefix := m.terminalIcon(terminalIconCursor) + " "
		if row.depth > 0 {
			prefix = strings.Repeat("  ", row.depth) + m.toolStatusSymbol(row.status) + " "
		}
		labelColor := colorFor(m.theme, "subtext", "#9da8ca")
		if row.depth == 0 {
			labelColor = colorFor(m.theme, "text", "#f8f8f2")
		}
		if row.active {
			labelColor = colorFor(m.theme, "primary", "#8be9fd")
		}
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color(labelColor)).
			Render(prefix+truncateEnd(row.label, max(width-len(prefix), 4))))
	}
	return header + "\n" + strings.Join(lines, "\n")
}
