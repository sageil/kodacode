package app

import (
	"context"

	"github.com/sageil/kodacode/internal/provider"
)

type stepToolBatchRunInput struct {
	SessionID             string
	TurnID                string
	Model                 provider.ModelRef
	State                 *turnLoopState
	StepConversationStart *int
	Executor              stepToolExecutor
	CapabilityResolver    stepToolCapabilityResolver
	Progress              *stepToolProgress
	Batch                 stepToolBatch
	CommitStepState       func()
}

type stepToolBatchRunResult struct {
	Execution        stepToolBatchExecution
	DurableProgress  bool
	Failed           bool
	PendingRequestID string
}

func (r *TurnRunner) executeStepToolBatch(ctx context.Context, input stepToolBatchRunInput) (stepToolBatchRunResult, error) {
	resolver := input.CapabilityResolver
	if resolver == nil {
		resolver = newStepToolCapabilityResolver(input.Executor.tools)
	}
	executionSchedule := scheduleStepToolBatchWithResolver(resolver, input.Batch)
	executable := executionSchedule.Executable
	if executable.Len() == 0 {
		return stepToolBatchRunResult{Execution: stepToolBatchExecution{Schedule: executionSchedule}}, nil
	}

	for _, call := range executable.Calls {
		if err := r.appendStepToolCallDeclared(ctx, input.SessionID, input.TurnID, call); err != nil {
			return stepToolBatchRunResult{}, err
		}
		r.logToolDeclared(input.SessionID, input.TurnID, call.CallID, call.ToolName, call.Arguments)
	}
	if _, err := r.appendStepToolCallBatch(ctx, input.SessionID, input.TurnID, executable); err != nil {
		return stepToolBatchRunResult{}, err
	}
	if input.CommitStepState != nil {
		input.CommitStepState()
	}

	execution, execErr := input.Executor.ExecuteScheduledBatch(ctx, input.SessionID, input.TurnID, executionSchedule)
	runResult := stepToolBatchRunResult{
		Execution:       execution,
		DurableProgress: len(execution.Results) > 0,
	}

	for idx, result := range execution.Results {
		if idx >= len(execution.Schedule.Executable.Calls) {
			break
		}
		call := execution.Schedule.Executable.Calls[idx]
		if result.TurnFailure != nil {
			if input.Progress != nil {
				input.Progress.ExecutedTools++
				input.Progress.FailedTools++
			}
			if _, err := r.failTurn(ctx, input.SessionID, input.TurnID, provider.ModelRoute{Primary: input.Model}, result.TurnFailure); err != nil {
				return runResult, err
			}
			runResult.Failed = true
			if input.CommitStepState != nil {
				input.CommitStepState()
			}
			return runResult, nil
		}

		conversationUpdate := applyStepToolResultToConversation(input.State, input.StepConversationStart, call, result)
		if conversationUpdate.Pending {
			runResult.PendingRequestID = result.PendingRequestID
			if input.CommitStepState != nil {
				input.CommitStepState()
			}
			return runResult, nil
		}
		if input.Progress != nil {
			input.Progress.Record(r.tools, call, conversationUpdate.Arguments, result)
		}
		if input.CommitStepState != nil {
			input.CommitStepState()
		}
	}
	if input.State != nil {
		input.State.Conversation = normalizeToolCallBatch(input.State.Conversation, execution.Schedule.Executable.CallIDs())
	}
	if input.CommitStepState != nil {
		input.CommitStepState()
	}
	if execErr != nil {
		return runResult, execErr
	}
	return runResult, nil
}
