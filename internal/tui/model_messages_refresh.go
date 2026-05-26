package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

func (m *Model) queueTranscriptRefreshPlan(plan transcriptRefreshPlan) {
	if m == nil {
		return
	}
	switch plan.kind {
	case transcriptRefreshNone:
		return
	case transcriptRefreshStructure:
		m.transcriptRefresh.plan = transcriptRefreshPlan{kind: transcriptRefreshStructure}
		return
	case transcriptRefreshTurns:
		if len(plan.turnIDs) == 0 {
			return
		}
		if m.transcriptRefresh.plan.kind == transcriptRefreshStructure {
			return
		}
		combined := make([]string, 0, len(m.transcriptRefresh.plan.turnIDs)+len(plan.turnIDs))
		combined = append(combined, m.transcriptRefresh.plan.turnIDs...)
		combined = append(combined, plan.turnIDs...)
		m.transcriptRefresh.plan = transcriptRefreshPlan{
			kind:    transcriptRefreshTurns,
			turnIDs: dedupeTranscriptTurnIDs(combined),
		}
	}
}

func (m *Model) flushQueuedTranscriptRefresh() {
	if m == nil {
		return
	}
	_ = m.applyTranscriptRefreshPlanWithState(m.projector.CurrentState(), m.transcriptRefresh.plan)
}

func dedupeTranscriptTurnIDs(turnIDs []string) []string {
	if len(turnIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(turnIDs))
	deduped := make([]string, 0, len(turnIDs))
	for _, turnID := range turnIDs {
		turnID = strings.TrimSpace(turnID)
		if turnID == "" {
			continue
		}
		if _, ok := seen[turnID]; ok {
			continue
		}
		seen[turnID] = struct{}{}
		deduped = append(deduped, turnID)
	}
	return deduped
}

func transcriptBatchTurnIDsForPartialRefresh(m Model, stateBefore, stateAfter events.SessionState, batch []events.Event) []string {
	if !sameOrderedTurnIDs(orderedSessionTurnIDs(stateBefore), orderedSessionTurnIDs(stateAfter)) {
		return nil
	}
	if hasDraftTranscriptSection(m, stateBefore) != hasDraftTranscriptSection(m, stateAfter) {
		return nil
	}
	if pendingDelegatedInteractionHandoffID(stateBefore, m.turnID) != pendingDelegatedInteractionHandoffID(stateAfter, m.turnID) {
		return nil
	}
	turnIDs := make([]string, 0, len(batch))
	for _, event := range batch {
		if !shouldSyncTranscriptForEvent(event) {
			continue
		}
		if event.Type == events.TypeSessionStateSnapshot {
			return nil
		}
		turnID := strings.TrimSpace(event.TurnID)
		if turnID == "" {
			return nil
		}
		turnIDs = append(turnIDs, turnID)
	}
	return expandTranscriptTurnIDsForHistoryCompactionSuppression(stateAfter, dedupeTranscriptTurnIDs(turnIDs))
}

func expandTranscriptTurnIDsForHistoryCompactionSuppression(state events.SessionState, turnIDs []string) []string {
	if len(turnIDs) == 0 {
		return nil
	}
	expanded := make([]string, 0, len(turnIDs)*2)
	expanded = append(expanded, turnIDs...)
	for _, turnID := range turnIDs {
		turn := state.Turns[strings.TrimSpace(turnID)]
		if turn == nil || turn.ContinuationStart == nil || turn.Continuation == nil {
			continue
		}
		previousTurnID := strings.TrimSpace(turn.ContinuationStart.PreviousTurnID)
		if previousTurnID != "" {
			expanded = append(expanded, previousTurnID)
		}
	}
	return dedupeTranscriptTurnIDs(expanded)
}

func transcriptRefreshPlanForBatch(m Model, stateBefore, stateAfter events.SessionState, batch []events.Event) transcriptRefreshPlan {
	turnIDs := transcriptBatchTurnIDsForPartialRefresh(m, stateBefore, stateAfter, batch)
	if len(turnIDs) == 0 {
		if hasTranscriptRefreshableEvents(batch) {
			return transcriptRefreshPlan{kind: transcriptRefreshStructure}
		}
		return transcriptRefreshPlan{}
	}
	if !m.canSyncTranscriptTurns(stateAfter, turnIDs...) {
		return transcriptRefreshPlan{kind: transcriptRefreshStructure}
	}
	return transcriptRefreshPlan{
		kind:    transcriptRefreshTurns,
		turnIDs: turnIDs,
	}
}

