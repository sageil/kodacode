package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

type scopedToolCallKey struct {
	SessionID string
	TurnID    string
	CallID    string
}

type inspectorHandoffTarget struct {
	SessionID string
	TurnID    string
	HandoffID string
}

type inspectorToolTarget struct {
	SessionID string
	Ref       sessionToolCallRef
}

func scopedToolKey(sessionID string, ref sessionToolCallRef) scopedToolCallKey {
	return scopedToolCallKey{
		SessionID: strings.TrimSpace(sessionID),
		TurnID:    strings.TrimSpace(ref.TurnID),
		CallID:    strings.TrimSpace(ref.CallID),
	}
}

func normalizeToolTargetSessionID(currentSessionID, sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		return sessionID
	}
	return strings.TrimSpace(currentSessionID)
}

func selectedToolSessionID(m Model) string {
	if sessionID := strings.TrimSpace(m.selection.callSessionID); sessionID != "" {
		return normalizeToolTargetSessionID(m.sessionID, sessionID)
	}
	if handoff := selectedDelegatedHandoff(m.projector.Snapshot(), m); handoff != nil {
		return normalizeToolTargetSessionID(m.sessionID, handoff.ChildSessionID)
	}
	return strings.TrimSpace(m.sessionID)
}

func selectedToolMatchesSession(m Model, sessionID string, ref sessionToolCallRef) bool {
	return normalizeToolTargetSessionID(m.sessionID, sessionID) == selectedToolSessionID(m) &&
		strings.TrimSpace(m.selection.callTurnID) == strings.TrimSpace(ref.TurnID) &&
		strings.TrimSpace(m.selection.callID) == strings.TrimSpace(ref.CallID)
}

func selectedDelegatedHandoff(state events.SessionState, m Model) *events.AgentHandoffState {
	return explicitSelectedHandoff(currentTurn(state, m.turnID), strings.TrimSpace(m.selection.handoffID))
}

func activeDelegatedHandoff(state events.SessionState, m Model) *events.AgentHandoffState {
	turn := currentTurn(state, m.turnID)
	if turn == nil {
		return nil
	}
	for idx := len(turn.HandoffOrder) - 1; idx >= 0; idx-- {
		handoff := turn.Handoffs[turn.HandoffOrder[idx]]
		if handoff == nil || !handoff.PreviewActive {
			continue
		}
		if strings.TrimSpace(handoff.ChildSessionID) == "" || strings.TrimSpace(handoff.ChildTurnID) == "" {
			continue
		}
		return handoff
	}
	return nil
}

func (m Model) delegatedSnapshot(sessionID string) (events.SessionState, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return events.SessionState{}, false
	}
	state, ok := m.delegatedSnapshots.snapshots[sessionID]
	return state, ok
}

func (m Model) activeDelegatedSessionState(state events.SessionState) (events.SessionState, *events.AgentHandoffState, bool) {
	handoff := activeDelegatedHandoff(state, m)
	if handoff == nil {
		return events.SessionState{}, nil, false
	}
	childState, ok := m.delegatedSnapshot(handoff.ChildSessionID)
	if !ok {
		return events.SessionState{}, handoff, false
	}
	return childState, handoff, true
}

func (m Model) selectedDelegatedSessionState(state events.SessionState) (events.SessionState, *events.AgentHandoffState, bool) {
	handoff := selectedDelegatedHandoff(state, m)
	if handoff == nil {
		return events.SessionState{}, nil, false
	}
	childState, ok := m.delegatedSnapshot(handoff.ChildSessionID)
	if !ok {
		return events.SessionState{}, handoff, false
	}
	return childState, handoff, true
}

func delegateHandoffForCall(turn *events.TurnState, call *events.ToolCallState) *events.AgentHandoffState {
	if turn == nil || !isDelegateToolCall(call) {
		return nil
	}
	if handoffID := strings.TrimSpace(call.HandoffID); handoffID != "" {
		if handoff := turn.Handoffs[handoffID]; handoff != nil {
			return handoff
		}
	}
	for _, handoffID := range orderedHandoffIDs(turn) {
		handoff := turn.Handoffs[handoffID]
		if handoff == nil {
			continue
		}
		if strings.TrimSpace(handoff.ToolCallID) == strings.TrimSpace(call.CallID) {
			return handoff
		}
	}
	return nil
}

func orderedDelegatedSessionIDs(state events.SessionState) []string {
	seen := map[string]struct{}{}
	sessionIDs := make([]string, 0, len(state.TurnOrder))
	for _, turnID := range orderedSessionTurnIDs(state) {
		turn := state.Turns[turnID]
		for _, handoffID := range orderedHandoffIDs(turn) {
			handoff := turn.Handoffs[handoffID]
			if handoff == nil {
				continue
			}
			sessionID := strings.TrimSpace(handoff.ChildSessionID)
			if sessionID == "" {
				continue
			}
			if _, ok := seen[sessionID]; ok {
				continue
			}
			seen[sessionID] = struct{}{}
			sessionIDs = append(sessionIDs, sessionID)
		}
	}
	return sessionIDs
}

func (m *Model) ensureDelegatedSessionSnapshotLoadedCmd(sessionID string) tea.Cmd {
	if m == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if _, ok := m.delegatedSnapshots.snapshots[sessionID]; ok {
		return nil
	}
	if m.delegatedSnapshots.loading[sessionID] {
		return nil
	}
	m.delegatedSnapshots.loading[sessionID] = true
	return refreshSessionSnapshotCmd(m.ctx, m.controller, sessionID)
}

