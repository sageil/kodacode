package tui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

func (m Model) handleTranscriptInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.chrome.focus != focusTranscript {
		return m, nil
	}

	if m.transcriptView.visualActive {
		switch msg.String() {
		case "esc", "v":
			m.clearTranscriptVisualSelection()
			return m, nil
		case "y":
			cmd := m.copyTranscriptVisualSelectionCmd()
			m.clearTranscriptVisualSelection()
			return m, cmd
		case "left", "h":
			m.moveTranscriptCursorHorizontal(-1)
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		case "right", "l":
			m.moveTranscriptCursorHorizontal(1)
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		case "up", "k":
			m.moveTranscriptCursor(-1)
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		case "down", "j":
			m.moveTranscriptCursor(1)
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		case "pgdown", "ctrl+d":
			m.pageTranscriptCursor(1)
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		case "pgup", "ctrl+u":
			m.pageTranscriptCursor(-1)
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		case "home", "g":
			m.moveTranscriptCursorToStart()
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		case "end", "G":
			m.moveTranscriptCursorToEnd()
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		case "0":
			m.moveTranscriptCursorToLineStart()
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		case "$":
			m.moveTranscriptCursorToLineEnd()
			m.syncDeferredTranscriptIfNeeded()
			return m, nil
		default:
			return m, nil
		}
	}

	switch msg.String() {
	case "left", "h":
		m.moveTranscriptCursorHorizontal(-1)
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "right", "l":
		m.moveTranscriptCursorHorizontal(1)
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "up":
		m.moveTranscriptCursor(-1)
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "down":
		m.moveTranscriptCursor(1)
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "pgdown", "ctrl+d":
		m.pageTranscriptCursor(1)
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "pgup", "ctrl+u":
		m.pageTranscriptCursor(-1)
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "home":
		m.moveTranscriptCursorToStart()
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "end":
		m.moveTranscriptCursorToEnd()
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "0":
		m.moveTranscriptCursorToLineStart()
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "$":
		m.moveTranscriptCursorToLineEnd()
		m.syncDeferredTranscriptIfNeeded()
		return m, nil
	case "enter":
		if shellLayoutEnabled(m) && m.shellToolCallsVisible {
			if cmd := m.openSelectedTranscriptToolDialog(); cmd != nil {
				return m, cmd
			}
		}
		return m.startChildSessionView()
	case "r":
		return m.reuseSelectedHandoffResult()
	case "backspace":
		return m.returnToParentSessionView()
	case "k":
		return m, m.moveSelectedTool(-1)
	case "j":
		return m, m.moveSelectedTool(1)
	case "{":
		m.moveSelectedHandoff(-1)
		return m, m.ensureSelectedDelegatedSessionSnapshotLoadedCmd()
	case "]":
		m.moveSelectedHandoff(1)
		return m, m.ensureSelectedDelegatedSessionSnapshotLoadedCmd()
	case "v":
		m.startTranscriptVisualSelection()
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) moveSelectedTool(delta int) tea.Cmd {
	state := m.projector.Snapshot()
	return m.moveSelectedToolAcross(visibleToolSelectionRefs(*m, state), delta, true)
}

func (m *Model) moveSelectedInspectorTool(delta int) tea.Cmd {
	state := m.projector.Snapshot()
	targets := inspectorVisibleToolTargets(*m, state)
	if len(targets) == 0 {
		return nil
	}
	selected := inspectorToolTarget{
		SessionID: selectedToolSessionID(*m),
		Ref: sessionToolCallRef{
			TurnID: strings.TrimSpace(m.selection.callTurnID),
			CallID: strings.TrimSpace(m.selection.callID),
		},
	}
	index := indexOfInspectorToolTarget(targets, selected)
	switch {
	case index < 0 && delta < 0:
		index = len(targets) - 1
	case index < 0:
		index = 0
	default:
		index = max(min(index+delta, len(targets)-1), 0)
	}
	return m.selectInspectorToolTarget(targets[index])
}

func (m *Model) openSelectedInspectorToolDialog() tea.Cmd {
	state := m.projector.Snapshot()
	target, ok := inspectorActiveToolTarget(*m, state)
	if !ok {
		return nil
	}
	return tea.Batch(m.selectInspectorToolTarget(target), m.openInspectorToolTargetDialog(target))
}

