package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderInlinePermissionPrompt(m Model, state events.SessionState, width int) string {
	if m.pendingInteractionSubmissionInFlight() {
		return ""
	}
	if pending := m.pendingExecution(); pending != nil {
		return renderInlineExecutionApprovalPrompt(m, state, *pending, width)
	}
	if pending := m.pendingPermission(); pending != nil {
		return renderInlinePermissionRequestPrompt(m, state, *pending, width)
	}
	return ""
}

func renderInlineExecutionApprovalPrompt(m Model, state events.SessionState, pending events.ExecutionApprovalState, width int) string {
	title := "Execution approval required"
	details := []string{
		strings.TrimSpace(pending.Reason),
		strings.TrimSpace(pending.ToolName + " · " + executionApprovalKindLabel(pending)),
		strings.TrimSpace(displayWorkingDirectory(state.WorkspaceRoot, pending.WorkingDirectory)),
		inlineCommandSummary(pending.Command),
	}
	return renderInlineDecisionPrompt(m, width, title, details, executionDecisionLabels(pending))
}

func renderInlinePermissionRequestPrompt(m Model, state events.SessionState, pending events.PermissionRequestState, width int) string {
	title := "Permission required"
	details := []string{
		strings.TrimSpace(pending.Reason),
		strings.TrimSpace(pending.ToolName + " · " + permissionKindLabel(pending.Kind, pending.Access)),
		inlinePermissionTargetLabel(state, pending),
		inlineCommandSummary(pending.Command),
	}
	return renderInlineDecisionPrompt(m, width, title, details, inlinePermissionDecisionLabels())
}

func renderInlineDecisionPrompt(m Model, width int, title string, details []string, choices []string) string {
	width = max(width, 1)
	accentColor := colorFor(m.theme, "warning", "#ffd28f")
	textColor := colorFor(m.theme, "text", "#ecf0ff")
	subtextColor := colorFor(m.theme, "subtext", "#9da8ca")
	selectedColor := colorFor(m.theme, "primary", "#7cc7ff")

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(subtextColor))
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentColor)).
		Bold(true)
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(selectedColor)).
		Bold(true)
	optionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(textColor))

	topBorder := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentColor)).
		Render(strings.Repeat("▔", width))
	bottomSep := dimStyle.Render(strings.Repeat("─", width))

	lines := []string{topBorder}
	for _, line := range wrapTranscriptText(strings.TrimSpace(title), max(min(width-8, 96), 20)) {
		lines = append(lines, centerInlinePromptLine(promptStyle.Render(line), width))
	}
	for _, detail := range details {
		for _, line := range wrapTranscriptText(strings.TrimSpace(detail), max(min(width-10, 104), 20)) {
			if strings.TrimSpace(line) == "" {
				continue
			}
			lines = append(lines, centerInlinePromptLine(dimStyle.Render(line), width))
		}
	}
	lines = append(lines, centerInlinePromptLine(dimStyle.Render(permissionPanelHints(len(choices))), width))

	optionWidth := max(min(width-10, 96), 18)
	for idx, option := range choices {
		bullet := m.terminalIcon(terminalIconUnselected) + " "
		style := optionStyle
		if idx == m.interaction.cursor {
			bullet = m.terminalIcon(terminalIconSelected) + " "
			style = selectedStyle
		}
		number := fmt.Sprintf("%d.", idx+1)
		label := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(option), number))
		numberRendered := dimStyle.Render(number)
		labelWidth := max(optionWidth-lipgloss.Width(number)-1-lipgloss.Width(bullet), 8)
		wrapped := wrapTranscriptText(label, labelWidth)
		for lineIdx, line := range wrapped {
			if lineIdx == 0 {
				lines = append(lines, centerInlinePromptLine(numberRendered+" "+style.Render(bullet+line), width))
				continue
			}
			continuation := strings.Repeat(" ", lipgloss.Width(number)+1)
			lines = append(lines, centerInlinePromptLine(continuation+style.Render(strings.Repeat(" ", lipgloss.Width(bullet))+line), width))
		}
	}

	lines = append(lines, bottomSep)
	return strings.Join(lines, "\n")
}

func permissionPromptPanelHeight(m Model, state events.SessionState, width int) int {
	panel := renderInlinePermissionPrompt(m, state, width)
	if strings.TrimSpace(panel) == "" {
		return 0
	}
	return lipgloss.Height(panel) + 2
}

func inlinePermissionDecisionLabels() []string {
	return []string{
		"1. allow once",
		"2. allow for session duration",
		"3. deny",
	}
}

func permissionPanelHints(count int) string {
	switch {
	case count <= 1:
		return "1 choose · enter confirm"
	case count <= 9:
		return fmt.Sprintf("1-%d quick select · ↑/↓ choose · enter confirm", count)
	default:
		return "↑/↓ choose · enter confirm"
	}
}

func inlinePermissionTargetLabel(state events.SessionState, pending events.PermissionRequestState) string {
	switch pending.Kind {
	case events.PermissionRequestKindExecution:
		return "dir: " + displayWorkingDirectory(state.WorkspaceRoot, pending.WorkingDirectory)
	case events.PermissionRequestKindNetwork:
		return "target: " + strings.TrimSpace(pending.Path)
	default:
		target := displaySessionPath(state.WorkspaceRoot, pending.Path)
		if target == "" {
			return ""
		}
		return strings.ToLower(permissionTargetLabel(pending.Kind, pending.Access)) + ": " + target
	}
}

func inlineCommandSummary(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	lines := strings.Split(command, "\n")
	head := strings.TrimSpace(lines[0])
	if len(lines) == 1 {
		return "command: " + truncateEnd(head, 88)
	}
	extra := fmt.Sprintf("%d more lines", len(lines)-1)
	if strings.Contains(head, "<<") {
		extra = fmt.Sprintf("heredoc, %d more lines", len(lines)-1)
	}
	return "command: " + truncateEnd(head, 72) + " · " + extra
}
