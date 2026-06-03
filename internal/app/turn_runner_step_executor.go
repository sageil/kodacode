package app

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

type stepToolExecutor struct {
	tools                   *ToolExecutor
	allowedTools            []string
	modelVisibleInputs      []provider.Input
	temporaryGrants         []workspace.Grant
	temporaryNetworkTargets []string
	workflowBudget          workflowTurnBudget
}

func newStepToolExecutor(tools *ToolExecutor, allowedTools []string, modelVisibleInputs []provider.Input, temporaryGrants []workspace.Grant, temporaryNetworkTargets []string) stepToolExecutor {
	return stepToolExecutor{
		tools:                   tools,
		allowedTools:            append([]string(nil), allowedTools...),
		modelVisibleInputs:      cloneProviderInputs(modelVisibleInputs),
		temporaryGrants:         append([]workspace.Grant(nil), temporaryGrants...),
		temporaryNetworkTargets: append([]string(nil), temporaryNetworkTargets...),
	}
}

func (e stepToolExecutor) Execute(ctx context.Context, sessionID, turnID string, call stepToolCall) (stepToolResult, error) {
	result, err := e.execute(ctx, sessionID, turnID, call)
	if err != nil {
		return stepToolResult{}, err
	}
	return stepToolResultFromExecution(call, result), nil
}

func (e stepToolExecutor) execute(ctx context.Context, sessionID, turnID string, call stepToolCall) (ToolExecutionResult, error) {
	return e.tools.Execute(ctx, e.executeInput(sessionID, turnID, call))
}

func (e stepToolExecutor) executeBuffered(ctx context.Context, sessionID, turnID string, call stepToolCall) (bufferedToolExecution, error) {
	return e.tools.executeBuffered(ctx, e.executeInput(sessionID, turnID, call))
}

func (e stepToolExecutor) executeInput(sessionID, turnID string, call stepToolCall) ExecuteToolInput {
	return ExecuteToolInput{
		SessionID:               sessionID,
		TurnID:                  turnID,
		ToolCallID:              call.CallID,
		ToolName:                call.ToolName,
		ToolKind:                call.ToolKind,
		Arguments:               json.RawMessage(call.Arguments),
		ModelVisibleInputs:      cloneProviderInputs(e.modelVisibleInputs),
		AllowedTools:            slices.Clone(e.allowedTools),
		TemporaryGrants:         append([]workspace.Grant(nil), e.temporaryGrants...),
		TemporaryNetworkTargets: append([]string(nil), e.temporaryNetworkTargets...),
		WorkflowBudget:          e.workflowBudget,
	}
}

func stepToolResultFromExecution(call stepToolCall, result ToolExecutionResult) stepToolResult {
	return stepToolResult{
		CallID:              call.CallID,
		ToolName:            call.ToolName,
		CanonicalArguments:  result.CanonicalArguments,
		Output:              result.Output,
		Error:               result.Error,
		FailureClass:        result.FailureClass,
		TurnFailure:         result.TurnFailure,
		Status:              result.Status,
		PendingRequestID:    result.PendingRequestID,
		RetryOfCallID:       result.RetryOfCallID,
		ReusedFromCallID:    result.ReusedFromCallID,
		ReusedFromSessionID: result.ReusedFromSessionID,
		ReusedFromTurnID:    result.ReusedFromTurnID,
	}
}

func (e stepToolExecutor) ExecuteBatch(ctx context.Context, sessionID, turnID string, batch stepToolBatch) (stepToolBatchExecution, error) {
	schedule := scheduleStepToolBatchWithResolver(newStepToolCapabilityResolver(e.tools), batch)
	return e.ExecuteScheduledBatch(ctx, sessionID, turnID, schedule)
}