func (m *Model) openSelectedTranscriptToolDialog() tea.Cmd {
	if m == nil {
		return nil
	}
	state := m.projector.Snapshot()
	sessionID, ref, _, _, ok := selectedSessionToolCall(state, *m)
	if !ok {
		return nil
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = m.sessionID
	}
	if strings.TrimSpace(sessionID) == strings.TrimSpace(m.sessionID) {
		return m.openToolCallDialog(ref)
	}
	childState, ok := m.delegatedSnapshot(sessionID)
	if !ok {
		return m.ensureDelegatedSessionSnapshotLoadedCmd(sessionID)
	}
	return m.openToolCallDialogForSession(sessionID, childState, ref)
}

func (m *Model) moveSelectedToolAcross(refs []sessionToolCallRef, delta int, clearOnEmpty bool) tea.Cmd {
	if len(refs) == 0 {
		if clearOnEmpty {
			m.clearSelectedToolCall()
		}
		return nil
	}

	selected := sessionToolCallRef{
		TurnID: strings.TrimSpace(m.selection.callTurnID),
		CallID: strings.TrimSpace(m.selection.callID),
	}
	index := indexOfToolCallRef(refs, selected)
	switch {
	case index < 0 && delta < 0:
		index = len(refs) - 1
	case index < 0:
		index = 0
	default:
		index = max(min(index+delta, len(refs)-1), 0)
	}
	return m.selectToolCall(refs[index])
}

func (m *Model) selectToolCall(ref sessionToolCallRef) tea.Cmd {
	ref = sessionToolCallRef{
		TurnID: strings.TrimSpace(ref.TurnID),
		CallID: strings.TrimSpace(ref.CallID),
	}
	if ref.TurnID == "" || ref.CallID == "" {
		m.clearSelectedToolCall()
		return nil
	}
	if strings.TrimSpace(m.selection.callTurnID) == ref.TurnID &&
		strings.TrimSpace(m.selection.callID) == ref.CallID &&
		selectedToolSessionID(*m) == strings.TrimSpace(m.sessionID) &&
		strings.TrimSpace(m.selection.handoffID) == "" {
		return m.ensureSelectedToolResultLoadedCmd()
	}
	oldSelectedCallTurnID := strings.TrimSpace(m.selection.callTurnID)
	selectedHandoffTurnID := ""
	if strings.TrimSpace(m.selection.handoffID) != "" {
		selectedHandoffTurnID = strings.TrimSpace(m.turnID)
	}
	m.selection.callSessionID = strings.TrimSpace(m.sessionID)
	m.selection.callTurnID = ref.TurnID
	m.selection.callID = ref.CallID
	m.selection.handoffID = ""
	_ = m.applyTranscriptRefreshPlan(transcriptTurnRefreshPlan(oldSelectedCallTurnID, ref.TurnID, selectedHandoffTurnID))
	m.jumpTranscriptToSelectedTool()
	m.syncInspectorBody(true)
	return m.ensureSelectedToolResultLoadedCmd()
}

func (m *Model) openToolCallDialog(ref sessionToolCallRef) tea.Cmd {
	return m.openToolCallDialogForSession(m.sessionID, m.projector.Snapshot(), ref)
}

func (m *Model) openToolCallDialogForSession(sessionID string, state events.SessionState, ref sessionToolCallRef) tea.Cmd {
	_, call := sessionToolCall(state, ref)
	if call == nil {
		return nil
	}
	loadResultCmd := m.ensureToolResultLoadedForSessionCmd(sessionID, ref, call)
	m.retainToolMutationDetailForRef(sessionID, ref)
	dialog := newToolDetailDialogForSession(*m, sessionID, state, ref, call)
	width, height := dialogRenderSize(*m, state)
	dialog.SetFrame(width, height)
	m.dialog = dialog
	return tea.Batch(
		loadResultCmd,
		m.ensureToolMutationDetailLoadedForSessionCmd(sessionID, ref, call),
	)
}

func (m *Model) syncToolDetailDialog() {
	if m == nil {
		return
	}
	dialog, ok := m.dialog.(*toolDetailDialog)
	if !ok {
		return
	}
	state := m.projector.Snapshot()
	if strings.TrimSpace(dialog.sessionID) != "" && strings.TrimSpace(dialog.sessionID) != strings.TrimSpace(m.sessionID) {
		delegatedState, ok := m.delegatedSnapshot(dialog.sessionID)
		if !ok {
			return
		}
		state = delegatedState
	}
	_, call := sessionToolCall(state, dialog.ref)
	if call == nil {
		m.dialog = nil
		return
	}
	dialog.ApplyTheme(m.theme)
	dialog.SetFrame(dialogRenderSize(*m, state))
	dialog.Sync(*m, dialog.sessionID, state, dialog.ref, call)
}

