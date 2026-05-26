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
		return r.loadSessionTurnResult(ctx, sessionID, turnID, result)
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
	return failed, nil
}
