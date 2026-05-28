package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

func (m Model) handoffSessionState(sessionID string) (events.SessionState, bool) {
	sessionID = normalizeToolTargetSessionID(m.sessionID, sessionID)
	if sessionID == strings.TrimSpace(m.sessionID) {
		return m.projector.Snapshot(), true
	}
	return m.delegatedSnapshot(sessionID)
}

func sessionHandoff(state events.SessionState, target inspectorHandoffTarget) (*events.TurnState, *events.AgentHandoffState) {
	turnID := strings.TrimSpace(target.TurnID)
	handoffID := strings.TrimSpace(target.HandoffID)
	if turnID == "" || handoffID == "" {
		return nil, nil
	}
	turn := state.Turns[turnID]
	if turn == nil {
		return nil, nil
	}
	return turn, turn.Handoffs[handoffID]
}

func (m *Model) openHandoffDetailDialog(target inspectorHandoffTarget) tea.Cmd {
	if m == nil {
		return nil
	}
	sessionID := normalizeToolTargetSessionID(m.sessionID, target.SessionID)
	state, ok := m.handoffSessionState(sessionID)
	if !ok {
		return m.ensureDelegatedSessionSnapshotLoadedCmd(sessionID)
	}
	return m.openHandoffDetailDialogForSession(sessionID, state, target)
}

func (m *Model) openHandoffDetailDialogForSession(sessionID string, state events.SessionState, target inspectorHandoffTarget) tea.Cmd {
	if m == nil {
		return nil
	}
	target.SessionID = normalizeToolTargetSessionID(m.sessionID, sessionID)
	_, handoff := sessionHandoff(state, target)
	if handoff == nil {
		return nil
	}
	dialog := newHandoffDetailDialog(*m, sessionID, state, target, handoff)
	width, height := dialogRenderSize(*m, state)
	dialog.SetFrame(width, height)
	m.openDialog(dialog)
	return nil
}

func (m *Model) syncHandoffDetailDialog() {
	if m == nil {
		return
	}
	dialog, ok := m.dialog.(*handoffDetailDialog)
	if !ok {
		return
	}
	state, ok := m.handoffSessionState(dialog.sessionID)
	if !ok {
		return
	}
	_, handoff := sessionHandoff(state, dialog.target)
	if handoff == nil {
		m.dialog = nil
		return
	}
	dialog.ApplyTheme(m.theme)
	dialog.SetFrame(dialogRenderSize(*m, state))
	dialog.Sync(*m, dialog.sessionID, state, dialog.target, handoff)
}
