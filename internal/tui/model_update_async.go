package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

func (m Model) updateAsyncStateMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch typed := msg.(type) {
	case toolResultLoadedMsg:
		key := scopedToolKey(typed.sessionID, typed.ref)
		delete(m.toolHydration.loadingResults, key)
		if typed.err == nil {
			m.toolHydration.loadedResults[key] = typed.result
			if typed.sessionID == m.sessionID {
				_ = m.applyTranscriptRefreshPlan(m.transcriptRefreshPlanForLoadedToolResult(m.projector.Snapshot(), typed.ref))
			}
			return m, m.requestDialogRefresh(dialogIDToolDetail), true
		}
		return m, nil, true
	case toolMutationDetailLoadedMsg:
		key := scopedToolKey(typed.sessionID, typed.ref)
		delete(m.toolHydration.loadingMutations, key)
		if typed.err == nil && m.shouldCacheToolMutationDetail(typed.sessionID, typed.ref) {
			m.toolHydration.loadedMutations = map[scopedToolCallKey]loadedToolMutationDetail{key: typed.detail}
			return m, m.requestDialogRefresh(dialogIDToolDetail), true
		}
		return m, nil, true
	case workspaceStatusLoadedMsg:
		if typed.sessionID != m.sessionID {
			return m, nil, true
		}
		m.footerStatus.workspaceLoading = false
		if typed.err != nil {
			m.footerStatus.workspace = app.WorkspaceStatus{}
			return m, nil, true
		}
		m.footerStatus.workspace = typed.status
		return m, nil, true
	case budgetStatusLoadedMsg:
		if typed.sessionID != m.sessionID || typed.err != nil {
			return m, nil, true
		}
		m.footerStatus.budget = typed.status
		return m, m.requestDialogRefresh(dialogIDCost), true
	case sessionUsageSummaryLoadedMsg:
		if typed.sessionID != m.sessionID || typed.err != nil {
			return m, nil, true
		}
		m.footerStatus.sessionUsage = typed.summary
		return m, m.requestDialogRefresh(dialogIDCost), true
	case promptHistoryLoadedMsg:
		m.composerState.promptHistoryBusy = false
		if typed.err != nil {
			m.setComposerError(typed.err.Error())
			return m, nil, true
		}
		m.clearComposerError()
		m.composerState.promptHistory = append([]app.PromptHistoryEntry(nil), typed.entries...)
		m.composerState.promptHistoryPending = filterPendingPromptHistoryEntries(m.composerState.promptHistoryPending, m.composerState.promptHistory)
		m.composerState.promptHistoryLoaded = true
		if m.composerState.popupMode != composerPopupHistory && m.composerState.historyRecallActive {
			m.applyComposerHistoryRecall()
		}
		m.clampComposerPopupCursor()
		return m, nil, true
	case composerSkillsLoadedMsg:
		m.composerState.skillsBusy = false
		if typed.err != nil {
			m.setComposerError(typed.err.Error())
			return m, nil, true
		}
		m.clearComposerError()
		m.composerState.skills = append([]app.AvailableSkill(nil), typed.skills...)
		m.composerState.skillsLoaded = true
		if m.composerState.popupMode == composerPopupSkills && m.shouldDismissEmptySkillPopup() {
			m.dismissComposerPopup()
			return m, nil, true
		}
		m.clampComposerPopupCursor()
		return m, nil, true
	case composerWorkspacePathsLoadedMsg:
		m.composerState.workspacePathsBusy = false
		if typed.err != nil {
			m.setComposerError(typed.err.Error())
			return m, nil, true
		}
		m.clearComposerError()
		m.composerState.workspacePaths = append([]app.WorkspacePath(nil), typed.paths...)
		m.composerState.workspacePathsLoaded = true
		m.clampComposerPopupCursor()
		return m, nil, true
	case operationDoneMsg:
		next, cmd := m.handleOperationDoneMsg(typed)
		return next, cmd, true
	case workflowResumedMsg:
		next, cmd := m.handleWorkflowResumedMsg(typed)
		return next, cmd, true
	case turnWritesRestoredMsg:
		next, cmd := m.handleTurnWritesRestoredMsg(typed)
		return next, cmd, true
	case sessionCompactedMsg:
		next, cmd := m.handleSessionCompactedMsg(typed)
		return next, cmd, true
	case workspaceInstructionsInitializedMsg:
		next, cmd := m.handleWorkspaceInstructionsInitializedMsg(typed)
		return next, cmd, true
	case workspacePromptSourcesCompressedMsg:
		next, cmd := m.handleWorkspacePromptSourcesCompressedMsg(typed)
		return next, cmd, true
	case turnCancelRequestedMsg:
		next, cmd := m.handleTurnCancelRequestedMsg(typed)
		return next, cmd, true
	case sessionSnapshotRefreshedMsg:
		next, cmd := m.handleSessionSnapshotRefreshedMsg(typed)
		return next, cmd, true
	case transcriptCopiedMsg:
		if typed.err != nil {
			m.setFooterError(transcriptCopyErrorText(typed.err))
			return m, nil, true
		}
		m.clearFooterError()
		return m, m.showFooterActivity(typed.label, footerActivityToneInfo, ""), true
	default:
		return m, nil, false
	}
}

