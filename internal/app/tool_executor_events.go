package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

type toolExecutionEventAppender interface {
	AppendToolExecStart(ctx context.Context, input ExecuteToolInput) error
	AppendToolExecEnd(ctx context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload, execErr error, output, errorText string, runtime *tool.ExecutionRuntime) error
}

type durableToolExecutionEventAppender struct {
	executor *ToolExecutor
}

func (a durableToolExecutionEventAppender) AppendToolExecStart(ctx context.Context, input ExecuteToolInput) error {
	return a.executor.appendToolExecStart(ctx, input)
}

func (a durableToolExecutionEventAppender) AppendToolExecEnd(ctx context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload, execErr error, output, errorText string, runtime *tool.ExecutionRuntime) error {
	return a.executor.appendToolExecEnd(ctx, input, payload, execErr, output, errorText, runtime)
}

type bufferedToolExecutionEventKind string

const (
	bufferedToolExecutionEventStart bufferedToolExecutionEventKind = "start"
	bufferedToolExecutionEventEnd   bufferedToolExecutionEventKind = "end"
)

type bufferedToolExecutionEvent struct {
	kind      bufferedToolExecutionEventKind
	input     ExecuteToolInput
	payload   events.ToolExecEndPayload
	execErr   error
	output    string
	errorText string
	runtime   *tool.ExecutionRuntime
}

type bufferedToolExecutionEventAppender struct {
	events []bufferedToolExecutionEvent
}

func (a *bufferedToolExecutionEventAppender) AppendToolExecStart(_ context.Context, input ExecuteToolInput) error {
	a.events = append(a.events, bufferedToolExecutionEvent{
		kind:  bufferedToolExecutionEventStart,
		input: cloneExecuteToolInput(input),
	})
	return nil
}

func (a *bufferedToolExecutionEventAppender) AppendToolExecEnd(_ context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload, execErr error, output, errorText string, runtime *tool.ExecutionRuntime) error {
	a.events = append(a.events, bufferedToolExecutionEvent{
		kind:      bufferedToolExecutionEventEnd,
		input:     cloneExecuteToolInput(input),
		payload:   cloneToolExecEndPayload(payload),
		execErr:   execErr,
		output:    output,
		errorText: errorText,
		runtime:   cloneToolExecutionRuntime(runtime),
	})
	return nil
}

func (a *bufferedToolExecutionEventAppender) Commit(ctx context.Context, executor *ToolExecutor) error {
	if a == nil {
		return nil
	}
	for _, event := range a.events {
		switch event.kind {
		case bufferedToolExecutionEventStart:
			if err := executor.appendToolExecStart(ctx, event.input); err != nil {
				return err
			}
		case bufferedToolExecutionEventEnd:
			if err := executor.appendToolExecEnd(ctx, event.input, event.payload, event.execErr, event.output, event.errorText, event.runtime); err != nil {
				return err
			}
		}
	}
	return nil
}

func cloneExecuteToolInput(input ExecuteToolInput) ExecuteToolInput {
	input.Arguments = append([]byte(nil), input.Arguments...)
	input.AllowedTools = append([]string(nil), input.AllowedTools...)
	input.TemporaryGrants = append([]workspace.Grant(nil), input.TemporaryGrants...)
	input.TemporaryNetworkTargets = append([]string(nil), input.TemporaryNetworkTargets...)
	return input
}

func cloneToolExecEndPayload(payload events.ToolExecEndPayload) events.ToolExecEndPayload {
	payload.ErrorDetail = cloneToolErrorDetail(payload.ErrorDetail)
	payload.StructuredResult = cloneStructuredResult(payload.StructuredResult)
	payload.MutationRanges = append([]events.MutationRange(nil), payload.MutationRanges...)
	if payload.WriteMutation != nil {
		mutation := *payload.WriteMutation
		if mutation.BeforeBlob != nil {
			blob := *mutation.BeforeBlob
			mutation.BeforeBlob = &blob
		}
		if mutation.DiffPreview != nil {
			preview := *mutation.DiffPreview
			preview.Ops = append([]textdiff.PreviewOp(nil), preview.Ops...)
			mutation.DiffPreview = &preview
		}
		payload.WriteMutation = &mutation
	}
	payload.WriteMutations = cloneWriteMutationPayloads(payload.WriteMutations)
	payload.ObservedResources = cloneObservedToolResources(payload.ObservedResources)
	if payload.OutputBlob != nil {
		blob := *payload.OutputBlob
		payload.OutputBlob = &blob
	}
	if payload.ErrorBlob != nil {
		blob := *payload.ErrorBlob
		payload.ErrorBlob = &blob
	}
	return payload
}

func cloneWriteMutationPayloads(mutations []events.WriteMutation) []events.WriteMutation {
	if len(mutations) == 0 {
		return nil
	}
	out := make([]events.WriteMutation, len(mutations))
	for idx, mutation := range mutations {
		if mutation.BeforeBlob != nil {
			blob := *mutation.BeforeBlob
			mutation.BeforeBlob = &blob
		}
		if mutation.DiffPreview != nil {
			preview := *mutation.DiffPreview
			preview.Ops = append([]textdiff.PreviewOp(nil), preview.Ops...)
			mutation.DiffPreview = &preview
		}
		out[idx] = mutation
	}
	return out
}

func cloneToolExecutionRuntime(runtime *tool.ExecutionRuntime) *tool.ExecutionRuntime {
	if runtime == nil {
		return nil
	}
	clone := *runtime
	return &clone
}

func (e *ToolExecutor) appendToolExecStart(ctx context.Context, input ExecuteToolInput) error {
	if _, err := e.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeToolExecStart,
		Payload: events.ToolExecStartPayload{
			CallID:   input.ToolCallID,
			ToolName: input.ToolName,
			ToolKind: string(inputToolKindOrDefault(input.ToolKind)),
			Input:    string(input.Arguments),
		},
	}); err != nil {
		return err
	}
	e.logger.Op("tool execution started",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
	)
	e.logger.Debug("tool execution input",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
		"arguments", string(input.Arguments),
	)
	return nil
}

func (e *ToolExecutor) appendToolExecEnd(ctx context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload, execErr error, output, errorText string, runtime *tool.ExecutionRuntime) error {
	if _, err := e.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeToolExecEnd,
		Payload:   payload,
	}); err != nil {
		return err
	}
	if execErr != nil || strings.TrimSpace(errorText) != "" {
		e.logger.Error("tool execution completed with error", execErr,
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
			"tool_call_id", input.ToolCallID,
			"tool_name", input.ToolName,
			"tool_error_bytes", len(errorText),
		)
	} else {
		e.logger.Op("tool execution completed",
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
			"tool_call_id", input.ToolCallID,
			"tool_name", input.ToolName,
		)
	}
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
	return nil
}
