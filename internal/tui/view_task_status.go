package tui

import "github.com/sageil/kodacode/internal/events"

func turnTaskStatus(turn *events.TurnState) string {
	if turn == nil {
		return "waiting"
	}
	switch turn.Status {
	case events.TurnStatusCompleted:
		return "done"
	case events.TurnStatusCanceled:
		return "canceled"
	case events.TurnStatusFailed:
		return "error"
	case events.TurnStatusRunning:
		return "running"
	default:
		return "waiting"
	}
}

func handoffTaskStatus(handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return "waiting"
	}
	if handoff.PreviewActive {
		return "running"
	}
	switch handoff.Status {
	case events.AgentResultStatusCompleted:
		return "done"
	case events.AgentResultStatusFailed:
		return "error"
	case events.AgentResultStatusPendingPermission, events.AgentResultStatusPendingQuestion:
		return "waiting"
	default:
		return "running"
	}
}
