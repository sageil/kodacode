package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

const (
	watchEventBatchLimit  = 64
	watchEventBatchWindow = 50 * time.Millisecond
)

func openSessionCmd(ctx context.Context, controller controller, view sessionView, startTurn bool, watchID int) tea.Cmd {
	return func() tea.Msg {
		state, err := controller.Snapshot(ctx, view.SessionID)
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		watchCtx, cancel := context.WithCancel(ctx)
		stream, err := controller.Watch(watchCtx, view.SessionID, state.LastSequence)
		if err != nil {
			cancel()
			return sessionOpenedMsg{err: err}
		}
		return sessionOpenedMsg{
			view:       view,
			state:      state,
			stateOwned: true,
			stream:     stream,
			cancel:     cancel,
			watchID:    watchID,
			startTurn:  startTurn,
		}
	}
}

func watchSessionCmd(ctx context.Context, controller controller, sessionID string, afterSequence int64, startTurn bool, watchID int) tea.Cmd {
	return func() tea.Msg {
		watchCtx, cancel := context.WithCancel(ctx)
		stream, err := controller.Watch(watchCtx, sessionID, afterSequence)
		if err != nil {
			cancel()
			return sessionWatchOpenedMsg{err: err}
		}
		return sessionWatchOpenedMsg{
			stream:    stream,
			cancel:    cancel,
			watchID:   watchID,
			startTurn: startTurn,
		}
	}
}

func waitForEventCmd(stream <-chan events.Event, watchID int) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-stream
		if !ok {
			return watchEventMsg{open: false, id: watchID}
		}
		if shouldFlushEventBatch(event) {
			return watchEventMsg{event: event, open: true, id: watchID}
		}
		batch := []events.Event{event}
		if shouldBatchWatchEvent(event) {
			return collectWatchEventBatch(stream, watchID, batch)
		}
		for len(batch) < watchEventBatchLimit {
			select {
			case next, ok := <-stream:
				if !ok {
					return watchEventsMsg{events: batch, closed: true, id: watchID}
				}
				batch = append(batch, next)
				if shouldFlushEventBatch(next) {
					return watchEventsMsg{events: batch, id: watchID}
				}
			default:
				if len(batch) == 1 {
					return watchEventMsg{event: batch[0], open: true, id: watchID}
				}
				return watchEventsMsg{events: batch, id: watchID}
			}
		}
		return watchEventsMsg{events: batch, id: watchID}
	}
}

func collectWatchEventBatch(stream <-chan events.Event, watchID int, batch []events.Event) tea.Msg {
	timer := time.NewTimer(watchEventBatchWindow)
	defer timer.Stop()

	for len(batch) < watchEventBatchLimit {
		select {
		case next, ok := <-stream:
			if !ok {
				return watchEventsMsg{events: batch, closed: true, id: watchID}
			}
			batch = append(batch, next)
			if shouldFlushEventBatch(next) || !shouldBatchWatchEvent(next) {
				return watchEventsMsg{events: batch, id: watchID}
			}
		case <-timer.C:
			return watchEventsMsg{events: batch, id: watchID}
		}
	}
	return watchEventsMsg{events: batch, id: watchID}
}

func shouldFlushEventBatch(event events.Event) bool {
	switch event.Type {
	case events.TypeContextCompactionStarted:
		// Render compaction start immediately so the status bar can show
		// explicit history-compaction status before the eventual summary arrives.
		return true
	default:
		return false
	}
}

func shouldBatchWatchEvent(event events.Event) bool {
	switch event.Type {
	case events.TypeAssistantPreviewDelta,
		events.TypeAssistantPreviewReset,
		events.TypeReasoningDelta,
		events.TypeToolCallDeclared,
		events.TypeToolCallDelta,
		events.TypeToolExecStart,
		events.TypeToolExecOutput,
		events.TypeToolExecEnd,
		events.TypeExecutionDeclared,
		events.TypeExecutionStarted,
		events.TypeExecutionOutput,
		events.TypeExecutionBackgroundStarted,
		events.TypeExecutionBackgroundObserved,
		events.TypeExecutionBackgroundReady,
		events.TypeExecutionBackgroundExited,
		events.TypeExecutionBackgroundLost,
		events.TypeTurnWorkStateUpdated:
		return true
	default:
		return false
	}
}

