package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

type focusRegion string

const (
	focusTranscript focusRegion = "transcript"
	focusInspector  focusRegion = "inspector"
	focusComposer   focusRegion = "composer"
)

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.beginShutdown()
		return m, tea.Quit
	}
	if m.dialog != nil {
		updated, cmd := m.dialog.Update(msg)
		m.dialog = updated
		return m, tea.Batch(cmd, m.syncDeferredDialogIfNeeded())
	}

	if updated, cmd, handled := m.handleTurnCancelShortcut(msg); handled {
		return updated, cmd
	}

	if updated, cmd, handled := m.handleAgentCycleShortcut(msg); handled {
		return updated, cmd
	}

	if updated, cmd, handled := m.handleQuestionInput(msg); handled {
		return updated, cmd
	}

	if updated, cmd, handled := m.handleInlinePermissionInput(msg); handled {
		return updated, cmd
	}

	if isTranscriptFocusShortcut(msg) && m.chrome.focus != focusTranscript {
		return m.enterNormalMode()
	}

	if isDrawerToggleShortcut(msg) {
		return m.toggleDrawerVisibility()
	}

	if isNewSessionShortcut(msg) {
		return m.startNewWorkspaceSession(m.chrome.focus == focusComposer, m.chrome.focus == focusComposer)
	}

	if isWorkflowDialogShortcut(msg) {
		if m.busy || m.hasPendingInteraction() {
			return m, nil
		}
		return m, m.openWorkflowDialog()
	}

	if isLayoutToggleShortcut(msg) {
		return m.toggleLayoutMode()
	}

	if updated, cmd, handled := m.handleShellToolsShortcut(msg); handled {
		return updated, cmd
	}

	if m.chrome.focus == focusComposer {
		return m.handleComposerInput(msg)
	}

	switch msg.String() {
	case "?":
		m.chrome.hintsExpanded = !m.chrome.hintsExpanded
		return m, nil
	case "i":
		return m.enterInsertMode()
	case "/":
		if !m.hasPendingInteraction() {
			return m, m.openCommandPalette()
		}
		return m, nil
	case "ctrl+p":
		return m, m.openCommandPalette()
	default:
		if updated, cmd, handled := m.handlePaneFocusShortcut(msg); handled {
			return updated, cmd
		}
		if m.chrome.focus == focusInspector {
			activeTab := effectiveInspectorTab(m)
			switch msg.String() {
			case "h", "left":
				nextTab := stepInspectorTab(m, -1)
				if m.inspector.tab != nextTab || activeTab != nextTab {
					m.inspector.tab = nextTab
					m.syncInspectorBody(true)
				}
				return m, nil
			case "l", "right":
				nextTab := stepInspectorTab(m, 1)
				if m.inspector.tab != nextTab || activeTab != nextTab {
					m.inspector.tab = nextTab
					m.syncInspectorBody(true)
				}
				return m, nil
			case "k":
				if activeTab == inspectorTabTools {
					return m, m.moveSelectedInspectorTool(-1)
				}
				if activeTab == inspectorTabTasks {
					return m, m.moveSelectedInspectorTask(-1)
				}
			case "j":
				if activeTab == inspectorTabTools {
					return m, m.moveSelectedInspectorTool(1)
				}
				if activeTab == inspectorTabTasks {
					return m, m.moveSelectedInspectorTask(1)
				}
			case "enter":
				if activeTab == inspectorTabTools {
					return m, m.openSelectedInspectorToolDialog()
				}
				if activeTab == inspectorTabTasks {
					return m, m.openSelectedInspectorTaskDialog()
				}
			case "up":
				m.inspector.body.ScrollUp(1)
				return m, nil
			case "down":
				m.inspector.body.ScrollDown(1)
				return m, nil
			case "pgdown", "J":
				m.inspector.body.PageDown()
				return m, nil
			case "pgup", "K":
				m.inspector.body.PageUp()
				return m, nil
			case "home":
				m.inspector.body.GotoTop()
				return m, nil
			case "end":
				m.inspector.body.GotoBottom()
				return m, nil
			}
		}
		return m.handleTranscriptInput(msg)
	}
}

func isTranscriptFocusShortcut(msg tea.KeyPressMsg) bool {
	switch msg.Keystroke() {
	case "ctrl+]", "esc":
		return true
	default:
		return false
	}
}

func isDrawerToggleShortcut(msg tea.KeyPressMsg) bool {
	return msg.Keystroke() == "ctrl+\\"
}

