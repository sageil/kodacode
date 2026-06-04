package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

func (m Model) shouldAutoQuit() bool {
	return !m.holdOpen
}

func (m Model) currentView() sessionView {
	return sessionView{
		SessionID:             m.sessionID,
		TurnID:                m.turnID,
		UserText:              m.userText,
		AgentID:               m.agentID,
		WorkflowID:            m.workflowID,
		SkillIDs:              append([]string(nil), m.skillIDs...),
		ThinkingEnabled:       m.thinkingEnabled,
		ReasoningVariant:      m.reasoningVariant,
		WorkspaceRoot:         m.workspace,
		DetailTurnID:          m.selection.detailTurnID,
		SelectedCallSessionID: m.selection.callSessionID,
		SelectedCallTurnID:    m.selection.callTurnID,
		SelectedCallID:        m.selection.callID,
		ExpandedCallSessionID: m.selection.expandedCallSessionID,
		ExpandedCallTurnID:    m.selection.expandedCallTurnID,
		ExpandedCallID:        m.selection.expandedCallID,
		SelectedTaskID:        m.selection.taskID,
		Focus:                 m.chrome.focus,
		InspectorOpen:         m.chrome.inspectorOpen,
		WideSidebarOpen:       m.chrome.wideSidebarOpen,
	}
}

func (m *Model) applyView(view sessionView, state events.SessionState, stateOwned bool, stream <-chan events.Event, cancel func(), watchID int) {
	nextWorkspace := resolvedWorkspaceRoot(state, view.WorkspaceRoot)
	m.sessionID = view.SessionID
	m.turnID = view.TurnID
	m.userText = resolvedUserText(state, view)
	m.agentID = resolvedAgentID(state, view.TurnID, view.AgentID)
	m.workflowID = resolvedWorkflowID(state, view.TurnID, view.WorkflowID)
	m.skillIDs = resolvedSkillIDs(state, view.TurnID, view.SkillIDs)
	m.thinkingEnabled = resolvedThinkingEnabled(state, view.TurnID, view.ThinkingEnabled)
	m.reasoningVariant = resolvedReasoningVariant(state, view.TurnID, view.ReasoningVariant)
	m.workspace = nextWorkspace
	if strings.TrimSpace(view.DetailTurnID) != "" {
		m.selection.detailTurnID = view.DetailTurnID
	} else {
		m.selection.detailTurnID = view.TurnID
	}
	m.selection.callSessionID = view.SelectedCallSessionID
	m.selection.callTurnID = view.SelectedCallTurnID
	m.selection.callID = view.SelectedCallID
	m.selection.expandedCallSessionID = view.ExpandedCallSessionID
	m.selection.expandedCallTurnID = view.ExpandedCallTurnID
	m.selection.expandedCallID = view.ExpandedCallID
	m.selection.taskID = view.SelectedTaskID
	m.chrome.focus = view.Focus
	m.chrome.inspectorOpen = view.InspectorOpen
	m.chrome.wideSidebarOpen = view.WideSidebarOpen
	m.syncInspectorTabAvailability()
	m.interaction.resolveReq = ""
	m.interaction.cursor = 0
	m.resetDialogRefreshState()
	m.transcriptRefresh.plan = transcriptRefreshPlan{}
	m.transcriptView.layout = transcriptLayout{}
	m.transcriptView.turnSourceKeys = make(map[string]string)
	m.renderCache.transcriptPane = newRenderedTextCache("transcript_pane")
	m.renderCache.splitTranscriptPane = newRenderedTextCache("split_transcript_pane")
	m.renderCache.splitInspectorPane = newRenderedTextCache("split_inspector_pane")
	m.renderCache.splitWideView = newRenderedTextCache("split_wide_view")
	m.renderCache.dialogOverlay = newRenderedOverlayCache()
	m.renderCache.composerOverlay = newRenderedOverlayCache()
	m.renderCache.transcriptMarkdown = newStreamingMarkdownSurfaceCache(64)
	m.transcriptView.selectionLines = nil
	m.transcriptView.cursorLine = 0
	m.transcriptView.cursorColumn = 0
	m.transcriptView.cursorGoalColumn = 0
	m.transcriptView.cursorInitialized = false
	m.transcriptView.mouseSelecting = false
	m.transcriptView.mouseAnchorLine = 0
	m.transcriptView.mouseAnchorColumn = 0
	m.transcriptView.visualActive = false
	m.transcriptView.visualAnchorLine = 0
	m.transcriptView.visualAnchorColumn = 0
	m.toolHydration.loadedResults = make(map[scopedToolCallKey]app.ToolResultDetail)
	m.toolHydration.loadingResults = make(map[scopedToolCallKey]bool)
	m.toolHydration.loadedMutations = make(map[scopedToolCallKey]loadedToolMutationDetail)
	m.toolHydration.loadingMutations = make(map[scopedToolCallKey]bool)
	m.footerStatus.workspace = app.WorkspaceStatus{}
	m.footerStatus.workspaceLoading = false
	m.footerStatus.budget = app.BudgetStatus{}
	m.footerStatus.sessionUsage = app.SessionUsageSummary{}
	m.clearFooterActivity()
	m.resetComposerHistoryRecall()
	m.dismissComposerPopup()
	if stateOwned {
		m.projector = events.NewProjectorFromOwnedSnapshot(state)
	} else {
		m.projector = events.NewProjectorFromSnapshot(state)
	}
	m.primeTranscriptTurnSourceKeys(state)
	m.stream = stream
	m.cancel = cancel
	m.watchID = watchID
	_ = m.refreshAvailableAgents()
	_ = m.refreshDialogState()
	m.syncFocusState()
	m.syncComposerFocus()
}