func (e stepToolExecutor) ExecuteScheduledBatch(ctx context.Context, sessionID, turnID string, schedule stepToolSchedule) (stepToolBatchExecution, error) {
	execution := stepToolBatchExecution{Schedule: schedule}
	if schedule.Executable.Len() == 0 {
		return execution, nil
	}
	if e.canExecuteBufferedBatchInParallel(schedule.Executable) {
		return e.executeBufferedBatchParallel(ctx, sessionID, turnID, execution, schedule.Executable)
	}
	return e.executeBufferedBatchSequential(ctx, sessionID, turnID, execution, schedule.Executable)
}

func (e stepToolExecutor) executeBufferedBatchSequential(ctx context.Context, sessionID, turnID string, execution stepToolBatchExecution, batch stepToolBatch) (stepToolBatchExecution, error) {
	results := make([]stepToolResult, 0, batch.Len())
	for _, call := range batch.Calls {
		result, err := e.Execute(ctx, sessionID, turnID, call)
		if err != nil {
			execution.Results = results
			return execution, err
		}
		results = append(results, result)
		if result.TurnFailure != nil || result.Status == ToolExecutionStatusPending {
			break
		}
	}
	execution.Results = results
	return execution, nil
}

func (e stepToolExecutor) executeBufferedBatchParallel(ctx context.Context, sessionID, turnID string, execution stepToolBatchExecution, batch stepToolBatch) (stepToolBatchExecution, error) {
	type parallelResult struct {
		index    int
		buffered bufferedToolExecution
		result   stepToolResult
		err      error
	}

	resultsCh := make(chan parallelResult, batch.Len())
	var wg sync.WaitGroup
	for idx, call := range batch.Calls {
		wg.Add(1)
		go func(idx int, call stepToolCall) {
			defer wg.Done()
			buffered, err := e.executeBuffered(ctx, sessionID, turnID, call)
			resultsCh <- parallelResult{
				index:    idx,
				buffered: buffered,
				result:   stepToolResultFromExecution(call, buffered.Result),
				err:      err,
			}
		}(idx, call)
	}
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	ordered := make([]parallelResult, batch.Len())
	received := make([]bool, batch.Len())
	for result := range resultsCh {
		ordered[result.index] = result
		received[result.index] = true
	}

	firstErrIndex := -1
	var firstErr error
	bufferedExecutions := make([]bufferedToolExecution, 0, batch.Len())
	for idx, result := range ordered {
		if !received[idx] {
			continue
		}
		bufferedExecutions = append(bufferedExecutions, result.buffered)
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			firstErrIndex = idx
		}
	}
	if err := e.commitBufferedExecutions(ctx, bufferedExecutions); err != nil {
		return execution, err
	}

	results := make([]stepToolResult, 0, batch.Len())
	for idx, result := range ordered {
		if !received[idx] {
			continue
		}
		if firstErrIndex >= 0 && idx >= firstErrIndex {
			break
		}
		results = append(results, result.result)
	}
	execution.Results = results
	if firstErr != nil {
		return execution, firstErr
	}
	return execution, nil
}

func (e stepToolExecutor) commitBufferedExecutions(ctx context.Context, executions []bufferedToolExecution) error {
	for _, buffered := range executions {
		if err := buffered.appender.Commit(ctx, e.tools); err != nil {
			return err
		}
	}
	return nil
}

func (e stepToolExecutor) canExecuteBufferedBatchInParallel(batch stepToolBatch) bool {
	if batch.Len() < 2 || e.tools == nil {
		return false
	}
	resolver := newStepToolCapabilityResolver(e.tools)
	for _, call := range batch.Calls {
		if !providerStepParallelReadToolCallWithResolver(resolver, call.ToolName, call.Arguments) {
			return false
		}
		tl, ok := e.tools.tools[call.ToolName]
		if !ok || toolUsesDirectDurableAppendPath(tl) {
			return false
		}
	}
	return true
}

func toolUsesDirectDurableAppendPath(tl tool.Tool) bool {
	if _, ok := tl.(tool.PathIntrospector); ok {
		return true
	}
	if _, ok := tl.(tool.NetworkIntrospector); ok {
		return true
	}
	if _, ok := tl.(tool.ExecutionRequestIntrospector); ok {
		return true
	}
	return false
}
