package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func executionToolExecEndPayload(ctx context.Context, blobs ToolResultBlobStore, request tool.ExecutionRequest, input ExecuteToolInput, status events.ExecutionStatus, output, errorText string, runtime *tool.ExecutionRuntime) (events.ToolExecEndPayload, error) {
	stored, err := prepareToolResultPayload(ctx, blobs, input.SessionID, input.TurnID, input.ToolCallID, input.ToolName, output, errorText)
	if err != nil {
		return events.ToolExecEndPayload{}, err
	}
	payload := events.ToolExecEndPayload{
		CallID:          input.ToolCallID,
		ToolName:        input.ToolName,
		ToolKind:        string(inputToolKindOrDefault(input.ToolKind)),
		ExecutionID:     executionID(input.ToolCallID),
		ExecutionStatus: string(status),
		FailureClass:    executionToolFailureClass(status, errorText),
		Succeeded:       status == events.ExecutionStatusCompleted && strings.TrimSpace(errorText) == "",
		Output:          stored.Output,
		Error:           stored.Error,
		ErrorDetail:     toolExecutionErrorDetail(input, nil, errorText),
		OutputBlob:      stored.OutputBlob,
		ErrorBlob:       stored.ErrorBlob,
		OutputTruncated: stored.OutputTruncated,
		ErrorTruncated:  stored.ErrorTruncated,
		CommandActions:  executionCommandActions(request, status),
	}
	if runtime != nil {
		payload.Backend = runtime.Backend
		payload.ExitCode = cloneExecutionRuntimeExitCode(runtime.ExitCode)
		payload.DurationMS = runtime.DurationMS
	}
	return payload, nil
}

func executionToolFailureClass(status events.ExecutionStatus, errorText string) string {
	if status == events.ExecutionStatusCompleted || strings.TrimSpace(errorText) == "" {
		return ""
	}
	return toolFailureClassExecution
}

func (s *ExecutionService) appendExecutionToolEnd(
	ctx context.Context,
	input ExecuteToolInput,
	completed events.ToolExecEndPayload,
) error {
	_, err := s.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeToolExecEnd,
		Payload:   completed,
	})
	return err
}

func executionRuntimeFromRunResult(result executionRunResult) *tool.ExecutionRuntime {
	if result.Backend == "" {
		return nil
	}
	return &tool.ExecutionRuntime{
		Backend:    result.Backend,
		ExitCode:   cloneExecutionRuntimeExitCode(result.ExitCode),
		DurationMS: result.DurationMS,
	}
}

func executionStatusFromRunError(runErr error) events.ExecutionStatus {
	if runErr != nil {
		return events.ExecutionStatusFailed
	}
	return events.ExecutionStatusCompleted
}

func executionCommandActions(request tool.ExecutionRequest, status events.ExecutionStatus) []string {
	actions := make([]string, 0, 4)
	if strings.TrimSpace(request.ShellCommand) != "" {
		actions = append(actions, "shell")
	} else {
		actions = append(actions, "exec")
	}
	if executionRunsInBackground(request.Intent) {
		actions = append(actions, "background")
	}
	if status == events.ExecutionStatusFailed {
		actions = append(actions, "error")
	}
	return actions
}

func cloneExecutionRuntimeExitCode(code *int) *int {
	if code == nil {
		return nil
	}
	copyCode := *code
	return &copyCode
}

func formatExecutionResult(request tool.ExecutionRequest, output string, truncated bool, runErr error) string {
	body := strings.TrimSpace(output)
	if truncated {
		if body != "" {
			body += "\n\n[output truncated]"
		} else {
			body = "[output truncated]"
		}
	}
	if body == "" && runErr != nil {
		body = executionFailureSummary(request, runErr)
	} else if body != "" && runErr != nil {
		body += "\n\n[" + executionFailureSummary(request, runErr) + "]"
	}
	if body == "" {
		return "(no output)"
	}
	return body
}

func executionFailureSummary(request tool.ExecutionRequest, runErr error) string {
	if runErr == nil {
		return ""
	}
	switch {
	case errors.Is(runErr, context.DeadlineExceeded):
		return fmt.Sprintf("command timed out after %s", executionTimeoutLabel(request.TimeoutMS))
	case errors.Is(runErr, context.Canceled):
		return "command was canceled"
	default:
		return strings.TrimSpace(runErr.Error())
	}
}

func executionTimeoutLabel(timeoutMS int) string {
	effective := timeoutMS
	if effective <= 0 {
		effective = int(executionCommandTimeout / time.Millisecond)
	}
	return (time.Duration(effective) * time.Millisecond).String()
}
