package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func showCommandToolInTranscript(turn *events.TurnState, call *events.ToolCallState) bool {
	if call == nil || !isCommandToolCall(call) {
		return false
	}
	if !transcriptOwnsToolCallRow(call) {
		return false
	}
	return true
}

func renderCommandTranscriptSection(m Model, workspaceRoot string, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	title := strings.TrimSpace(commandToolDisplayName(workspaceRoot, call))
	if title == "" {
		title = strings.TrimSpace(call.ToolName)
	}
	if title == "" {
		title = "command"
	}
	return renderWideToolSection(m, title, toolStatus(call), nil, commandTranscriptStatusLabel(call), width)
}

func commandTranscriptStatusLabel(call *events.ToolCallState) string {
	switch normalizeOutcomeStatus(toolStatus(call)) {
	case "error":
		return "Failed"
	case "running", "declared", "preparing", "building":
		return "Running"
	default:
		return "Succeeded"
	}
}
