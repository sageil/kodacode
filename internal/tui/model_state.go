package tui

import (
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func (m Model) pendingExecution() *events.ExecutionApprovalState {
	return clonePendingExecutionApprovalState(pendingExecutionFromState(m.projector.CurrentState()))
}

func (m Model) pendingPermission() *events.PermissionRequestState {
	return clonePendingPermissionRequestState(pendingPermissionFromState(m.projector.CurrentState()))
}

func (m Model) pendingQuestion() *events.QuestionRequestState {
	return clonePendingQuestionRequestState(effectivePendingQuestionFromState(m.projector.CurrentState(), m.turnID))
}

func (m Model) hasPendingApproval() bool {
	return hasPendingApprovalInState(m.projector.CurrentState(), m.turnID)
}

func (m Model) hasPendingInteraction() bool {
	return hasPendingInteractionInState(m.projector.CurrentState(), m.turnID)
}

func (m Model) interactionResolutionInFlight() bool {
	return strings.TrimSpace(m.interaction.resolveReq) != "" || strings.TrimSpace(m.interaction.resolveHandoff) != ""
}

func (m Model) pendingInteractionSubmissionInFlight() bool {
	return m.pendingInteractionSubmissionInFlightForState(m.projector.CurrentState())
}

func (m Model) pendingInteractionSubmissionInFlightForState(state events.SessionState) bool {
	if !m.interactionResolutionInFlight() {
		return false
	}
	return hasPendingInteractionInState(state, m.turnID) || isFinishedInState(state, m.turnID)
}

func (m Model) pendingDelegatedPermission() *events.AgentHandoffState {
	return clonePendingDelegatedPermissionState(pendingDelegatedPermissionFromState(m.projector.CurrentState(), m.turnID))
}

func (m Model) pendingDelegatedQuestion() *events.AgentHandoffState {
	return clonePendingDelegatedPermissionState(pendingDelegatedQuestionFromState(m.projector.CurrentState(), m.turnID))
}

func (m Model) isFinished() bool {
	return isFinishedInState(m.projector.CurrentState(), m.turnID)
}

func (m Model) currentTurnRunning() bool {
	turnID := strings.TrimSpace(m.turnID)
	if turnID == "" {
		return false
	}
	if m.projector != nil {
		if turn := currentTurn(m.projector.CurrentState(), turnID); turn != nil {
			return turn.Status == events.TurnStatusRunning
		}
	}
	return m.liveTurn.spinnerArmed
}

func clonePendingExecutionApprovalState(state *events.ExecutionApprovalState) *events.ExecutionApprovalState {
	if state == nil {
		return nil
	}
	return &events.ExecutionApprovalState{
		RequestID:             state.RequestID,
		ExecutionID:           state.ExecutionID,
		TurnID:                state.TurnID,
		ToolCallID:            state.ToolCallID,
		ToolName:              state.ToolName,
		Command:               state.Command,
		WorkingDirectory:      state.WorkingDirectory,
		Reason:                state.Reason,
		PrefixRule:            append([]string(nil), state.PrefixRule...),
		SessionGrantPaths:     append([]string(nil), state.SessionGrantPaths...),
		NetworkTargets:        append([]string(nil), state.NetworkTargets...),
		AvailableDecisions:    append([]events.ExecutionApprovalDecision(nil), state.AvailableDecisions...),
		ProposedExecPolicy:    cloneExecutionPolicyAmendment(state.ProposedExecPolicy),
		ProposedNetworkPolicy: cloneExecutionNetworkPolicyAmendment(state.ProposedNetworkPolicy),
		RequestedAtSeq:        state.RequestedAtSeq,
	}
}

func clonePendingPermissionRequestState(state *events.PermissionRequestState) *events.PermissionRequestState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}

func clonePendingQuestionRequestState(state *events.QuestionRequestState) *events.QuestionRequestState {
	if state == nil {
		return nil
	}
	copyState := *state
	copyState.Options = append([]string(nil), state.Options...)
	return &copyState
}

func clonePendingDelegatedPermissionState(state *events.AgentHandoffState) *events.AgentHandoffState {
	if state == nil {
		return nil
	}
	copyState := *state
	copyState.AllowedTools = slices.Clone(state.AllowedTools)
	copyState.QuestionOptions = append([]string(nil), state.QuestionOptions...)
	copyState.ExecutionApproval = clonePendingExecutionApprovalState(state.ExecutionApproval)
	return &copyState
}

func cloneExecutionPolicyAmendment(state *events.ExecutionPolicyAmendment) *events.ExecutionPolicyAmendment {
	if state == nil {
		return nil
	}
	copyState := *state
	if state.AllowLoginShell != nil {
		value := *state.AllowLoginShell
		copyState.AllowLoginShell = &value
	}
	return &copyState
}

func cloneExecutionNetworkPolicyAmendment(state *events.ExecutionNetworkPolicyAmendment) *events.ExecutionNetworkPolicyAmendment {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}