func isNewSessionShortcut(msg tea.KeyPressMsg) bool {
	return msg.Keystroke() == "ctrl+n"
}

func isWorkflowDialogShortcut(msg tea.KeyPressMsg) bool {
	return msg.Keystroke() == "ctrl+w"
}

func isLayoutToggleShortcut(msg tea.KeyPressMsg) bool {
	return msg.Keystroke() == "ctrl+l"
}

func (m Model) toggleLayoutMode() (tea.Model, tea.Cmd) {
	label := "Shell layout"
	if shellLayoutEnabled(m) {
		m.layout = tuiLayoutClassic
		label = "Classic layout"
	} else {
		m.layout = tuiLayoutShell
		if m.chrome.focus == focusInspector {
			m.chrome.focus = focusTranscript
		}
	}
	m.syncViewportLayout()
	return m, tea.Batch(
		m.syncComposerFocus(),
		m.showFooterActivity(label, footerActivityToneInfo, ""),
		persistTUILayoutCmd(m.ctx, m.backend, m.layout),
	)
}

func (m Model) handleShellToolsShortcut(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !shellLayoutEnabled(m) {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+t":
		return m, m.openShellToolsDialog(), true
	case "t":
		if m.chrome.focus != focusTranscript || m.transcriptView.visualActive {
			return m, nil, false
		}
		return m, m.openShellToolsDialog(), true
	case "T":
		if m.chrome.focus == focusComposer {
			return m, nil, false
		}
		m.shellToolCallsVisible = !m.shellToolCallsVisible
		m.syncTranscriptStructureWithState(m.projector.Snapshot())
		if m.shellToolCallsVisible {
			return m, m.showFooterActivity("Tool calls shown in shell", footerActivityToneInfo, ""), true
		} else {
			return m, m.showFooterActivity("Tool calls hidden from shell", footerActivityToneInfo, ""), true
		}
	default:
		return m, nil, false
	}
}

func (m Model) handleTurnCancelShortcut(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.transcriptView.visualActive {
		return m, nil, false
	}
	if !m.turnCancellationAvailable() {
		return m, nil, false
	}
	if msg.String() != "esc" {
		return m, nil, false
	}
	if m.liveTurn.cancelRequested {
		return m, nil, true
	}
	m.liveTurn.cancelRequested = true
	return m, cancelTurnCmd(m.ctx, m.controller, m.sessionID, m.turnID), true
}

func (m Model) turnCancellationAvailable() bool {
	if strings.TrimSpace(m.sessionID) == "" || strings.TrimSpace(m.turnID) == "" {
		return false
	}
	if m.interaction.resolveReq != "" {
		return true
	}
	if turn := currentTurn(m.projector.Snapshot(), m.turnID); turn != nil {
		return turn.Status == events.TurnStatusRunning
	}
	return m.liveTurn.spinnerArmed
}

func (m Model) handlePaneFocusShortcut(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "1":
		m.chrome.focus = focusTranscript
	case "2":
		if isWideShell(m) {
			m.chrome.wideSidebarOpen = true
			m.chrome.focus = focusInspector
			m.chrome.inspectorOpen = true
		} else if resolveShellLayout(m, m.projector.Snapshot()).showInspector {
			m.chrome.focus = focusInspector
			m.chrome.inspectorOpen = true
		} else {
			m.chrome.focus = focusTranscript
		}
	case "3":
		updated, cmd := m.enterInsertMode()
		return updated, cmd, true
	case "4":
		updated, cmd := m.enterInsertMode()
		return updated, cmd, true
	default:
		return m, nil, false
	}
	m.syncViewportLayout()
	return m, m.syncComposerFocus(), true
}

func (m Model) handleAgentCycleShortcut(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	delta := 0
	switch msg.String() {
	case "tab":
		delta = 1
	case "shift+tab", "backtab":
		delta = -1
	default:
		return m, nil, false
	}
	if m.currentTurnRunning() {
		return m, nil, true
	}
	return m.cycleSelectedAgent(delta), nil, true
}

func (m Model) enterNormalMode() (tea.Model, tea.Cmd) {
	m.chrome.focus = focusTranscript
	m.dismissComposerPopup()
	m.syncViewportLayout()
	return m, m.syncComposerFocus()
}

func (m *Model) resetInspectorAgentSelectionToCurrentTurn() {
	if m == nil {
		return
	}
	oldSelectedCallTurnID := strings.TrimSpace(m.selection.callTurnID)
	m.selection.detailTurnID = m.turnID
	m.selection.callSessionID = ""
	m.selection.callTurnID = ""
	m.selection.callID = ""
	m.clearExpandedToolCall()
	_ = m.applyTranscriptRefreshPlan(transcriptTurnRefreshPlan(oldSelectedCallTurnID))
}

