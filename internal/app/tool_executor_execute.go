package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func (e *ToolExecutor) Execute(ctx context.Context, input ExecuteToolInput) (ToolExecutionResult, error) {
	return e.executeWithEventAppender(ctx, input, durableToolExecutionEventAppender{executor: e})
}

type bufferedToolExecution struct {
	Result   ToolExecutionResult
	appender bufferedToolExecutionEventAppender
}

func (e *ToolExecutor) executeBuffered(ctx context.Context, input ExecuteToolInput) (bufferedToolExecution, error) {
	appender := bufferedToolExecutionEventAppender{}
	result, err := e.executeWithEventAppender(ctx, input, &appender)
	return bufferedToolExecution{Result: result, appender: appender}, err
}

func (e *ToolExecutor) executeWithEventAppender(ctx context.Context, input ExecuteToolInput, appender toolExecutionEventAppender) (ToolExecutionResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return ToolExecutionResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return ToolExecutionResult{}, ErrTurnIDRequired
	}
	if strings.TrimSpace(input.ToolCallID) == "" {
		return ToolExecutionResult{}, ErrToolCallIDRequired
	}
	if strings.TrimSpace(input.ToolName) == "" {
		return ToolExecutionResult{}, ErrToolNameRequired
	}

	tl, ok := e.tools[input.ToolName]
	if !ok {
		return e.completeToolPreflightError(ctx, appender, input, fmt.Errorf("%w: %s", ErrToolNotFound, input.ToolName), "")
	}
	if !toolAllowed(input.ToolName, input.AllowedTools) {
		return e.completeToolPreflightError(ctx, appender, input, fmt.Errorf("%w: %s", ErrToolNotAllowed, input.ToolName), "")
	}

	state, err := e.sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	retryOfCallID := retryableLogicalToolRetryOfCall(state, input, e.tools)
	if normalized, err := e.normalizeToolArguments(state, input); err != nil {
		return e.completeToolPreflightError(ctx, appender, input, err, retryOfCallID)
	} else {
		input.Arguments = normalized
	}
	if err := enforcePlannerSavePlanQuestionShape(input); err != nil {
		return e.completeToolPreflightError(ctx, appender, input, err, retryOfCallID)
	}
	if err := enforcePlannerSavePlanQuestionVisible(state, input); err != nil {
		return e.completeToolPreflightError(ctx, appender, input, err, retryOfCallID)
	}
	if normalized, err := e.preparePlannerSavePlanQuestion(ctx, state, input); err != nil {
		return e.completeToolPreflightError(ctx, appender, input, err, retryOfCallID)
	} else {
		input = normalized
	}
	if result, handled, err := e.authorizeToolPaths(ctx, tl, state, &input); handled {
		if err != nil {
			return e.completeToolPreflightError(ctx, appender, input, err, retryOfCallID)
		}
		return result, nil
	}
	if result, handled, err := e.authorizeToolNetwork(ctx, tl, state, input); handled {
		if err != nil {
			return e.completeToolPreflightError(ctx, appender, input, err, retryOfCallID)
		}
		return result, nil
	}
	if result, handled, err := e.exec.Execute(ctx, tl, state, input); handled {
		if err != nil {
			if errors.Is(err, tool.ErrInvalidArguments) || errors.Is(err, ErrPermissionPolicyDenied) {
				return e.completeToolPreflightError(ctx, appender, input, err, retryOfCallID)
			}
		}
		return result, err
	}
	scope, err := scopeFromState(state, input.TemporaryGrants...)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if err := runTextMutationPreflightChecks(state, scope, input); err != nil {
		return e.completeToolPreflightError(ctx, appender, input, err, retryOfCallID)
	}
	execCtx := e.executionContext(ctx, state, input, scope)
	writeMutation, mutationArgs, err := captureTextMutation(scope, input)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if writeMutation != nil {
		execCtx.BeforeMutation = func(resolvedPath string) error {
			if !textMutationPathMatches(writeMutation, resolvedPath) {
				return nil
			}
			return captureTextMutationBefore(writeMutation)
		}
	}

	if appender == nil {
		appender = durableToolExecutionEventAppender{executor: e}
	}
	if err := appender.AppendToolExecStart(ctx, input); err != nil {
		return ToolExecutionResult{}, err
	}

	execCtx.OutputEmitter = e.toolOutputEmitter(ctx, input)
	result, execErr := tl.Execute(ctx, execCtx, input.Arguments)
	output := result.Output
	errorText := result.Error
	if execErr == nil && strings.TrimSpace(errorText) == "" {
		if approvalResult, handled, err := e.requestPlannerPlanApprovalForDelegateResult(ctx, input, output); handled || err != nil {
			return approvalResult, err
		}
	}
	handoffID := delegateHandoffIDFromToolResult(input.ToolName, output)
	pendingHandoffID := delegatePendingHandoffIDFromToolResult(input.ToolName, output)
	if execErr != nil {
		errorText = toolExecutionErrorText(input.ToolName, execErr)
	}
	errorDetail := toolExecutionErrorDetail(input, execErr, errorText)
	if strings.TrimSpace(result.PendingQuestionID) != "" {
		if execErr != nil || strings.TrimSpace(result.Output) != "" || strings.TrimSpace(result.Error) != "" || result.Execution != nil {
			return e.completeToolPostflightError(ctx, appender, input, "", nil, nil, ErrToolPendingQuestionConflict, retryOfCallID, handoffID)
		}
		e.logger.Op("tool execution pending question",
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
			"tool_call_id", input.ToolCallID,
			"tool_name", input.ToolName,
			"request_id", result.PendingQuestionID,
		)
		return ToolExecutionResult{
			Status:             ToolExecutionStatusPending,
			CanonicalArguments: string(input.Arguments),
			PendingRequestID:   result.PendingQuestionID,
		}, nil
	}
	if strings.TrimSpace(pendingHandoffID) != "" {
		if execErr != nil || strings.TrimSpace(errorText) != "" || result.Execution != nil {
			return e.completeToolPostflightError(ctx, appender, input, output, nil, result.Execution, ErrToolPendingQuestionConflict, retryOfCallID, handoffID)
		}
		e.logger.Op("tool execution pending delegated handoff",
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
			"tool_call_id", input.ToolCallID,
			"tool_name", input.ToolName,
			"handoff_id", pendingHandoffID,
		)
		return ToolExecutionResult{
			Status:             ToolExecutionStatusPending,
			CanonicalArguments: string(input.Arguments),
			PendingRequestID:   pendingHandoffID,
		}, nil
	}
	if err := validateMutationResultContract(input.ToolName, input.Arguments, output, errorText, result.MutationRanges); err != nil {
		return e.completeToolPostflightError(ctx, appender, input, output, result.ObservedResources, result.Execution, err, retryOfCallID, handoffID)
	}
	if execErr == nil && strings.TrimSpace(errorText) == "" {
		output = e.syncCodeIntelMutationAndAugmentOutput(ctx, state, scope, input, output)
	}
	if execErr == nil && strings.TrimSpace(errorText) == "" {
		mutationArgs = finalizeTextMutation(writeMutation, mutationArgs)
		result.ObservedResources = augmentTextMutationObservedResources(result.ObservedResources, writeMutation, mutationArgs, result.MutationRanges)
	}
	stored, err := prepareToolResultPayload(ctx, e.sessions.blobs, input.SessionID, input.TurnID, input.ToolCallID, input.ToolName, output, errorText)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if execErr != nil || strings.TrimSpace(errorText) != "" {
		writeMutation = nil
		mutationArgs = textMutationArguments{}
		result.TextMutations = nil
	}
	writeMutation, err = prepareTextMutationPayload(ctx, e.sessions.blobs, input.SessionID, input.TurnID, input.ToolCallID, writeMutation, mutationArgs)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	writeMutations, err := prepareToolTextMutationPayloads(ctx, e.sessions.blobs, input.SessionID, input.TurnID, input.ToolCallID, toolTextMutationPayloads(result.TextMutations))
	if err != nil {
		return ToolExecutionResult{}, err
	}

	endPayload := events.ToolExecEndPayload{
		Succeeded:         strings.TrimSpace(errorText) == "",
		CallID:            input.ToolCallID,
		ToolName:          input.ToolName,
		ToolKind:          string(inputToolKindOrDefault(input.ToolKind)),
		RetryOfCallID:     retryOfCallID,
		HandoffID:         handoffID,
		Output:            stored.Output,
		Error:             stored.Error,
		ErrorDetail:       errorDetail,
		StructuredResult:  cloneStructuredResult(result.StructuredResult),
		MutationRanges:    toolMutationRangesToEvents(result.MutationRanges),
		WriteMutation:     writeMutation,
		WriteMutations:    writeMutations,
		ObservedResources: toolObservedResourcesToEvents(result.ObservedResources),
		OutputBlob:        stored.OutputBlob,
		ErrorBlob:         stored.ErrorBlob,
		OutputTruncated:   stored.OutputTruncated,
		ErrorTruncated:    stored.ErrorTruncated,
		Backend:           toolExecutionBackend(result.Execution),
		FailureClass:      toolExecutionFailureClass(execErr, errorText),
	}
	if err := appender.AppendToolExecEnd(ctx, input, endPayload, execErr, output, errorText, result.Execution); err != nil {
		return ToolExecutionResult{}, err
	}

	return ToolExecutionResult{
		Status:             ToolExecutionStatusExecuted,
		CanonicalArguments: string(input.Arguments),
		Output:             output,
		Error:              errorText,
		ErrorDetail:        errorDetail,
		StructuredResult:   cloneStructuredResult(result.StructuredResult),
		FailureClass:       toolExecutionFailureClass(execErr, errorText),
		RetryOfCallID:      retryOfCallID,
	}, nil
}