func (m Model) handleOperationDoneMsg(msg operationDoneMsg) (Model, tea.Cmd) {
	m.busy = false
	m.liveTurn.cancelRequested = false
	if msg.err != nil {
		m.interaction.resolveReq = ""
		m.disarmLiveTurn()
		m.err = msg.err
		m.chrome.focus = focusComposer
		m.syncViewportLayout()
		return m, m.syncComposerFocus()
	}
	if msg.sessionResult != nil {
		m.trackSessionTurnResult(*msg.sessionResult)
	}
	m.interaction.resolveReq = ""
	m.clearFooterError()
	if m.isFinished() && !m.hasPendingApproval() {
		m.chrome.focus = focusComposer
		m.syncViewportLayout()
	} else if !m.hasPendingApproval() {
		m.chrome.focus = focusTranscript
		m.syncViewportLayout()
	}
	return m, m.syncComposerFocus()
}

func (m Model) handleWorkflowResumedMsg(msg workflowResumedMsg) (Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		m.clearFooterError()
		m.setComposerError(msg.err.Error())
		m.chrome.focus = focusComposer
		m.syncViewportLayout()
		return m, m.syncComposerFocus()
	}
	m.clearComposerError()
	m.clearFooterError()
	m.chrome.focus = focusComposer
	m.syncViewportLayout()
	return m, tea.Batch(
		m.syncComposerFocus(),
		m.showFooterActivity("Workflow resumed", footerActivityToneInfo, ""),
	)
}

func (m Model) handleTurnWritesRestoredMsg(msg turnWritesRestoredMsg) (Model, tea.Cmd) {
	m.busy = false
	m.liveTurn.cancelRequested = false
	if msg.err != nil {
		m.clearFooterError()
		m.setComposerError(msg.err.Error())
		m.chrome.focus = focusComposer
		m.syncViewportLayout()
		return m, m.syncComposerFocus()
	}
	m.clearFooterError()
	m.clearComposerError()
	if !m.hasPendingApproval() {
		m.chrome.focus = focusTranscript
		m.syncViewportLayout()
	}
	state := m.projector.Snapshot()
	ordinal := sessionToolTurnOrdinal(state, msg.result.SourceTurnID)
	label := "Restored writes"
	if ordinal > 0 {
		label = "Restored turn " + intToString(ordinal) + " writes"
	}
	return m, tea.Batch(
		m.syncComposerFocus(),
		m.showFooterActivity(label, footerActivityToneInfo, ""),
	)
}

func (m Model) handleSessionCompactedMsg(msg sessionCompactedMsg) (Model, tea.Cmd) {
	turnStillRunning := false
	if turn := currentTurn(m.projector.Snapshot(), m.turnID); turn != nil && turn.Status == events.TurnStatusRunning {
		turnStillRunning = true
	}
	if !turnStillRunning {
		m.busy = false
		m.liveTurn.cancelRequested = false
		m.disarmLiveTurn()
	}
	if msg.err != nil {
		m.clearFooterError()
		m.setComposerError(msg.err.Error())
		m.chrome.focus = focusComposer
		m.syncViewportLayout()
		return m, m.syncComposerFocus()
	}
	m.clearFooterError()
	m.clearComposerError()
	if !m.hasPendingInteraction() {
		m.chrome.focus = focusTranscript
		m.syncViewportLayout()
	}
	text := "No older completed turns are eligible for compaction"
	tone := footerActivityToneInfo
	kind := footerActivityKind("")
	if msg.result.Continuation != nil {
		text = historyCompactionActivityText(*msg.result.Continuation)
		tone = footerActivityToneWarning
		kind = footerActivityKindHistory
	}
	return m, tea.Batch(
		m.syncComposerFocus(),
		m.showFooterActivity(text, tone, kind),
	)
}

func (m Model) handleWorkspaceInstructionsInitializedMsg(msg workspaceInstructionsInitializedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.setComposerError(msg.err.Error())
		return m, m.syncComposerFocus()
	}
	m.clearComposerError()
	m.clearFooterError()
	return m, tea.Batch(
		m.syncComposerFocus(),
		m.showFooterActivity(workspaceInstructionsInitializedActivityText(msg.result), footerActivityToneInfo, ""),
	)
}