func isTurnFinished(turn *events.TurnState) bool {
	if turn == nil {
		return false
	}
	return turn.Status == events.TurnStatusCompleted || turn.Status == events.TurnStatusCanceled || turn.Status == events.TurnStatusFailed
}

func pendingExecutionFromState(state events.SessionState) *events.ExecutionApprovalState {
	for _, requestID := range state.PendingExecutionOrder {
		request := state.PendingExecutions[requestID]
		if request != nil {
			return request
		}
	}
	return nil
}

func pendingPermissionFromState(state events.SessionState) *events.PermissionRequestState {
	for _, requestID := range state.PendingPermissionOrder {
		request := state.PendingPermissions[requestID]
		if request != nil {
			return request
		}
	}
	return nil
}

func pendingQuestionFromState(state events.SessionState) *events.QuestionRequestState {
	for _, requestID := range state.PendingQuestionOrder {
		request := state.PendingQuestions[requestID]
		if request != nil {
			return request
		}
	}
	return nil
}

func effectivePendingQuestionFromState(state events.SessionState, turnID string) *events.QuestionRequestState {
	if pending := pendingQuestionFromState(state); pending != nil {
		return pending
	}
	return delegatedQuestionFromHandoff(pendingDelegatedQuestionFromState(state, turnID), turnID)
}

func pendingLoopQuestionFromState(state events.SessionState) *events.QuestionRequestState {
	request := pendingQuestionFromState(state)
	if request == nil || strings.TrimSpace(request.Purpose) != events.QuestionPurposeTurnLoopResolution {
		return nil
	}
	return request
}

func pendingDelegatedPermissionFromState(state events.SessionState, turnID string) *events.AgentHandoffState {
	return pendingDelegatedPermissionHandoff(currentTurn(state, turnID))
}

func pendingDelegatedQuestionFromState(state events.SessionState, turnID string) *events.AgentHandoffState {
	return pendingDelegatedQuestionHandoff(currentTurn(state, turnID))
}

func pendingDelegatedInteractionFromState(state events.SessionState, turnID string) *events.AgentHandoffState {
	if handoff := pendingDelegatedPermissionFromState(state, turnID); handoff != nil {
		return handoff
	}
	return pendingDelegatedQuestionFromState(state, turnID)
}

func delegatedExecutionApproval(handoff *events.AgentHandoffState) *events.ExecutionApprovalState {
	if handoff == nil || handoff.PermissionKind != events.PermissionRequestKindExecution {
		return nil
	}
	return handoff.ExecutionApproval
}

func hasPendingApprovalInState(state events.SessionState, turnID string) bool {
	return pendingExecutionFromState(state) != nil || pendingPermissionFromState(state) != nil || pendingDelegatedPermissionFromState(state, turnID) != nil
}

func hasPendingInteractionInState(state events.SessionState, turnID string) bool {
	return hasPendingApprovalInState(state, turnID) || effectivePendingQuestionFromState(state, turnID) != nil
}

func delegatedQuestionFromHandoff(handoff *events.AgentHandoffState, turnID string) *events.QuestionRequestState {
	if handoff == nil {
		return nil
	}
	return &events.QuestionRequestState{
		QuestionID: strings.TrimSpace(handoff.HandoffID),
		TurnID:     strings.TrimSpace(turnID),
		ToolName:   strings.TrimSpace(handoff.QuestionToolName),
		Question:   strings.TrimSpace(handoff.QuestionText),
		Options:    append([]string(nil), handoff.QuestionOptions...),
	}
}

func isFinishedInState(state events.SessionState, turnID string) bool {
	return isTurnFinished(state.Turns[turnID])
}

func shouldRefreshSnapshotForStaleExecutionState(state events.SessionState, turnID string, batch []events.Event) bool {
	turn := currentTurn(state, turnID)
	if turn == nil || isTurnFinished(turn) {
		return false
	}
	maxDurableSeq := int64(-1)
	for _, event := range batch {
		if event.Ephemeral || event.TurnID != turnID {
			continue
		}
		if event.Sequence > maxDurableSeq {
			maxDurableSeq = event.Sequence
		}
	}
	if maxDurableSeq < 0 {
		return false
	}
	for _, callID := range orderedToolCallIDs(turn) {
		call := turn.ToolCalls[callID]
		if isStaleExecutingOneShotCall(call, maxDurableSeq) {
			return true
		}
	}
	return false
}

func isStaleExecutingOneShotCall(call *events.ToolCallState, maxDurableSeq int64) bool {
	if call == nil || !call.Executing || call.Completed || call.Execution == nil {
		return false
	}
	if call.Execution.Background != nil {
		return false
	}
	intent := strings.TrimSpace(call.Execution.Intent)
	if intent != "" && intent != "one_shot" {
		return false
	}
	return maxDurableSeq > call.LastUpdatedSeq
}
