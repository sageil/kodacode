package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
	tuitheme "github.com/sageil/kodacode/internal/tui/theme"
)

func (m *Model) openCommandPalette() tea.Cmd {
	return m.openCommandPaletteWithQuery("")
}

func (m *Model) openModelDialog() tea.Cmd {
	return func() tea.Msg {
		state, err := m.backend.DialogState(m.ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newModelDialog(buildModelItems(state, currentDialogModelRef(*m)), currentDialogModelSelection(*m), !m.currentTurnRunning(), m.theme)
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openAgentDialog() tea.Cmd {
	return func() tea.Msg {
		agents, err := m.backend.ListAgents(m.ctx, m.workspace)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newAgentDialog(buildAgentItems(agents), strings.TrimSpace(m.agentID), !m.currentTurnRunning(), m.theme)
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openUtilityModelDialog() tea.Cmd {
	return func() tea.Msg {
		state, err := m.backend.DialogState(m.ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newUtilityModelDialog(buildModelItems(state, state.UtilityModel), currentDialogUtilityModelSelection(state), !m.currentTurnRunning(), m.theme)
		dialog.refilter()
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openReviewerModelDialog() tea.Cmd {
	return func() tea.Msg {
		state, err := m.backend.DialogState(m.ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newReviewerModelDialog(buildModelItems(state, state.ReviewModelRoute.Primary), currentDialogReviewerModelSelection(state), !m.currentTurnRunning(), m.theme)
		dialog.refilter()
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openCommandPaletteWithQuery(query string) tea.Cmd {
	return func() tea.Msg {
		state, err := m.backend.DialogState(m.ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newCommandPaletteActions(commandPaletteActionsItems{
			ModelItems:            buildModelItems(state, currentDialogModelRef(*m)),
			CurrentUtilityModel:   currentDialogUtilityModelSelection(state),
			CurrentReviewerModel:  currentDialogReviewerModelSelection(state),
			AllowMutableSelection: !m.currentTurnRunning(),
		}, m.theme)
		dialog.filter.SetValue(strings.TrimSpace(query))
		dialog.refilter()
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openThemeDialog() tea.Cmd {
	return func() tea.Msg {
		names, err := tuitheme.Names()
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newThemeDialog(buildThemeItems(names), normalizedThemeSelection(m.themeName), m.theme)
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openSessionsDialog() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.backend.ListSessions(m.ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newSessionsDialog(buildSessionItems(filterSessionSummaries(sessions, m.sessionID)), m.theme)
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openTimelineDialog() tea.Cmd {
	return func() tea.Msg {
		state := m.projector.Snapshot()
		sessions, err := m.backend.ListSessions(m.ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newTimelineDialog(state, sessions, m.theme)
		width, height := dialogRenderSize(*m, state)
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openConnectDialog() tea.Cmd {
	return func() tea.Msg {
		state, err := m.backend.DialogState(m.ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newConnectDialog(buildConnectEntries(state), m.theme)
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openNewSessionDialog() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.backend.ListSessions(m.ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newSessionsDialog(buildSessionItems(filterSessionSummaries(sessions, m.sessionID)), m.theme)
		dialog.mode = sessionsDialogCreate
		dialog.focusIndex = 0
		dialog.syncFocus()
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openGitHubCopilotAuthDialog(baseURL string) tea.Cmd {
	return func() tea.Msg {
		dialog := newGitHubCopilotAuthDialog(m.ctx, m.backend, baseURL, m.theme)
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openOpenAIAuthDialog() tea.Cmd {
	return func() tea.Msg {
		dialog := newOpenAIAuthDialog(m.ctx, m.backend, m.theme)
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) openTrustDialog() tea.Cmd {
	return func() tea.Msg {
		state, err := m.backend.WorkspaceTrustState(m.ctx, m.workspace)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newTrustDialogWithIcons(state, m.theme, m.terminalIcons)
		width, height := dialogRenderSize(*m, m.projector.CurrentState())
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func (m *Model) handleDialogClosed(msg dialogClosedMsg) (tea.Model, tea.Cmd) {
	if msg.id == dialogIDToolDetail {
		m.clearToolMutationDetailCache()
	}
	if msg.result == nil {
		m.restorePreviousDialogOrClose()
		return *m, m.syncComposerFocus()
	}

	if msg.id == dialogIDCommandPalette {
		switch typed := msg.result.(type) {
		case agentItem:
			if m.currentTurnRunning() {
				m.closeAllDialogs()
				return *m, nil
			}
			m.closeAllDialogs()
			m.agentID = typed.ID
			m.resetInspectorAgentSelectionToCurrentTurn()
			if err := m.refreshAvailableAgents(); err != nil {
				m.setFooterError(err.Error())
			} else {
				m.clearFooterError()
			}
			m.syncInspectorTabAvailability()
			m.syncInspectorBody(true)
			return *m, nil
		case provider.ModelRef:
			if m.currentTurnRunning() {
				m.closeAllDialogs()
				return *m, nil
			}
			model, ok := availableModelForRef(*m, typed)
			if !ok || len(model.SupportedReasoningVariants) == 0 {
				m.closeAllDialogs()
				m.reasoningVariant = ""
				return *m, tea.Batch(
					m.focusComposerAfterDialogSelection(),
					setPrimaryModelCmd(m.ctx, m.backend, m.currentView(), typed, m.nextWatch),
				)
			}
			return *m, m.openReasoningVariantDialog(typed, model.SupportedReasoningVariants, true, m.currentView())
		case utilityModelSelectionResult:
			if m.currentTurnRunning() {
				m.closeAllDialogs()
				return *m, nil
			}
			m.closeAllDialogs()
			return *m, tea.Batch(
				m.focusComposerAfterDialogSelection(),
				setUtilityModelCmd(m.ctx, m.backend, typed.Ref),
			)
		case reviewerModelSelectionResult:
			if m.currentTurnRunning() {
				m.closeAllDialogs()
				return *m, nil
			}
			m.closeAllDialogs()
			return *m, tea.Batch(
				m.focusComposerAfterDialogSelection(),
				setReviewerModelCmd(m.ctx, m.backend, typed.Ref),
			)
		case commandPaletteActionResult:
			if m.currentTurnRunning() && (typed.ActionID == "select-model" || typed.ActionID == "select-agent" || typed.ActionID == "manage-sessions" || typed.ActionID == "timeline" || typed.ActionID == "new-session" || typed.ActionID == "select-utility-model" || typed.ActionID == "unset-utility-model" || typed.ActionID == "select-reviewer-model" || typed.ActionID == "unset-reviewer-model") {
				m.closeAllDialogs()
				return *m, nil
			}
			switch typed.ActionID {
			case "select-model":
				return *m, m.openModelDialog()
			case "select-agent":
				return *m, m.openAgentDialog()
			case "select-theme":
				return *m, m.openThemeDialog()
			case "manage-sessions":
				return *m, m.openSessionsDialog()
			case "timeline":
				if strings.TrimSpace(m.sessionID) == "" {
					m.closeAllDialogs()
					m.setFooterError(timelineUnavailableMessage)
					return *m, nil
				}
				return *m, m.openTimelineDialog()
			case "new-session":
				return *m, m.openNewSessionDialog()
			case "manage-trust":
				return *m, m.openTrustDialog()
			case "connect-provider":
				return *m, m.openConnectDialog()
			case "select-utility-model":
				return *m, m.openUtilityModelDialog()
			case "unset-utility-model":
				m.closeAllDialogs()
				return *m, tea.Batch(
					m.focusComposerAfterDialogSelection(),
					setUtilityModelCmd(m.ctx, m.backend, provider.ModelRef{}),
				)
			case "select-reviewer-model":
				return *m, m.openReviewerModelDialog()
			case "unset-reviewer-model":
				m.closeAllDialogs()
				return *m, tea.Batch(
					m.focusComposerAfterDialogSelection(),
					setReviewerModelCmd(m.ctx, m.backend, provider.ModelRef{}),
				)
			}
		}
		m.closeAllDialogs()
		return *m, nil
	}

	switch msg.id {
	case dialogIDShellTools:
		result, ok := msg.result.(shellToolsDialogResult)
		if !ok {
			m.closeCurrentDialogPreservingStack()
			return *m, nil
		}
		return *m, m.openShellToolsDialogResult(result)
	case dialogIDTheme:
		item, ok := msg.result.(themeItem)
		if !ok {
			m.closeCurrentDialogPreservingStack()
			return *m, nil
		}
		m.closeAllDialogs()
		return *m, tea.Batch(m.focusComposerAfterDialogSelection(), applyThemeCmd(m.ctx, m.backend, item.Name))
	case dialogIDSessions:
		result, ok := msg.result.(sessionsDialogResult)
		if !ok {
			m.closeCurrentDialogPreservingStack()
			return *m, nil
		}
		switch {
		case strings.TrimSpace(result.OpenSessionID) != "":
			m.closeAllDialogs()
			m.busy = true
			return *m, switchSessionCmd(m.ctx, m.backend, sessionSwitchRequest{
				SessionID:        result.OpenSessionID,
				WorkspaceRoot:    m.workspace,
				AgentID:          m.agentID,
				ThinkingEnabled:  m.thinkingEnabled,
				ReasoningVariant: m.reasoningVariant,
				SkillIDs:         append([]string(nil), m.skillIDs...),
				InspectorOpen:    m.chrome.inspectorOpen,
				WideSidebarOpen:  m.chrome.wideSidebarOpen,
				WatchID:          m.nextWatch,
			})
		case result.Create:
			m.closeAllDialogs()
			m.busy = true
			return *m, openWorkspaceSessionCmd(m.ctx, m.backend, workspaceSessionOpenRequest{
				WorkspaceRoot:    m.workspace,
				UserText:         result.NewPrompt,
				TurnID:           app.NewTurnID(),
				AgentID:          m.agentID,
				StartTurnAgentID: m.agentID,
				ThinkingEnabled:  m.thinkingEnabled,
				ReasoningVariant: m.reasoningVariant,
				SkillIDs:         append([]string(nil), m.skillIDs...),
				InspectorOpen:    m.chrome.inspectorOpen,
				WideSidebarOpen:  m.chrome.wideSidebarOpen,
				WatchID:          m.nextWatch,
			})
		case strings.TrimSpace(result.DeleteID) != "":
			m.closeCurrentDialogPreservingStack()
			return *m, deleteSessionAndReopenDialogCmd(m.ctx, m.backend, m.sessionID, result.DeleteID, m.theme, m.width, m.height)
		case len(result.PurgeIDs) > 0:
			m.closeCurrentDialogPreservingStack()
			return *m, purgeSessionsAndReopenDialogCmd(m.ctx, m.backend, m.sessionID, result.PurgeIDs, m.theme, m.width, m.height)
		}
	case dialogIDTimeline:
		result, ok := msg.result.(timelineDialogResult)
		if !ok {
			m.closeCurrentDialogPreservingStack()
			return *m, nil
		}
		switch {
		case strings.TrimSpace(result.LabelSessionID) != "":
			m.closeAllDialogs()
			return *m, setSessionTitleCmd(m.ctx, m.backend, result.LabelSessionID, result.Label)
		case strings.TrimSpace(result.SummarySessionID) != "":
			m.closeAllDialogs()
			m.busy = true
			return *m, generateBranchSummaryCmd(m.ctx, m.backend, result.SummarySessionID)
		case strings.TrimSpace(result.OpenSessionID) != "":
			m.closeAllDialogs()
			m.busy = true
			return *m, switchSessionCmd(m.ctx, m.backend, sessionSwitchRequest{
				SessionID:        result.OpenSessionID,
				WorkspaceRoot:    m.workspace,
				AgentID:          m.agentID,
				ThinkingEnabled:  m.thinkingEnabled,
				ReasoningVariant: m.reasoningVariant,
				SkillIDs:         append([]string(nil), m.skillIDs...),
				InspectorOpen:    m.chrome.inspectorOpen,
				WideSidebarOpen:  m.chrome.wideSidebarOpen,
				WatchID:          m.nextWatch,
			})
		case strings.TrimSpace(result.TraceTurnID) != "":
			m.closeCurrentDialogPreservingStack()
			return *m, m.openTraceDialogForTurnID(result.TraceTurnID)
		case strings.TrimSpace(result.BranchTurnID) != "":
			m.closeAllDialogs()
			m.busy = true
			return *m, branchSessionFromTurnCmd(m.ctx, m.backend, timelineBranchRequest{
				SourceSessionID:  m.sessionID,
				SourceTurnID:     result.BranchTurnID,
				WorkspaceRoot:    m.workspace,
				AgentID:          m.agentID,
				ThinkingEnabled:  m.thinkingEnabled,
				ReasoningVariant: m.reasoningVariant,
				SkillIDs:         append([]string(nil), m.skillIDs...),
				InspectorOpen:    m.chrome.inspectorOpen,
				WideSidebarOpen:  m.chrome.wideSidebarOpen,
				WatchID:          m.nextWatch,
			})
		}
	case dialogIDSkills:
		result, ok := msg.result.(skillsDialogResult)
		if !ok {
			m.closeCurrentDialogPreservingStack()
			return *m, nil
		}
		m.closeAllDialogs()
		m.skillIDs = append([]string(nil), result.SkillIDs...)
		m.clearFooterError()
		m.syncInspectorBody(true)
		return *m, tea.Batch(
			m.focusComposerAfterDialogSelection(),
			m.showFooterActivity(skillSelectionFooterLabel(m.skillIDs), footerActivityToneInfo, ""),
		)
	case dialogIDConnect:
		switch typed := msg.result.(type) {
		case connectDialogResult:
			if typed.Save != nil {
				m.closeAllDialogs()
				return *m, saveProviderCmd(m.ctx, m.backend, *typed.Save)
			}
			if strings.TrimSpace(typed.Remove) != "" {
				m.closeAllDialogs()
				return *m, removeProviderCmd(m.ctx, m.backend, typed.Remove)
			}
		case openAIAuthDialogRequest:
			return *m, m.openOpenAIAuthDialog()
		case gitHubCopilotAuthDialogRequest:
			return *m, m.openGitHubCopilotAuthDialog(typed.BaseURL)
		}
	case dialogIDOpenAIAuth:
		result, ok := msg.result.(openAIAuthDialogResult)
		if !ok {
			m.closeCurrentDialogPreservingStack()
			return *m, nil
		}
		m.closeAllDialogs()
		m.cacheDialogState(result.State)
		m.clearFooterError()
		return *m, tea.Batch(
			m.syncComposerFocus(),
			m.showFooterActivity("OpenAI connected", footerActivityToneInfo, ""),
		)
	case dialogIDGitHubCopilotAuth:
		result, ok := msg.result.(gitHubCopilotAuthDialogResult)
		if !ok {
			m.closeCurrentDialogPreservingStack()
			return *m, nil
		}
		m.closeAllDialogs()
		m.cacheDialogState(result.State)
		m.clearFooterError()
		return *m, tea.Batch(
			m.syncComposerFocus(),
			m.showFooterActivity("GitHub Copilot connected", footerActivityToneInfo, ""),
		)
	case dialogIDTrust:
		result, ok := msg.result.(trustDialogResult)
		if !ok {
			m.closeCurrentDialogPreservingStack()
			return *m, nil
		}
		m.closeCurrentDialogPreservingStack()
		return *m, revokeTrustAndReopenDialogCmd(m.ctx, m.backend, m.sessionID, m.workspace, result, m.theme, m.terminalIcons, m.width, m.height)
	case dialogIDReasoningVariant:
		result, ok := msg.result.(reasoningVariantDialogResult)
		if !ok {
			m.closeCurrentDialogPreservingStack()
			return *m, nil
		}
		m.closeAllDialogs()
		m.reasoningVariant = strings.TrimSpace(strings.ToLower(result.Variant))
		if result.ApplyModel {
			return *m, tea.Batch(
				m.focusComposerAfterDialogSelection(),
				setPrimaryModelCmd(m.ctx, m.backend, result.View, result.Model, m.nextWatch),
			)
		}
		return *m, tea.Batch(
			m.focusComposerAfterDialogSelection(),
			m.showFooterActivity(reasoningVariantFooterLabel(m.reasoningVariant), footerActivityToneInfo, ""),
		)
	}
	m.closeCurrentDialogPreservingStack()
	return *m, nil
}