func resolvedUserText(state events.SessionState, view sessionView) string {
	turn := state.Turns[view.TurnID]
	if turn != nil && strings.TrimSpace(turn.UserText) != "" {
		return turn.UserText
	}
	return view.UserText
}

func resolvedWorkspaceRoot(state events.SessionState, fallback string) string {
	if strings.TrimSpace(state.WorkspaceRoot) != "" {
		return state.WorkspaceRoot
	}
	return fallback
}

func resolvedAgentID(state events.SessionState, turnID, fallback string) string {
	if turn := currentTurn(state, turnID); turn != nil && turn.Config != nil {
		if agentID := strings.TrimSpace(turn.Config.AgentID); agentID != "" {
			return agentID
		}
	}
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turn := state.Turns[state.TurnOrder[idx]]
		if turn == nil || turn.Config == nil {
			continue
		}
		if agentID := strings.TrimSpace(turn.Config.AgentID); agentID != "" {
			return agentID
		}
	}
	if agentID := strings.TrimSpace(fallback); agentID != "" {
		return agentID
	}
	return "builder"
}

func resolvedWorkflowID(state events.SessionState, turnID, fallback string) string {
	if turn := currentTurn(state, turnID); turn != nil && turn.Config != nil {
		if workflowID := strings.TrimSpace(turn.Config.WorkflowID); workflowID != "" {
			return workflowID
		}
	}
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turn := state.Turns[state.TurnOrder[idx]]
		if turn == nil || turn.Config == nil {
			continue
		}
		if workflowID := strings.TrimSpace(turn.Config.WorkflowID); workflowID != "" {
			return workflowID
		}
	}
	return strings.TrimSpace(fallback)
}

func resolvedSkillIDs(state events.SessionState, turnID string, fallback []string) []string {
	if turn := currentTurn(state, turnID); turn != nil && turn.Config != nil {
		if turn.Config.SelectedSkillIDs != nil {
			return append([]string(nil), turn.Config.SelectedSkillIDs...)
		}
	}
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turn := state.Turns[state.TurnOrder[idx]]
		if turn == nil || turn.Config == nil {
			continue
		}
		if turn.Config.SelectedSkillIDs != nil {
			return append([]string(nil), turn.Config.SelectedSkillIDs...)
		}
	}
	return append([]string(nil), fallback...)
}

func resolvedThinkingEnabled(state events.SessionState, turnID string, fallback bool) bool {
	if turn := currentTurn(state, turnID); turn != nil && turn.Config != nil {
		return turn.Config.ThinkingEnabled
	}
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turn := state.Turns[state.TurnOrder[idx]]
		if turn == nil || turn.Config == nil {
			continue
		}
		return turn.Config.ThinkingEnabled
	}
	return fallback
}

func resolvedReasoningVariant(state events.SessionState, turnID, fallback string) string {
	if turn := currentTurn(state, turnID); turn != nil && turn.Config != nil {
		if mode := strings.TrimSpace(turn.Config.ThinkingMode); mode != "" {
			return mode
		}
	}
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turn := state.Turns[state.TurnOrder[idx]]
		if turn == nil || turn.Config == nil {
			continue
		}
		if mode := strings.TrimSpace(turn.Config.ThinkingMode); mode != "" {
			return mode
		}
	}
	return strings.TrimSpace(fallback)
}

func (m *Model) cancelWatch() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

func (m *Model) beginShutdown() {
	m.shuttingDown = true
	m.cancelWatch()
}
