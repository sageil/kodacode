package app

import "context"

type stepToolBoundary struct {
	r                      *TurnRunner
	ctx                    context.Context
	sessionID              string
	turnID                 string
	state                  *turnLoopState
	batch                  *stepToolBatch
	committedToolBatchSize *int
	commitStepState        func()
}

type stepToolBoundaryInput struct {
	Runner                 *TurnRunner
	Context                context.Context
	SessionID              string
	TurnID                 string
	State                  *turnLoopState
	Batch                  *stepToolBatch
	CommittedToolBatchSize *int
	CommitStepState        func()
}

func newStepToolBoundary(input stepToolBoundaryInput) stepToolBoundary {
	return stepToolBoundary{
		r:                      input.Runner,
		ctx:                    input.Context,
		sessionID:              input.SessionID,
		turnID:                 input.TurnID,
		state:                  input.State,
		batch:                  input.Batch,
		committedToolBatchSize: input.CommittedToolBatchSize,
		commitStepState:        input.CommitStepState,
	}
}

func (b stepToolBoundary) Commit() error {
	if b.batch != nil && b.committedToolBatchSize != nil && b.batch.Len() >= 2 && b.batch.Len() > *b.committedToolBatchSize {
		callIDs, err := b.r.appendStepToolCallBatch(b.ctx, b.sessionID, b.turnID, *b.batch)
		if err != nil {
			return err
		}
		if b.state != nil {
			b.state.Conversation = normalizeToolCallBatch(b.state.Conversation, callIDs)
		}
		*b.committedToolBatchSize = b.batch.Len()
	}
	if b.commitStepState != nil {
		b.commitStepState()
	}
	return nil
}
