package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

func (m *Model) trackSessionTurnResult(result app.RunSessionResult) {
	if strings.TrimSpace(result.SessionID) != strings.TrimSpace(m.sessionID) {
		return
	}
	m.trackTurnID(m.projector.CurrentState(), result.TurnID)
}

func (m *Model) trackDelegatedQuestionResult(result app.AnswerDelegatedSessionQuestionResult) {
	if strings.TrimSpace(result.ChildTurn.SessionID) != strings.TrimSpace(m.sessionID) {
		return
	}
	m.trackTurnID(m.projector.CurrentState(), result.ChildTurn.TurnID)
}

func (m *Model) trackTurnID(state events.SessionState, turnID string) bool {
	turnID = strings.TrimSpace(turnID)
	currentTurnID := strings.TrimSpace(m.turnID)
	if turnID == "" || turnID == currentTurnID {
		return false
	}

	followDetailTurn := strings.TrimSpace(m.selection.detailTurnID) == "" || strings.TrimSpace(m.selection.detailTurnID) == currentTurnID
	m.turnID = turnID
	if followDetailTurn {
		m.selection.detailTurnID = turnID
	}
	m.userText = resolvedUserText(state, sessionView{TurnID: turnID, UserText: m.userText})
	m.agentID = resolvedAgentID(state, turnID, m.agentID)
	m.skillIDs = resolvedSkillIDs(state, turnID, m.skillIDs)
	m.thinkingEnabled = resolvedThinkingEnabled(state, turnID, m.thinkingEnabled)
	m.reasoningVariant = resolvedReasoningVariant(state, turnID, m.reasoningVariant)
	m.syncInspectorTabAvailability()
	if turn := currentTurn(state, turnID); turn != nil && turn.Status == events.TurnStatusRunning {
		m.armLiveTurn()
	}
	return true
}

func (m *Model) trackRolloverContinuationTurn(stateBefore, stateAfter events.SessionState) bool {
	currentTurnID := strings.TrimSpace(m.turnID)
	if currentTurnID == "" {
		return false
	}
	nextTurnID := latestContinuationDescendantTurnID(stateAfter, currentTurnID)
	if nextTurnID == "" || nextTurnID == currentTurnID {
		return false
	}
	previousBefore := currentTurn(stateBefore, currentTurnID)
	currentAfter := currentTurn(stateAfter, currentTurnID)
	if previousBefore == nil && currentAfter == nil {
		return false
	}
	if previousBefore != nil && previousBefore.Status == events.TurnStatusRunning {
		return m.trackRolloverTurnID(stateAfter, nextTurnID)
	}
	if currentAfter != nil && isTurnFinished(currentAfter) {
		return m.trackRolloverTurnID(stateAfter, nextTurnID)
	}
	return false
}

func (m *Model) trackPendingInteractionTurn(state events.SessionState) bool {
	nextTurnID := pendingInteractionTurnIDFromState(state)
	if nextTurnID == "" {
		return false
	}
	return m.trackTurnID(state, nextTurnID)
}

func pendingInteractionTurnIDFromState(state events.SessionState) string {
	if pending := pendingExecutionFromState(state); pending != nil {
		return strings.TrimSpace(pending.TurnID)
	}
	if pending := pendingPermissionFromState(state); pending != nil {
		return strings.TrimSpace(pending.TurnID)
	}
	if pending := pendingQuestionFromState(state); pending != nil {
		return strings.TrimSpace(pending.TurnID)
	}
	return ""
}

func latestContinuationChildTurnID(state events.SessionState, previousTurnID string) string {
	previousTurnID = strings.TrimSpace(previousTurnID)
	if previousTurnID == "" {
		return ""
	}
	for index := len(state.TurnOrder) - 1; index >= 0; index-- {
		turnID := strings.TrimSpace(state.TurnOrder[index])
		turn := state.Turns[turnID]
		if turn == nil || turn.ContinuationStart == nil {
			continue
		}
		if strings.TrimSpace(turn.ContinuationStart.PreviousTurnID) != previousTurnID {
			continue
		}
		return turnID
	}
	return ""
}

func latestContinuationDescendantTurnID(state events.SessionState, previousTurnID string) string {
	currentTurnID := strings.TrimSpace(previousTurnID)
	if currentTurnID == "" {
		return ""
	}
	for {
		nextTurnID := latestContinuationChildTurnID(state, currentTurnID)
		if nextTurnID == "" || nextTurnID == currentTurnID {
			return currentTurnID
		}
		currentTurnID = nextTurnID
	}
}

func (m *Model) trackRolloverTurnID(state events.SessionState, turnID string) bool {
	if !m.trackTurnID(state, turnID) {
		return false
	}
	if turn := currentTurn(state, turnID); turn != nil {
		if turn.ContinuationStart != nil &&
			strings.TrimSpace(turn.ContinuationStart.Reason) == events.TurnContinuationReasonContextLimit &&
			strings.TrimSpace(turn.UserText) == "" {
			m.userText = ""
		}
		if turn.Status == events.TurnStatusRunning {
			m.busy = true
		}
	}
	return true
}