func hasTranscriptRefreshableEvents(batch []events.Event) bool {
	for _, event := range batch {
		if shouldSyncTranscriptForEvent(event) {
			return true
		}
	}
	return false
}

func sameOrderedTurnIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if strings.TrimSpace(left[idx]) != strings.TrimSpace(right[idx]) {
			return false
		}
	}
	return true
}

func hasDraftTranscriptSection(m Model, state events.SessionState) bool {
	if m.busy {
		return false
	}
	if strings.TrimSpace(m.userText) == "" {
		return false
	}
	turn := currentTurn(state, m.turnID)
	return turn == nil || strings.TrimSpace(turn.UserText) == ""
}

func pendingDelegatedInteractionHandoffID(state events.SessionState, turnID string) string {
	handoff := pendingDelegatedInteractionFromState(state, turnID)
	if handoff == nil {
		return ""
	}
	return strings.TrimSpace(handoff.HandoffID)
}

func (m Model) shouldDeferLiveTranscriptRefresh() bool {
	return m.busy && !m.messages.AtBottom()
}

func (m Model) shouldDeferTranscriptRefreshPlan(plan transcriptRefreshPlan) bool {
	if plan.kind == transcriptRefreshStructure {
		return false
	}
	return m.shouldDeferLiveTranscriptRefresh()
}

func (m Model) shouldThrottleLiveTranscriptBatch(batch []events.Event) bool {
	if !m.busy {
		return false
	}
	throttleable := false
	for _, event := range batch {
		if !shouldSyncTranscriptForEvent(event) {
			continue
		}
		if !shouldThrottleTranscriptRefreshForEvent(event) {
			return false
		}
		throttleable = true
	}
	return throttleable
}

func shouldThrottleTranscriptRefreshForEvent(event events.Event) bool {
	switch event.Type {
	case events.TypeAssistantPreviewDelta,
		events.TypeAssistantPreviewReset,
		events.TypeAssistantWorklogCommit,
		events.TypeReasoningDelta,
		events.TypeToolCallDeclared,
		events.TypeToolExecStart,
		events.TypeToolExecEnd,
		events.TypeExecutionDeclared,
		events.TypeExecutionStarted,
		events.TypeExecutionBackgroundStarted,
		events.TypeExecutionBackgroundReady,
		events.TypeExecutionBackgroundExited,
		events.TypeExecutionBackgroundLost,
		events.TypeAgentHandoff,
		events.TypeAgentResult,
		events.TypeAgentResultReused:
		return true
	default:
		return false
	}
}

func transcriptRefreshTickCmd(delay time.Duration) tea.Cmd {
	if delay < 0 {
		delay = 0
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return transcriptRefreshTickMsg{}
	})
}

func (m Model) transcriptRefreshDelay(now time.Time) time.Duration {
	if m.transcriptRefresh.lastAt.IsZero() {
		return 0
	}
	elapsed := now.Sub(m.transcriptRefresh.lastAt)
	if elapsed >= transcriptRefreshThrottle {
		return 0
	}
	return transcriptRefreshThrottle - elapsed
}

func (m *Model) scheduleTranscriptRefresh(now time.Time, delay time.Duration) tea.Cmd {
	if m == nil {
		return nil
	}
	m.transcriptRefresh.pending = true
	if m.transcriptRefresh.ticking {
		return nil
	}
	m.transcriptRefresh.ticking = true
	return transcriptRefreshTickCmd(delay)
}

func (m *Model) flushPendingTranscriptRefresh(now time.Time) tea.Cmd {
	if m == nil || !m.transcriptRefresh.pending {
		return nil
	}
	if m.shouldDeferTranscriptRefreshPlan(m.transcriptRefresh.plan) {
		m.transcriptRefresh.deferred = true
		m.transcriptRefresh.pending = false
		return nil
	}
	if delay := m.transcriptRefreshDelay(now); delay > 0 {
		return m.scheduleTranscriptRefresh(now, delay)
	}
	m.flushQueuedTranscriptRefresh()
	return nil
}

func (m *Model) syncDeferredTranscriptIfNeeded() {
	if m == nil || !m.transcriptRefresh.deferred || m.shouldDeferLiveTranscriptRefresh() {
		return
	}
	m.flushQueuedTranscriptRefresh()
}

func (m *Model) enterTranscriptScrollMode() tea.Cmd {
	if m == nil || !m.liveTurn.cancelRequested || m.chrome.focus == focusTranscript {
		return nil
	}
	m.chrome.focus = focusTranscript
	return m.syncComposerFocus()
}