func (m Model) handleWatchEvents(watchID int, batch []events.Event, closed bool) (tea.Model, tea.Cmd) {
	if watchID != m.watchID {
		return m, nil
	}
	if len(batch) == 0 {
		if closed {
			return m.handleWatchClosed()
		}
		return m, tea.Batch(waitForEventCmd(m.stream, m.watchID), m.ensureAnimTicking())
	}

	stateBefore := m.projector.CurrentState()
	resolutionInFlight := m.interactionResolutionInFlight()
	layoutBefore := viewportLayoutStateFor(m, stateBefore)
	focusBefore := m.chrome.focus
	var footerActivityCmd tea.Cmd
	var dialogRefreshCmd tea.Cmd
	var transcriptRefreshCmd tea.Cmd
	dialogTargets := captureWatchDialogTargets(m)
	refresh, err := m.applyWatchEventBatch(batch, dialogTargets)
	if err != nil {
		m.err = err
		m.cancelWatch()
		m.busy = false
		m.disarmLiveTurn()
		m.liveTurn.cancelRequested = false
		return m, nil
	}
	if refresh.refreshAvailableModels {
		_ = m.refreshDialogState()
	}
	stateAfter := m.projector.CurrentState()
	m.refreshTranscriptTurnSourceKeysForBatch(stateAfter, batch)
	m.trackRolloverContinuationTurn(stateBefore, stateAfter)
	m.trackPendingInteractionTurn(stateAfter)
	trackedTurnFinished := isFinishedInState(stateAfter, m.turnID)
	holdLiveTurnState := resolutionInFlight && trackedTurnFinished
	m.syncPendingResolutionState(stateAfter)
	if m.interaction.resolveReq == "" {
		m.interaction.cursor = 0
	}
	if trackedTurnFinished && !holdLiveTurnState {
		m.busy = false
		m.disarmLiveTurn()
		m.liveTurn.cancelRequested = false
	}
	switch activity := footerActivityBatchOutcome(batch); {
	case activity.show:
		footerActivityCmd = m.showFooterActivityKeyed(activity.text, activity.tone, activity.kind, activity.key)
	case activity.clear:
		m.clearFooterActivity()
	}
	layoutAfter := viewportLayoutStateFor(m, stateAfter)
	layoutChanged := layoutBefore != layoutAfter
	transcriptPlan := transcriptRefreshPlan{}
	if refresh.refreshTranscript && !layoutChanged {
		transcriptPlan = transcriptRefreshPlanForBatch(m, stateBefore, stateAfter, batch)
	}
	deferTranscriptRefresh := refresh.refreshTranscript && m.shouldDeferTranscriptRefreshPlan(transcriptPlan)
	throttleTranscriptRefresh := refresh.refreshTranscript && m.shouldThrottleLiveTranscriptBatch(batch)
	if layoutChanged {
		m.syncViewportLayout()
	} else {
		if refresh.refreshTranscript {
			if deferTranscriptRefresh {
				m.queueTranscriptRefreshPlan(transcriptPlan)
				m.transcriptRefresh.deferred = true
				m.transcriptRefresh.pending = false
			} else {
				m.transcriptRefresh.deferred = false
				if throttleTranscriptRefresh {
					now := time.Now()
					if delay := m.transcriptRefreshDelay(now); delay > 0 {
						m.queueTranscriptRefreshPlan(transcriptPlan)
						transcriptRefreshCmd = m.scheduleTranscriptRefresh(now, delay)
					} else {
						_ = m.applyTranscriptRefreshPlanWithState(stateAfter, transcriptPlan)
					}
				} else {
					_ = m.applyTranscriptRefreshPlanWithState(stateAfter, transcriptPlan)
				}
			}
		}
		if refresh.refreshInspector {
			m.syncInspectorBody(false)
		}
	}
	m.syncFocusState()
	switch {
	case refresh.refreshCostDialog:
		dialogRefreshCmd = m.requestDialogRefresh(dialogIDCost)
	case refresh.refreshTraceDialog:
		dialogRefreshCmd = m.requestDialogRefresh(dialogIDTrace)
	case refresh.refreshToolDetailDialog:
		dialogRefreshCmd = m.requestDialogRefresh(dialogIDToolDetail)
	case refresh.refreshTaskDetailDialog:
		dialogRefreshCmd = m.requestDialogRefresh(dialogIDTaskDetail)
	}
	trackedTurn := currentTurn(stateAfter, m.turnID)
	trackedTurnCompleted := trackedTurn != nil && trackedTurn.Status == events.TurnStatusCompleted
	if trackedTurnCompleted && !holdLiveTurnState && !hasPendingApprovalInState(stateAfter, m.turnID) {
		m.chrome.focus = focusComposer
		m.syncViewportLayout()
	}
	m.syncDeferredTranscriptIfNeeded()
	if closed {
		return m.handleWatchClosed()
	}
	cmds := []tea.Cmd{
		waitForEventCmd(m.stream, m.watchID),
		m.ensureAnimTicking(),
		m.ensureSelectedToolResultLoadedCmd(),
		m.ensureOpenToolMutationDetailLoadedCmd(stateAfter),
	}
	if transcriptRefreshCmd != nil {
		cmds = append(cmds, transcriptRefreshCmd)
	}
	if dialogRefreshCmd != nil {
		cmds = append(cmds, dialogRefreshCmd)
	}
	if focusBefore != m.chrome.focus {
		cmds = append(cmds, m.syncComposerFocus())
	}
	if refresh.refreshWorkspaceStatus {
		cmds = append(cmds, m.ensureWorkspaceStatusLoadedCmd())
	}
	if refresh.refreshBudgetStatus {
		cmds = append(cmds, loadBudgetStatusCmd(m.ctx, m.controller, m.sessionID))
	}
	if refresh.refreshSessionUsageSummary {
		cmds = append(cmds, loadSessionUsageSummaryCmd(m.ctx, m.controller, m.sessionID))
	}
	if shouldRefreshSnapshotForStaleExecutionState(stateAfter, m.turnID, batch) {
		cmds = append(cmds, refreshSessionSnapshotCmd(m.ctx, m.controller, m.sessionID))
	}
	if footerActivityCmd != nil {
		cmds = append(cmds, footerActivityCmd)
	}
	return m, tea.Batch(cmds...)
}