func (e *ToolExecutor) normalizeToolArguments(state events.SessionState, input ExecuteToolInput) (json.RawMessage, error) {
	toolName := input.ToolName
	args := input.Arguments
	switch strings.TrimSpace(toolName) {
	case tool.ApplyPatchToolName:
		return normalizeApplyPatchToolArguments(args)
	case tool.SearchToolName:
		return tool.NormalizeSearchArguments(args, e.search)
	case tool.WebSearchToolName:
		return tool.NormalizeWebSearchArguments(args, e.webSearch)
	default:
		return args, nil
	}
}

func normalizeApplyPatchToolArguments(args json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(args))
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return args, nil
	}
	var input struct {
		Patch string `json:"patch"`
	}
	if err := tool.DecodeArgsStrict(tool.ApplyPatchToolName, args, &input, "patch"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Patch) == "" {
		return nil, tool.InvalidArguments(tool.ApplyPatchToolName, tool.ErrApplyPatchEmpty)
	}
	return json.RawMessage(input.Patch), nil
}

func toolMutationRangesToEvents(ranges []tool.MutationRange) []events.MutationRange {
	if len(ranges) == 0 {
		return nil
	}
	out := make([]events.MutationRange, len(ranges))
	for i, r := range ranges {
		out[i] = events.MutationRange{
			OldStartLine: r.OldStartLine,
			NewStartLine: r.NewStartLine,
		}
	}
	return out
}

func toolObservedResourcesToEvents(resources []tool.ObservedResource) []events.ObservedResource {
	if len(resources) == 0 {
		return nil
	}
	out := make([]events.ObservedResource, 0, len(resources))
	for _, resource := range resources {
		if strings.TrimSpace(string(resource.Kind)) == "" || strings.TrimSpace(resource.Path) == "" || strings.TrimSpace(resource.Version) == "" {
			continue
		}
		out = append(out, events.ObservedResource{
			Kind:       strings.TrimSpace(string(resource.Kind)),
			Path:       resource.Path,
			Version:    strings.TrimSpace(resource.Version),
			State:      strings.TrimSpace(resource.State),
			Complete:   resource.Complete,
			StartLine:  resource.StartLine,
			EndLine:    resource.EndLine,
			TotalLines: resource.TotalLines,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validateMutationResultContract(toolName string, args json.RawMessage, output, errorText string, ranges []tool.MutationRange) error {
	if strings.TrimSpace(errorText) != "" {
		return nil
	}
	return nil
}