func (m *Model) ensureOpenToolMutationDetailLoadedCmd(state events.SessionState) tea.Cmd {
	if m == nil {
		return nil
	}
	dialog, ok := m.dialog.(*toolDetailDialog)
	if !ok {
		return nil
	}
	if strings.TrimSpace(dialog.sessionID) != "" && strings.TrimSpace(dialog.sessionID) != strings.TrimSpace(m.sessionID) {
		delegatedState, ok := m.delegatedSnapshot(dialog.sessionID)
		if !ok {
			return nil
		}
		state = delegatedState
	}
	_, call := sessionToolCall(state, dialog.ref)
	return m.ensureToolMutationDetailLoadedForSessionCmd(dialog.sessionID, dialog.ref, call)
}

func (m *Model) clearSelectedToolCall() {
	if strings.TrimSpace(m.selection.callSessionID) == "" &&
		strings.TrimSpace(m.selection.callTurnID) == "" &&
		strings.TrimSpace(m.selection.callID) == "" {
		return
	}
	oldSelectedCallTurnID := strings.TrimSpace(m.selection.callTurnID)
	m.selection.callSessionID = ""
	m.selection.callTurnID = ""
	m.selection.callID = ""
	_ = m.applyTranscriptRefreshPlan(transcriptTurnRefreshPlan(oldSelectedCallTurnID))
	m.messages.GotoBottom()
	m.syncInspectorBody(true)
}

func (m *Model) jumpTranscriptToSelectedTool() {
	if m == nil {
		return
	}
	ref := sessionToolCallRef{
		TurnID: strings.TrimSpace(m.selection.callTurnID),
		CallID: strings.TrimSpace(m.selection.callID),
	}
	if ref.TurnID == "" || ref.CallID == "" {
		return
	}
	if line, ok := m.transcriptView.toolLines[ref]; ok {
		m.messages.GotoLine(line)
		m.transcriptView.cursorLine = line
		m.transcriptView.cursorColumn = 0
		m.transcriptView.cursorGoalColumn = 0
		m.transcriptView.cursorInitialized = true
	}
}

func (m *Model) syncSelectionStateWithState(state events.SessionState) {
	if strings.TrimSpace(m.selection.callTurnID) == "" || strings.TrimSpace(m.selection.callID) == "" {
		m.selection.callSessionID = ""
		m.selection.callTurnID = ""
		m.selection.callID = ""
	} else if selectedSessionID := selectedToolSessionID(*m); selectedSessionID != strings.TrimSpace(m.sessionID) {
		if childState, ok := m.delegatedSnapshot(selectedSessionID); ok {
			selected := sessionToolCallRef{
				TurnID: strings.TrimSpace(m.selection.callTurnID),
				CallID: strings.TrimSpace(m.selection.callID),
			}
			if _, call := sessionToolCall(childState, selected); call == nil {
				m.selection.callSessionID = ""
				m.selection.callTurnID = ""
				m.selection.callID = ""
			}
		}
	} else if turn := state.Turns[m.selection.callTurnID]; turn == nil || turn.ToolCalls[m.selection.callID] == nil {
		m.selection.callSessionID = ""
		m.selection.callTurnID = ""
		m.selection.callID = ""
	} else if indexOfToolCallRef(orderedSessionToolCallRefs(state), sessionToolCallRef{
		TurnID: strings.TrimSpace(m.selection.callTurnID),
		CallID: strings.TrimSpace(m.selection.callID),
	}) < 0 {
		m.selection.callSessionID = ""
		m.selection.callTurnID = ""
		m.selection.callID = ""
	}
	syncTaskSelectionStateWithState(m, state)
	m.syncHandoffSelectionState()
}

func indexOfString(values []string, needle string) int {
	for idx, value := range values {
		if value == needle {
			return idx
		}
	}
	return -1
}

