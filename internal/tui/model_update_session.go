package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

func (m Model) updateSessionLifecycleMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch typed := msg.(type) {
	case sessionOpenedMsg:
		next, cmd := m.handleSessionOpenedMsg(typed)
		return next, cmd, true
	case sessionWatchOpenedMsg:
		next, cmd := m.handleSessionWatchOpenedMsg(typed)
		return next, cmd, true
	case watchEventMsg:
		if !typed.open {
			next, cmd := m.handleWatchEvents(typed.id, nil, true)
			return next.(Model), cmd, true
		}
		next, cmd := m.handleWatchEvents(typed.id, []events.Event{typed.event}, false)
		return next.(Model), cmd, true
	case watchEventsMsg:
		next, cmd := m.handleWatchEvents(typed.id, typed.events, typed.closed)
		return next.(Model), cmd, true
	default:
		return m, nil, false
	}
}

func (m Model) handleSessionOpenedMsg(msg sessionOpenedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.busy = false
		m.disarmLiveTurn()
		m.liveTurn.cancelRequested = false
		return m, nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.clearFooterError()
	m.applyView(msg.view, msg.state, msg.stateOwned, msg.stream, msg.cancel, msg.watchID)
	m.syncViewportLayout()
	if msg.watchID >= m.nextWatch {
		m.nextWatch = msg.watchID + 1
	}
	if msg.startTurn {
		startAgentID := strings.TrimSpace(msg.startTurnAgentID)
		if startAgentID == "" {
			startAgentID = m.agentID
		}
		m.busy = true
		m.armLiveTurn()
		return m, tea.Batch(
			waitForEventCmd(m.stream, m.watchID),
			loadBudgetStatusCmd(m.ctx, m.controller, m.sessionID),
			loadSessionUsageSummaryCmd(m.ctx, m.controller, m.sessionID),
			startTurnCmd(m.ctx, m.controller, m.sessionID, m.turnID, m.userText, append([]app.AttachmentInput(nil), msg.attachments...), startAgentID, m.thinkingEnabled, m.reasoningVariant, m.skillIDs),
			m.ensureWorkspaceStatusLoadedCmd(),
			m.ensureAnimTicking(),
			m.ensureSelectedToolResultLoadedCmd(),
			m.ensureRelevantDelegatedSessionSnapshotsLoadedCmd(m.projector.Snapshot()),
			m.ensureSelectedDelegatedSessionSnapshotLoadedCmd(),
		)
	}
	if msg.startReview {
		m.busy = true
		m.armLiveTurn()
		return m, tea.Batch(
			waitForEventCmd(m.stream, m.watchID),
			loadBudgetStatusCmd(m.ctx, m.controller, m.sessionID),
			loadSessionUsageSummaryCmd(m.ctx, m.controller, m.sessionID),
			startReviewCmd(
				m.ctx,
				m.controller,
				m.sessionID,
				m.turnID,
				msg.reviewInstructions,
				msg.reviewThinkingEnabled,
				msg.reviewThinkingMode,
				append([]string(nil), msg.reviewSkillIDs...),
			),
			m.ensureWorkspaceStatusLoadedCmd(),
			m.ensureAnimTicking(),
			m.ensureSelectedToolResultLoadedCmd(),
			m.ensureRelevantDelegatedSessionSnapshotsLoadedCmd(m.projector.Snapshot()),
			m.ensureSelectedDelegatedSessionSnapshotLoadedCmd(),
		)
	}
	if strings.TrimSpace(msg.localShellCommand) != "" {
		m.busy = true
		m.armLiveTurn()
		return m, tea.Batch(
			waitForEventCmd(m.stream, m.watchID),
			loadBudgetStatusCmd(m.ctx, m.controller, m.sessionID),
			loadSessionUsageSummaryCmd(m.ctx, m.controller, m.sessionID),
			runLocalShellCommandCmd(m.ctx, m.controller, m.sessionID, m.turnID, msg.localShellCommand),
			m.ensureWorkspaceStatusLoadedCmd(),
			m.ensureAnimTicking(),
			m.ensureSelectedToolResultLoadedCmd(),
			m.ensureRelevantDelegatedSessionSnapshotsLoadedCmd(m.projector.Snapshot()),
			m.ensureSelectedDelegatedSessionSnapshotLoadedCmd(),
		)
	}
	m.syncLiveTurnWithState(msg.state)
	m.busy = false
	if m.hasPendingInteraction() {
		m.chrome.focus = focusTranscript
	} else {
		m.chrome.focus = focusComposer
	}
	m.syncViewportLayout()
	return m, tea.Batch(
		waitForEventCmd(m.stream, m.watchID),
		loadBudgetStatusCmd(m.ctx, m.controller, m.sessionID),
		loadSessionUsageSummaryCmd(m.ctx, m.controller, m.sessionID),
		m.ensureWorkspaceStatusLoadedCmd(),
		m.syncComposerFocus(),
		m.ensureSelectedToolResultLoadedCmd(),
		m.ensureRelevantDelegatedSessionSnapshotsLoadedCmd(m.projector.Snapshot()),
		m.ensureSelectedDelegatedSessionSnapshotLoadedCmd(),
	)
}

func (m Model) handleSessionWatchOpenedMsg(msg sessionWatchOpenedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.busy = false
		m.disarmLiveTurn()
		m.liveTurn.cancelRequested = false
		return m, nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.clearFooterError()
	m.stream = msg.stream
	m.cancel = msg.cancel
	m.watchID = msg.watchID
	m.syncViewportLayout()
	if msg.watchID >= m.nextWatch {
		m.nextWatch = msg.watchID + 1
	}
	if msg.startTurn {
		m.busy = true
		m.armLiveTurn()
		return m, tea.Batch(
			m.syncComposerFocus(),
			waitForEventCmd(m.stream, m.watchID),
			loadBudgetStatusCmd(m.ctx, m.controller, m.sessionID),
			loadSessionUsageSummaryCmd(m.ctx, m.controller, m.sessionID),
			startTurnCmd(m.ctx, m.controller, m.sessionID, m.turnID, m.userText, nil, m.agentID, m.thinkingEnabled, m.reasoningVariant, m.skillIDs),
			m.ensureWorkspaceStatusLoadedCmd(),
			m.ensureAnimTicking(),
			m.ensureSelectedToolResultLoadedCmd(),
			m.ensureSelectedDelegatedSessionSnapshotLoadedCmd(),
		)
	}
	m.syncLiveTurnWithState(m.projector.CurrentState())
	m.busy = false
	return m, tea.Batch(
		m.syncComposerFocus(),
		waitForEventCmd(m.stream, m.watchID),
		loadBudgetStatusCmd(m.ctx, m.controller, m.sessionID),
		loadSessionUsageSummaryCmd(m.ctx, m.controller, m.sessionID),
		m.ensureWorkspaceStatusLoadedCmd(),
		m.ensureSelectedToolResultLoadedCmd(),
		m.ensureSelectedDelegatedSessionSnapshotLoadedCmd(),
	)
}
