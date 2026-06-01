package app

import (
	"context"
	"fmt"
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
		return loaded, nil
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
	return failed, nil
}
