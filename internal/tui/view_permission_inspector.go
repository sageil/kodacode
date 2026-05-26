package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderPendingExecutionInspector(m Model, state events.SessionState, pending events.ExecutionApprovalState, width int) string {
	warningColor := colorFor(m.theme, "warning", "#f9e2af")
	detailWidth := max(width-4, 1)

	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(warningColor)).
		Bold(true).
		Render("Execution Approval Required")
	if strings.TrimSpace(pending.Reason) != "" {
		header += "\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#a6adc8"))).
			Width(detailWidth).
			Render(pending.Reason)
	}

	cardStyle := lipgloss.NewStyle().
		Width(max(width, 1)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(warningColor)).
		Padding(0, 1)

	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m))).
		Render(strings.Repeat("─", detailWidth))

	details := strings.Join([]string{
		renderDetailRow(m, "TOOL", pending.ToolName, detailWidth, false),
		renderDetailRow(m, "KIND", executionApprovalKindLabel(pending), detailWidth, false),
		renderDetailRow(m, "DIR", displayWorkingDirectory(state.WorkspaceRoot, pending.WorkingDirectory), detailWidth, true),
	}, "\n")

	cardBody := header + "\n" + sep + "\n" + details
	parts := []string{cardStyle.Render(cardBody)}
	if strings.TrimSpace(pending.Command) != "" {
		parts = append(parts, renderCommandBox(m, pending.Command, width))
	}
	parts = append(parts, renderDecisionChoices(m, width))
	if grants := renderSessionGrantCard(m, state, width); grants != "" {
		parts = append(parts, grants)
	}
	return strings.Join(parts, "\n\n")
}

func renderPendingPermissionInspector(m Model, state events.SessionState, pending events.PermissionRequestState, width int) string {
	warningColor := colorFor(m.theme, "warning", "#f9e2af")
	detailWidth := max(width-4, 1)

	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(warningColor)).
		Bold(true).
		Render("Permission Required")
	if strings.TrimSpace(pending.Reason) != "" {
		header += "\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#a6adc8"))).
			Width(detailWidth).
			Render(pending.Reason)
	}

	cardStyle := lipgloss.NewStyle().
		Width(max(width, 1)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(warningColor)).
		Padding(0, 1)

	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m))).
		Render(strings.Repeat("─", detailWidth))

	rows := []string{
		renderDetailRow(m, "TOOL", pending.ToolName, detailWidth, false),
		renderDetailRow(m, "KIND", permissionKindLabel(pending.Kind, pending.Access), detailWidth, false),
	}
	switch pending.Kind {
	case events.PermissionRequestKindExecution:
		rows = append(rows, renderDetailRow(m, "DIR", displayWorkingDirectory(state.WorkspaceRoot, pending.WorkingDirectory), detailWidth, true))
	case events.PermissionRequestKindNetwork:
		rows = append(rows, renderDetailRow(m, "TARGET", strings.TrimSpace(pending.Path), detailWidth, true))
	default:
		rows = append(rows, renderDetailRow(m, "PATH", displaySessionPath(state.WorkspaceRoot, pending.Path), detailWidth, false))
		rows = append(rows, renderDetailRow(m, "ACCESS", permissionAccessLabel(pending.Access), detailWidth, true))
	}

	cardBody := header + "\n" + sep + "\n" + strings.Join(rows, "\n")
	parts := []string{cardStyle.Render(cardBody)}
	if strings.TrimSpace(pending.Command) != "" {
		parts = append(parts, renderCommandBox(m, pending.Command, width))
	}
	parts = append(parts, renderDecisionChoices(m, width))
	if grants := renderSessionGrantCard(m, state, width); grants != "" {
		parts = append(parts, grants)
	}
	return strings.Join(parts, "\n\n")
}
