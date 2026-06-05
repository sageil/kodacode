package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func (r *Runtime) finalizeTurnRunResult(
	ctx context.Context,
	_ context.Context,
	_ *activeTurnHandle,
	sessionID, turnID string,
	result RunTurnResult,
) (RunSessionResult, error) {
	if result.Status != TurnRunStatusFailed {
		loaded, err := r.loadSessionTurnResult(ctx, sessionID, turnID, result)
		if err != nil {
			return RunSessionResult{}, err
		}
		r.triggerTurnPrecompute(ctx, sessionID, turnID, loaded.Status)
		return r.maybeAdvanceWorkflowBoundTurn(ctx, sessionID, turnID, loaded)
	}
	if err := r.applyTaskWorkflowFailurePolicyForTurn(ctx, sessionID, turnID); err != nil {
		return RunSessionResult{}, err
	}
	failed, ok, err := r.failedSessionTurnResult(ctx, sessionID, turnID)
	if err != nil {
		return RunSessionResult{}, err
	}
	if !ok {
		return RunSessionResult{}, fmt.Errorf("failed turn state missing for %s", turnID)
	}
	r.triggerTurnPrecompute(ctx, sessionID, turnID, failed.Status)
	return r.maybeAdvanceWorkflowBoundTurn(ctx, sessionID, turnID, failed)
}

func (r *Runtime) maybeAdvanceWorkflowBoundTurn(ctx context.Context, sessionID, turnID string, result RunSessionResult) (RunSessionResult, error) {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	if !turnBelongsToActiveWorkflow(state, turnID) {
		return result, nil
	}
	return r.maybeAdvanceWorkflowAfterTurn(ctx, sessionID, turnID, result)
}

func turnBelongsToActiveWorkflow(state events.SessionState, turnID string) bool {
	workflow := state.Workflow
	if workflow == nil || workflow.Status == events.WorkflowStatusCompleted {
		return false
	}
	turn := state.Turns[strings.TrimSpace(turnID)]
	if turn == nil || turn.Config == nil {
		return false
	}
	workflowID := strings.TrimSpace(turn.Config.WorkflowID)
	return workflowID != "" && workflowID == strings.TrimSpace(workflow.WorkflowID)
}
