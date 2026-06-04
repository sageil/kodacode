package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func (e *ToolExecutor) completeToolPreflightError(ctx context.Context, appender toolExecutionEventAppender, input ExecuteToolInput, cause error, retryOfCallID string) (ToolExecutionResult, error) {
	if cause == nil {
		return ToolExecutionResult{}, nil
	}
	if appender == nil {
		appender = durableToolExecutionEventAppender{executor: e}
	}
	errorText := toolExecutionErrorText(input.ToolName, cause)
	errorDetail := toolExecutionErrorDetail(input, cause, errorText)
	payload := events.ToolExecEndPayload{
		CallID:        input.ToolCallID,
		ToolName:      input.ToolName,
		ToolKind:      string(inputToolKindOrDefault(input.ToolKind)),
		RetryOfCallID: retryOfCallID,
		Error:         errorText,
		ErrorDetail:   errorDetail,
		FailureClass:  toolFailureClassForCause(cause),
	}
	if err := appender.AppendToolExecEnd(ctx, input, payload, cause, "", errorText, nil); err != nil {
		return ToolExecutionResult{}, err
	}
	e.logger.Error("tool preflight failed", cause,
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
	)
	return ToolExecutionResult{
		Status:        ToolExecutionStatusExecuted,
		Error:         errorText,
		ErrorDetail:   errorDetail,
		FailureClass:  toolFailureClassForCause(cause),
		TurnFailure:   terminalTurnFailureForToolCause(cause),
		RetryOfCallID: retryOfCallID,
	}, nil
}

func (e *ToolExecutor) completeToolPostflightError(ctx context.Context, appender toolExecutionEventAppender, input ExecuteToolInput, output string, observed []tool.ObservedResource, runtime *tool.ExecutionRuntime, cause error, retryOfCallID string) (ToolExecutionResult, error) {
	if cause == nil {
		return ToolExecutionResult{}, nil
	}
	if appender == nil {
		appender = durableToolExecutionEventAppender{executor: e}
	}
	errorText := toolPostflightErrorText(input.ToolName, cause)
	errorDetail := toolExecutionErrorDetail(input, cause, errorText)
	stored, err := prepareToolResultPayload(ctx, e.sessions.blobs, input.SessionID, input.TurnID, input.ToolCallID, input.ToolName, output, errorText)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	payload := events.ToolExecEndPayload{
		CallID:            input.ToolCallID,
		ToolName:          input.ToolName,
		ToolKind:          string(inputToolKindOrDefault(input.ToolKind)),
		RetryOfCallID:     retryOfCallID,
		Output:            stored.Output,
		Error:             stored.Error,
		ErrorDetail:       errorDetail,
		ObservedResources: toolObservedResourcesToEvents(observed),
		OutputBlob:        stored.OutputBlob,
		ErrorBlob:         stored.ErrorBlob,
		OutputTruncated:   stored.OutputTruncated,
		ErrorTruncated:    stored.ErrorTruncated,
		Backend:           toolExecutionBackend(runtime),
		FailureClass:      toolFailureClassInvalidResult,
	}
	if err := appender.AppendToolExecEnd(ctx, input, payload, cause, output, errorText, runtime); err != nil {
		return ToolExecutionResult{}, err
	}
	e.logger.Error("tool postflight failed", cause,
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
	)
	e.logger.Debug("tool execution output",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
		"output", output,
		"error", errorText,
	)
	if runtime != nil {
		e.logger.Debug("tool execution runtime",
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
			"tool_call_id", input.ToolCallID,
			"tool_name", input.ToolName,
			"backend", runtime.Backend,
		)
	}
	return ToolExecutionResult{
		Status:        ToolExecutionStatusExecuted,
		Output:        output,
		Error:         errorText,
		ErrorDetail:   errorDetail,
		FailureClass:  toolFailureClassInvalidResult,
		RetryOfCallID: retryOfCallID,
	}, nil
}

func toolExecutionFailureClass(execErr error, errorText string) string {
	if execErr != nil {
		return toolFailureClassForCause(execErr)
	}
	if strings.TrimSpace(errorText) != "" {
		return toolFailureClassExecution
	}
	return ""
}

func toolFailureClassForCause(cause error) string {
	switch {
	case cause == nil:
		return ""
	case errors.Is(cause, tool.ErrInvalidArguments):
		return toolFailureClassInvalidArguments
	case errors.Is(cause, ErrToolNotFound):
		return toolFailureClassToolNotFound
	case errors.Is(cause, ErrToolNotAllowed):
		return toolFailureClassToolNotAllowed
	case errors.Is(cause, ErrToolPendingQuestionConflict):
		return toolFailureClassInvalidResult
	case errors.Is(cause, ErrPermissionPolicyDenied):
		return toolFailureClassPermissionDenied
	case errors.Is(cause, ErrWriteRequiresRead),
		errors.Is(cause, ErrWriteRequiresCompleteRead),
		errors.Is(cause, ErrWriteRequiresFreshRead),
		errors.Is(cause, ErrPlannerSavePlanQuestionRequiresVisiblePlan),
		errors.Is(cause, ErrPlannerSavePlanQuestionInvalid):
		return toolFailureClassContract
	default:
		message := strings.TrimSpace(cause.Error())
		switch {
		case strings.Contains(message, "must be read before it can be written"):
			return toolFailureClassContract
		default:
			return toolFailureClassExecution
		}
	}
}

func terminalTurnFailureForToolCause(cause error) error {
	switch {
	case errors.Is(cause, ErrPlannerSavePlanQuestionRequiresVisiblePlan):
		return ErrPlannerSavePlanQuestionRequiresVisiblePlan
	case errors.Is(cause, ErrPlannerSavePlanQuestionInvalid):
		return ErrPlannerSavePlanQuestionInvalid
	default:
		return nil
	}
}

func toolPostflightErrorText(toolName string, cause error) string {
	if cause == nil {
		return ""
	}
	if errors.Is(cause, ErrToolPendingQuestionConflict) {
		return fmt.Sprintf("runtime rejected `%s`: invalid pending question payload.", toolName)
	}
	return fmt.Sprintf("runtime rejected `%s`: %s", toolName, strings.TrimSpace(cause.Error()))
}

func toolExecutionBackend(runtime *tool.ExecutionRuntime) string {
	if runtime == nil {
		return ""
	}
	return runtime.Backend
}

func toolAllowed(name string, allowed []string) bool {
	if allowed == nil {
		return true
	}
	return tool.PolicyListContainsTool(allowed, name)
}
