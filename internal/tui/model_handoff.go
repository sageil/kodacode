package tui

import "github.com/sageil/kodacode/internal/events"

func pendingDelegatedPermissionHandoff(turn *events.TurnState) *events.AgentHandoffState {
	return pendingDelegatedHandoffWithStatus(turn, events.AgentResultStatusPendingPermission)
}

func pendingDelegatedQuestionHandoff(turn *events.TurnState) *events.AgentHandoffState {
	return pendingDelegatedHandoffWithStatus(turn, events.AgentResultStatusPendingQuestion)
}

func pendingDelegatedHandoffWithStatus(turn *events.TurnState, status events.AgentResultStatus) *events.AgentHandoffState {
	if turn == nil {
		return nil
	}
	for idx := len(turn.HandoffOrder) - 1; idx >= 0; idx-- {
		handoff := turn.Handoffs[turn.HandoffOrder[idx]]
		if handoff != nil && handoff.Status == status {
			return handoff
		}
	}
	return nil
}

func featuredHandoff(turn *events.TurnState) *events.AgentHandoffState {
	if turn == nil {
		return nil
	}
	for idx := len(turn.HandoffOrder) - 1; idx >= 0; idx-- {
		handoff := turn.Handoffs[turn.HandoffOrder[idx]]
		if handoff != nil {
			return handoff
		}
	}
	return nil
}

func (m *Model) syncHandoffSelectionState() {
	turn := inspectorDetailTurn(m.projector.Snapshot(), *m)
	if turn == nil || len(turn.HandoffOrder) == 0 {
		m.selection.handoffID = ""
		return
	}
	if m.selection.handoffID == "" {
		return
	}
	if turn.Handoffs[m.selection.handoffID] != nil {
		return
	}
	m.selection.handoffID = ""
}