func (m Model) cycleSelectedAgent(delta int) tea.Model {
	agents, err := m.backend.ListAgents(m.ctx, m.workspace)
	if err != nil {
		m.setFooterError(err.Error())
		return m
	}
	m.clearFooterError()
	m.cacheAvailableAgents(agents)

	ids := orderedAvailableAgentIDs(agents)
	if len(ids) == 0 {
		return m
	}

	current := strings.TrimSpace(m.agentID)
	index := indexOfString(ids, current)
	switch {
	case index < 0 && delta < 0:
		index = len(ids) - 1
	case index < 0:
		index = 0
	default:
		index = (index + delta + len(ids)) % len(ids)
	}

	nextAgent := ids[index]
	if nextAgent == "" || nextAgent == current {
		return m
	}

	m.agentID = nextAgent
	m.resetInspectorAgentSelectionToCurrentTurn()
	m.syncInspectorTabAvailability()
	m.syncInspectorBody(true)
	return m
}

func orderedAvailableAgentIDs(agents []app.AvailableAgent) []string {
	ids := make([]string, 0, len(agents))
	seen := make(map[string]struct{}, len(agents))
	for _, agent := range agents {
		id := strings.TrimSpace(agent.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func visibleFocusRegions(m Model, state events.SessionState, layout shellLayout) []focusRegion {
	regions := []focusRegion{focusTranscript}
	if m.composerInputEnabledForState(state) {
		regions = append(regions, focusComposer)
	}
	if layout.showInspector {
		regions = append(regions, focusInspector)
	}
	return regions
}

func (m Model) enterInsertMode() (tea.Model, tea.Cmd) {
	if m.hasPendingInteraction() {
		m.chrome.focus = focusTranscript
		m.syncViewportLayout()
		return m, m.syncComposerFocus()
	}
	if !m.composerInputEnabled() {
		m.chrome.focus = focusTranscript
		m.syncViewportLayout()
		return m, tea.Batch(m.syncComposerFocus(), m.openComposerSetupDialog())
	}
	m.chrome.focus = focusComposer
	m.syncViewportLayout()
	return m, m.syncComposerFocus()
}

func (m Model) toggleDrawerVisibility() (tea.Model, tea.Cmd) {
	if m.hasPendingApproval() {
		m.chrome.inspectorOpen = true
		m.chrome.wideSidebarOpen = true
		m.chrome.focus = focusTranscript
		m.syncViewportLayout()
		return m, m.syncComposerFocus()
	}

	if !shellLayoutEnabled(m) && (m.chrome.wideSidebarOpen || m.chrome.inspectorOpen) {
		m.chrome.wideSidebarOpen = true
		m.chrome.inspectorOpen = true
		m.chrome.focus = focusInspector
		m.syncViewportLayout()
		return m, m.syncComposerFocus()
	}

	if m.chrome.wideSidebarOpen || m.chrome.inspectorOpen {
		m.chrome.wideSidebarOpen = false
		m.chrome.inspectorOpen = false
		if m.chrome.focus == focusInspector {
			m.chrome.focus = focusTranscript
		}
		m.syncViewportLayout()
		return m, m.syncComposerFocus()
	}

	m.chrome.wideSidebarOpen = true
	m.chrome.inspectorOpen = true
	m.chrome.focus = focusInspector
	m.syncViewportLayout()
	return m, m.syncComposerFocus()
}

func (m *Model) syncFocusState() {
	state := m.projector.CurrentState()
	m.syncSelectionStateWithState(state)
	if pendingExecutionFromState(state) != nil || pendingPermissionFromState(state) != nil {
		m.chrome.focus = focusTranscript
		m.syncViewportLayout()
		return
	}
	if pendingQuestionFromState(state) != nil {
		m.chrome.focus = focusTranscript
		m.syncViewportLayout()
		return
	}
	layout := resolveShellLayout(*m, state)
	for _, region := range visibleFocusRegions(*m, state, layout) {
		if region == m.chrome.focus {
			return
		}
	}
	if !m.busy {
		if m.composerInputEnabledForState(state) {
			m.chrome.focus = focusComposer
		} else {
			m.chrome.focus = focusTranscript
		}
		m.syncViewportLayout()
		return
	}
	m.chrome.focus = focusTranscript
	m.syncViewportLayout()
}