func (m *Model) refreshDelegatedSessionSnapshotCmd(sessionID string) tea.Cmd {
	if m == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if m.delegatedSnapshots.loading[sessionID] {
		if m.delegatedSnapshots.pending == nil {
			m.delegatedSnapshots.pending = make(map[string]bool)
		}
		m.delegatedSnapshots.pending[sessionID] = true
		return nil
	}
	m.delegatedSnapshots.loading[sessionID] = true
	return refreshSessionSnapshotCmd(m.ctx, m.controller, sessionID)
}

func (m *Model) consumePendingDelegatedSessionSnapshotRefreshCmd(sessionID string) tea.Cmd {
	if m == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || m.delegatedSnapshots.pending == nil || !m.delegatedSnapshots.pending[sessionID] {
		return nil
	}
	delete(m.delegatedSnapshots.pending, sessionID)
	return m.refreshDelegatedSessionSnapshotCmd(sessionID)
}

func (m *Model) ensureSelectedDelegatedSessionSnapshotLoadedCmd() tea.Cmd {
	if m == nil {
		return nil
	}
	handoff := selectedDelegatedHandoff(m.projector.Snapshot(), *m)
	if handoff == nil {
		return nil
	}
	return m.ensureDelegatedSessionSnapshotLoadedCmd(handoff.ChildSessionID)
}

func (m Model) relevantDelegatedSessionSnapshotIDs(state events.SessionState) []string {
	seen := make(map[string]struct{})
	sessionIDs := make([]string, 0, 4)
	appendSessionID := func(sessionID string) {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" || sessionID == strings.TrimSpace(m.sessionID) {
			return
		}
		if _, ok := seen[sessionID]; ok {
			return
		}
		seen[sessionID] = struct{}{}
		sessionIDs = append(sessionIDs, sessionID)
	}

	layout := resolveShellLayout(m, state)
	if layout.showInspector && effectiveInspectorTab(m) == inspectorTabTools {
		for _, sessionID := range orderedDelegatedSessionIDs(state) {
			appendSessionID(sessionID)
		}
	}

	appendSessionID(m.selection.callSessionID)

	if dialog, ok := m.dialog.(*toolDetailDialog); ok {
		appendSessionID(dialog.sessionID)
	}
	if dialog, ok := m.dialog.(*handoffDetailDialog); ok {
		appendSessionID(dialog.sessionID)
	}

	return sessionIDs
}

func (m *Model) ensureRelevantDelegatedSessionSnapshotsLoadedCmd(state events.SessionState) tea.Cmd {
	if m == nil {
		return nil
	}
	sessionIDs := m.relevantDelegatedSessionSnapshotIDs(state)
	if len(sessionIDs) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if cmd := m.ensureDelegatedSessionSnapshotLoadedCmd(sessionID); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func handoffByID(state events.SessionState, handoffID string) *events.AgentHandoffState {
	handoffID = strings.TrimSpace(handoffID)
	if handoffID == "" {
		return nil
	}
	for _, turnID := range orderedSessionTurnIDs(state) {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		if handoff := turn.Handoffs[handoffID]; handoff != nil {
			return handoff
		}
	}
	return nil
}

func delegatedPreviewNeedsSnapshotRefresh(state events.SessionState, payload events.AgentHandoffPreviewPayload) bool {
	handoff := handoffByID(state, payload.HandoffID)
	if handoff == nil {
		return true
	}
	if handoff.PreviewActive != payload.Active {
		return true
	}
	if strings.TrimSpace(handoff.PreviewToolName) != strings.TrimSpace(payload.ToolName) {
		return true
	}
	if strings.TrimSpace(handoff.PreviewAction) != strings.TrimSpace(payload.Action) {
		return true
	}
	return false
}

func delegatedSessionSnapshotRefreshID(state events.SessionState, event events.Event) string {
	switch payload := event.Payload.(type) {
	case events.AgentHandoffPayload:
		return strings.TrimSpace(payload.ChildSessionID)
	case events.AgentHandoffPreviewPayload:
		if !delegatedPreviewNeedsSnapshotRefresh(state, payload) {
			return ""
		}
		return strings.TrimSpace(payload.ChildSessionID)
	case events.AgentResultPayload:
		return strings.TrimSpace(payload.ChildSessionID)
	case events.AgentResultReusedPayload:
		return strings.TrimSpace(payload.ChildSessionID)
	default:
		return ""
	}
}

func (m *Model) refreshRelevantDelegatedSessionSnapshotsCmd(state events.SessionState, sessionIDs []string) tea.Cmd {
	if m == nil || len(sessionIDs) == 0 {
		return nil
	}
	relevant := m.relevantDelegatedSessionSnapshotIDs(state)
	if len(relevant) == 0 {
		return nil
	}
	needed := make(map[string]struct{}, len(relevant))
	for _, sessionID := range relevant {
		needed[strings.TrimSpace(sessionID)] = struct{}{}
	}
	cmds := make([]tea.Cmd, 0, len(sessionIDs))
	seen := make(map[string]struct{}, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			continue
		}
		if _, ok := needed[sessionID]; !ok {
			continue
		}
		if _, ok := seen[sessionID]; ok {
			continue
		}
		seen[sessionID] = struct{}{}
		if cmd := m.refreshDelegatedSessionSnapshotCmd(sessionID); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}