func indexOfToolCallRef(values []sessionToolCallRef, needle sessionToolCallRef) int {
	for idx, value := range values {
		if value.TurnID == needle.TurnID && value.CallID == needle.CallID {
			return idx
		}
	}
	return -1
}

func indexOfInspectorToolTarget(values []inspectorToolTarget, needle inspectorToolTarget) int {
	needleSessionID := strings.TrimSpace(needle.SessionID)
	for idx, value := range values {
		if strings.TrimSpace(value.SessionID) == needleSessionID && value.Ref == needle.Ref {
			return idx
		}
	}
	return -1
}

func inspectorActiveToolTarget(m Model, state events.SessionState) (inspectorToolTarget, bool) {
	targets := inspectorVisibleToolTargets(m, state)
	if len(targets) == 0 {
		return inspectorToolTarget{}, false
	}
	selected := inspectorToolTarget{
		SessionID: selectedToolSessionID(m),
		Ref: sessionToolCallRef{
			TurnID: strings.TrimSpace(m.selection.callTurnID),
			CallID: strings.TrimSpace(m.selection.callID),
		},
	}
	for _, target := range targets {
		if strings.TrimSpace(target.SessionID) == strings.TrimSpace(selected.SessionID) && target.Ref == selected.Ref {
			return target, true
		}
	}
	return targets[0], true
}

func inspectorVisibleToolTargets(m Model, state events.SessionState) []inspectorToolTarget {
	if len(m.inspector.toolLines) > 0 {
		lines := make([]int, 0, len(m.inspector.toolLines))
		for line := range m.inspector.toolLines {
			lines = append(lines, line)
		}
		sort.Ints(lines)

		targets := make([]inspectorToolTarget, 0, len(lines))
		var last inspectorToolTarget
		hasLast := false
		for _, line := range lines {
			action := m.inspector.toolLines[line]
			if action.Kind != inspectorToolLineOpenCall {
				continue
			}
			if strings.TrimSpace(action.Target.Ref.TurnID) == "" || strings.TrimSpace(action.Target.Ref.CallID) == "" {
				continue
			}
			if hasLast &&
				strings.TrimSpace(last.SessionID) == strings.TrimSpace(action.Target.SessionID) &&
				last.Ref == action.Target.Ref {
				continue
			}
			targets = append(targets, action.Target)
			last = action.Target
			hasLast = true
		}
		return targets
	}
	if isWideShell(m) {
		return nil
	}
	refs := orderedSessionToolCallRefs(state)
	targets := make([]inspectorToolTarget, 0, len(refs))
	for _, ref := range refs {
		targets = append(targets, inspectorToolTarget{SessionID: state.SessionID, Ref: ref})
	}
	return targets
}

func inspectorVisibleToolRefs(m Model, state events.SessionState) []sessionToolCallRef {
	targets := inspectorVisibleToolTargets(m, state)
	refs := make([]sessionToolCallRef, 0, len(targets))
	for _, target := range targets {
		refs = append(refs, target.Ref)
	}
	return refs
}

func (m *Model) selectInspectorToolTarget(target inspectorToolTarget) tea.Cmd {
	sessionID := normalizeToolTargetSessionID(m.sessionID, target.SessionID)
	if sessionID == strings.TrimSpace(m.sessionID) && strings.TrimSpace(m.selection.handoffID) == "" {
		return m.selectToolCall(target.Ref)
	}
	if selectedToolMatchesSession(*m, sessionID, target.Ref) {
		return m.ensureSelectedToolResultLoadedCmd()
	}
	m.selection.callSessionID = sessionID
	m.selection.callTurnID = strings.TrimSpace(target.Ref.TurnID)
	m.selection.callID = strings.TrimSpace(target.Ref.CallID)
	m.syncInspectorBody(false)
	return m.ensureSelectedToolResultLoadedCmd()
}

func (m *Model) openInspectorToolTargetDialog(target inspectorToolTarget) tea.Cmd {
	sessionID := normalizeToolTargetSessionID(m.sessionID, target.SessionID)
	if sessionID == strings.TrimSpace(m.sessionID) && strings.TrimSpace(m.selection.handoffID) == "" {
		return m.openToolCallDialog(target.Ref)
	}
	state, ok := m.delegatedSnapshot(sessionID)
	if !ok {
		return m.ensureDelegatedSessionSnapshotLoadedCmd(sessionID)
	}
	return m.openToolCallDialogForSession(sessionID, state, target.Ref)
}