func (m Model) handleWorkspacePromptSourcesCompressedMsg(msg workspacePromptSourcesCompressedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.setComposerError(msg.err.Error())
		return m, m.syncComposerFocus()
	}
	m.clearComposerError()
	m.clearFooterError()
	return m, tea.Batch(
		m.syncComposerFocus(),
		m.showFooterActivity(workspacePromptSourcesCompressedActivityText(msg.result), footerActivityToneInfo, ""),
	)
}

func workspaceInstructionsInitializedActivityText(result app.InitializeWorkspaceInstructionsResult) string {
	created := make([]string, 0, 2)
	if result.AgentsCreated {
		created = append(created, "AGENTS.md")
	}
	if result.ClaudeCreated {
		created = append(created, "CLAUDE.md")
	}
	if len(created) == 0 {
		if strings.TrimSpace(result.ClaudePath) != "" {
			return "Workspace instructions already exist: AGENTS.md, CLAUDE.md"
		}
		return "Workspace instructions already exist: AGENTS.md"
	}
	if result.AgentsCreated {
		switch strings.TrimSpace(result.AgentsSource) {
		case "utility":
			if model := strings.TrimSpace(result.AgentsModel.String()); model != "" {
				if result.ClaudeCreated {
					return "Initialized AGENTS.md with " + model + " and CLAUDE.md"
				}
				return "Initialized AGENTS.md with " + model
			}
		case "template":
			if result.ClaudeCreated {
				return "Initialized AGENTS.md from template and CLAUDE.md"
			}
			return "Initialized AGENTS.md from template"
		}
	}
	return "Initialized " + strings.Join(created, " and ")
}

func workspacePromptSourcesCompressedActivityText(result app.CompressWorkspacePromptSourcesResult) string {
	if !result.AgentsPresent && result.MemoryCount == 0 {
		return "No workspace instructions or project memories to compress"
	}
	skipped := result.MemorySkippedLarge
	if result.AgentsSkippedLarge {
		skipped++
	}
	parts := make([]string, 0, 2)
	if result.AgentsUpdated {
		parts = append(parts, "AGENTS.md")
	}
	switch result.MemoryUpdatedCount {
	case 1:
		parts = append(parts, "1 project memory")
	case 0:
	default:
		parts = append(parts, fmt.Sprintf("%d project memories", result.MemoryUpdatedCount))
	}
	if len(parts) == 0 {
		if skipped > 0 {
			if skipped == 1 {
				return "Skipped 1 large prompt source; split or shorten it before compression"
			}
			return fmt.Sprintf("Skipped %d large prompt sources; split or shorten them before compression", skipped)
		}
		if result.AgentsPresent && result.MemoryCount > 0 {
			return "Workspace instructions and project memory are already concise"
		}
		if result.AgentsPresent {
			return "AGENTS.md is already concise"
		}
		return "Project memory is already concise"
	}
	message := "Compressed " + strings.Join(parts, " and ")
	if skipped > 0 {
		message += fmt.Sprintf("; skipped %d large", skipped)
	}
	return message
}

func (m Model) handleTurnCancelRequestedMsg(msg turnCancelRequestedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.liveTurn.cancelRequested = false
		if errors.Is(msg.err, app.ErrTurnNotRunning) {
			m.setFooterError(msg.err.Error())
			return m, refreshSessionSnapshotCmd(m.ctx, m.controller, m.sessionID)
		}
		m.setFooterError(msg.err.Error())
		return m, nil
	}
	m.clearFooterError()
	return m, nil
}

func (m Model) handleSessionSnapshotRefreshedMsg(msg sessionSnapshotRefreshedMsg) (Model, tea.Cmd) {
	if msg.sessionID != m.sessionID {
		return m, nil
	}
	if msg.err != nil {
		m.setFooterError(msg.err.Error())
		return m, nil
	}
	resolutionInFlight := m.interactionResolutionInFlight()
	m.projector = events.NewProjectorFromSnapshot(msg.state)
	m.primeTranscriptTurnSourceKeys(msg.state)
	trackedTurnFinished := isFinishedInState(msg.state, m.turnID)
	holdLiveTurnState := resolutionInFlight && trackedTurnFinished
	if trackedTurnFinished && !holdLiveTurnState {
		m.busy = false
		m.disarmLiveTurn()
		m.liveTurn.cancelRequested = false
		m.clearFooterError()
	}
	m.syncToolDetailDialog()
	m.syncShellToolsDialog()
	m.syncTaskDetailDialog()
	m.syncViewportLayout()
	return m, loadSessionUsageSummaryCmd(m.ctx, m.controller, m.sessionID)
}
