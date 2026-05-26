package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

func (m Model) reuseSelectedHandoffResult() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}

	turn := currentTurn(m.projector.Snapshot(), m.turnID)
	handoff := selectedHandoff(turn, m.selection.handoffID)
	if handoff == nil || handoff.Status != events.AgentResultStatusCompleted || handoff.Reused {
		return m, nil
	}
	targetSessionID := handoff.ParentSessionID
	if targetSessionID == "" {
		targetSessionID = m.sessionID
	}

	m.busy = true
	m.holdOpen = true
	return m, reuseDelegatedResultCmd(m.ctx, m.controller, targetSessionID, handoff.HandoffID)
}
