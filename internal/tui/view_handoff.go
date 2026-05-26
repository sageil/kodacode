package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func handoffPreviewInspectorText(handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return ""
	}
	lines := make([]string, 0, 3)
	if strings.TrimSpace(handoff.PreviewAction) != "" {
		lines = append(lines, "activity: "+handoff.PreviewAction)
	}
	if strings.TrimSpace(handoff.PreviewToolName) != "" {
		lines = append(lines, "tool: "+handoff.PreviewToolName)
	}
	if strings.TrimSpace(handoff.PreviewAssistantText) != "" {
		lines = append(lines, "preview: "+handoff.PreviewAssistantText)
	}
	return strings.Join(lines, "\n")
}

func handoffDisplayStatusColor(m Model, handoff *events.AgentHandoffState) color.Color {
	if handoff != nil && handoff.PreviewActive {
		return lipgloss.Color(colorFor(m.theme, "primary", "#8be9fd"))
	}
	return handoffStatusColor(m, handoff.Status)
}

func handoffDisplayStatusLabel(handoff *events.AgentHandoffState) string {
	if handoff != nil && handoff.PreviewActive {
		return "running"
	}
	return handoffStatusLabel(handoff.Status)
}

func handoffStatusColor(m Model, status events.AgentResultStatus) color.Color {
	switch status {
	case events.AgentResultStatusCompleted:
		return lipgloss.Color(colorFor(m.theme, "success", "#a6e3a1"))
	case events.AgentResultStatusPendingPermission, events.AgentResultStatusPendingQuestion:
		return lipgloss.Color(colorFor(m.theme, "warning", "#f9e2af"))
	default:
		return lipgloss.Color(colorFor(m.theme, "error", "#f38ba8"))
	}
}

func handoffStatusLabel(status events.AgentResultStatus) string {
	switch status {
	case events.AgentResultStatusCompleted:
		return "completed"
	case events.AgentResultStatusPendingPermission:
		return "pending permission"
	case events.AgentResultStatusPendingQuestion:
		return "pending question"
	case events.AgentResultStatusFailed:
		return "failed"
	default:
		return "delegated"
	}
}