type watchDialogTargets struct {
	costOpen      bool
	traceOpen     bool
	traceTurnID   string
	toolDetailRef sessionToolCallRef
	taskDetailID  string
}

type watchBatchRefresh struct {
	refreshTranscript          bool
	refreshInspector           bool
	refreshWorkspaceStatus     bool
	refreshBudgetStatus        bool
	refreshSessionUsageSummary bool
	refreshAvailableModels     bool
	refreshCostDialog          bool
	refreshTraceDialog         bool
	refreshToolDetailDialog    bool
	refreshTaskDetailDialog    bool
}

func captureWatchDialogTargets(m Model) watchDialogTargets {
	targets := watchDialogTargets{}
	_, targets.costOpen = m.dialog.(*costDialog)
	if dialog, ok := m.dialog.(*traceDialog); ok {
		targets.traceOpen = true
		targets.traceTurnID = strings.TrimSpace(dialog.turnID)
	}
	if dialog, ok := m.dialog.(*toolDetailDialog); ok {
		targets.toolDetailRef = dialog.ref
	}
	if dialog, ok := m.dialog.(*taskDetailDialog); ok {
		targets.taskDetailID = strings.TrimSpace(dialog.taskID)
	}
	return targets
}

func (m *Model) applyWatchEventBatch(batch []events.Event, dialogTargets watchDialogTargets) (watchBatchRefresh, error) {
	refresh := watchBatchRefresh{}
	for _, event := range batch {
		lastSequence := m.projector.CurrentState().LastSequence
		if event.Sequence < lastSequence {
			continue
		}
		if !event.Ephemeral && event.Sequence == lastSequence {
			continue
		}
		if err := m.projector.Apply(event); err != nil {
			return refresh, err
		}
		refresh.refreshTranscript = refresh.refreshTranscript || shouldSyncTranscriptForEvent(event)
		refresh.refreshInspector = refresh.refreshInspector || shouldSyncInspectorForEvent(event)
		refresh.refreshWorkspaceStatus = refresh.refreshWorkspaceStatus || shouldRefreshWorkspaceStatusForEvent(event)
		refresh.refreshBudgetStatus = refresh.refreshBudgetStatus || event.Type == events.TypeTurnProviderUsageRecorded || event.Type == events.TypeTurnProviderUsageReported
		refresh.refreshSessionUsageSummary = refresh.refreshSessionUsageSummary ||
			event.Type == events.TypeTurnProviderUsageRecorded ||
			event.Type == events.TypeTurnProviderUsageReported ||
			event.Type == events.TypeToolExecEnd
		if dialogTargets.costOpen {
			refresh.refreshCostDialog = refresh.refreshCostDialog || shouldSyncCostDialogForEvent(event)
		}
		if dialogTargets.traceOpen {
			if dialogTargets.traceTurnID == "" {
				refresh.refreshTraceDialog = refresh.refreshTraceDialog || shouldSyncCostDialogForEvent(event)
			} else {
				refresh.refreshTraceDialog = refresh.refreshTraceDialog || shouldSyncTraceDialogForEvent(event, dialogTargets.traceTurnID)
			}
		}
		if dialogTargets.toolDetailRef.TurnID != "" && dialogTargets.toolDetailRef.CallID != "" {
			refresh.refreshToolDetailDialog = refresh.refreshToolDetailDialog || shouldSyncToolDetailDialogForEvent(event, dialogTargets.toolDetailRef)
		}
		if dialogTargets.taskDetailID != "" {
			refresh.refreshTaskDetailDialog = refresh.refreshTaskDetailDialog || shouldSyncTaskDetailDialogForEvent(event, dialogTargets.taskDetailID)
		}
		if event.Type == events.TypeSessionModelRouteUpdated || event.Type == events.TypeSessionMCPCatalogUpdated {
			refresh.refreshAvailableModels = true
		}
	}
	return refresh, nil
}

func (m *Model) syncPendingResolutionState(state events.SessionState) {
	if m.interactionResolutionInFlight() && isFinishedInState(state, m.turnID) {
		return
	}
	if pending := pendingExecutionFromState(state); pending != nil {
		if pending.RequestID != m.interaction.resolveReq {
			m.interaction.resolveReq = ""
		}
	} else if pending := pendingPermissionFromState(state); pending != nil {
		if pending.RequestID != m.interaction.resolveReq {
			m.interaction.resolveReq = ""
		}
	} else if pending := pendingQuestionFromState(state); pending == nil || pending.QuestionID != m.interaction.resolveReq {
		m.interaction.resolveReq = ""
	}
}

func (m Model) handleWatchClosed() (tea.Model, tea.Cmd) {
	if m.shuttingDown {
		m.cancelWatch()
		return m, tea.Quit
	}
	if m.isFinished() && !m.busy && m.shouldAutoQuit() {
		m.cancelWatch()
		return m, tea.Quit
	}
	m.err = ErrEventStreamClosed
	m.cancelWatch()
	m.busy = false
	m.disarmLiveTurn()
	m.liveTurn.cancelRequested = false
	return m, nil
}
