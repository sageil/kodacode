package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
)

func pendingRunResult(state events.SessionState, sessionID string) (RunSessionResult, bool) {
	if len(state.PendingPermissionOrder) == 0 {
		if len(state.PendingExecutionOrder) == 0 && len(state.PendingQuestionOrder) == 0 {
			return RunSessionResult{}, false
		}
	}
	requestID := ""
	if len(state.PendingExecutionOrder) > 0 {
		requestID = state.PendingExecutionOrder[0]
	} else if len(state.PendingPermissionOrder) > 0 {
		requestID = state.PendingPermissionOrder[0]
	} else {
		requestID = state.PendingQuestionOrder[0]
	}
	pending := pendingPermissionRequestState(state, requestID)
	if pendingExecution := pendingExecutionApprovalState(state, requestID); pendingExecution != nil {
		turn := state.Turns[pendingExecution.TurnID]
		result := RunSessionResult{
			SessionID:        sessionID,
			TurnID:           pendingExecution.TurnID,
			Status:           TurnRunStatusPending,
			PendingRequestID: requestID,
		}
		if turn != nil {
			result.UserText = turn.UserText
			result.AssistantText = turn.AssistantText
		}
		copyPending := *pendingExecution
		result.PendingExecution = &copyPending
		return result, true
	}
	if pending == nil {
		pendingQuestion := pendingQuestionRequestState(state, requestID)
		if pendingQuestion == nil {
			return RunSessionResult{}, false
		}
		turn := state.Turns[pendingQuestion.TurnID]
		result := RunSessionResult{
			SessionID:        sessionID,
			TurnID:           pendingQuestion.TurnID,
			Status:           TurnRunStatusPending,
			PendingRequestID: requestID,
		}
		if turn != nil {
			result.UserText = turn.UserText
			result.AssistantText = turn.AssistantText
		}
		copyPending := *pendingQuestion
		result.PendingQuestion = &copyPending
		return result, true
	}
	turn := state.Turns[pending.TurnID]
	result := RunSessionResult{
		SessionID:        sessionID,
		TurnID:           pending.TurnID,
		Status:           TurnRunStatusPending,
		PendingRequestID: requestID,
	}
	if turn != nil {
		result.UserText = turn.UserText
		result.AssistantText = turn.AssistantText
	}
	copyPending := *pending
	result.PendingPermission = &copyPending
	return result, true
}

func resumePendingTurn(ctx context.Context, in CommandInput, runtime *Runtime, sessionID string) (RunSessionResult, bool, error) {
	state, err := runtime.SnapshotSession(ctx, sessionID)
	if err != nil {
		return RunSessionResult{}, false, err
	}
	result, ok := pendingRunResult(state, sessionID)
	if !ok {
		return RunSessionResult{}, false, nil
	}
	if in.UserText != "" {
		return RunSessionResult{}, true, ErrResumePendingTurnFirst
	}
	return result, true, nil
}
