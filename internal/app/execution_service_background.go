package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func executionRunsInBackground(intent tool.ExecutionIntent) bool {
	switch intent {
	case tool.ExecutionIntentServer, tool.ExecutionIntentWatcher:
		return true
	default:
		return false
	}
}

func (s *ExecutionService) executeBackgroundIntent(ctx context.Context, input ExecuteToolInput, resolved resolvedExecutionRequest) (ToolExecutionResult, error) {
	if s.backgroundLogs == nil {
		return ToolExecutionResult{}, fmt.Errorf("background log store is required")
	}
	if _, err := s.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeExecutionStarted,
		Payload: events.ExecutionStartedPayload{
			ExecutionID: executionID(input.ToolCallID),
			ToolCallID:  input.ToolCallID,
			ToolName:    input.ToolName,
			Input:       string(input.Arguments),
		},
	}); err != nil {
		return ToolExecutionResult{}, err
	}

	logHandle, err := s.backgroundLogs.Create(ctx, BackgroundExecutionLogKey{
		SessionID:   input.SessionID,
		TurnID:      input.TurnID,
		ExecutionID: executionID(input.ToolCallID),
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	observer := newBackgroundExecutionObserver(s.sessions, input)

	handle, err := startBackgroundExecutionCommand(ctx, resolved.Contract, executionBackgroundRunOptions{
		StdoutStream:  "combined",
		StderrStream:  "combined",
		ReadyPatterns: append([]string(nil), resolved.Request.ReadyPatterns...),
		Emit:          observer.Emit,
		LogWriter:     logHandle.Writer,
	})
	if err != nil {
		if logHandle.Writer != nil {
			_ = logHandle.Writer.Close()
		}
		return s.completeBackgroundExecution(ctx, input, resolved.Request, executionRunResult{}, err)
	}

	if _, err := s.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeExecutionBackgroundStarted,
		Payload: events.ExecutionBackgroundStartedPayload{
			ExecutionID:     executionID(input.ToolCallID),
			ToolCallID:      input.ToolCallID,
			ToolName:        input.ToolName,
			PID:             handle.PID,
			ProcessIdentity: handle.ProcessIdentity,
			SupervisorID:    s.instanceID,
			LogRef:          logHandle.Ref,
			ReadyPatterns:   append([]string(nil), resolved.Request.ReadyPatterns...),
		},
	}); err != nil {
		return ToolExecutionResult{}, err
	}

	monitorCtx, started := s.startBackgroundRun(executionID(input.ToolCallID))
	if !started {
		return ToolExecutionResult{}, fmt.Errorf("background execution %s is already tracked", executionID(input.ToolCallID))
	}

	if len(resolved.Request.ReadyPatterns) == 0 || handle.Ready == nil {
		go s.monitorBackgroundExecution(monitorCtx, input, false, handle, observer)
		return s.completeBackgroundExecution(ctx, input, resolved.Request, executionRunResult{
			Backend: "background_process",
		}, nil, formatBackgroundExecutionMessage(resolved.Request, handle.PID, nil))
	}

	select {
	case ready, ok := <-handle.Ready:
		if ok {
			if err := s.appendBackgroundReadyEventWithFinalize(ctx, input, ready, observer); err != nil {
				s.finishBackgroundRun(executionID(input.ToolCallID))
				return ToolExecutionResult{}, err
			}
			go s.monitorBackgroundExecution(monitorCtx, input, true, handle, observer)
			return s.completeBackgroundExecution(ctx, input, resolved.Request, executionRunResult{
				Backend: "background_process",
			}, nil, formatBackgroundExecutionMessage(resolved.Request, handle.PID, &ready))
		}
	case exited, ok := <-handle.Exited:
		if ok {
			s.finishBackgroundRun(executionID(input.ToolCallID))
			if err := s.appendBackgroundExitedEventWithFinalize(ctx, input, exited, observer); err != nil {
				return ToolExecutionResult{}, err
			}
			return s.completeBackgroundExecution(ctx, input, resolved.Request, exited.RunResult, exited.Err)
		}
	case <-time.After(executionBackgroundStartupTimeout):
	}

	go s.monitorBackgroundExecution(monitorCtx, input, false, handle, observer)
	return s.completeBackgroundExecution(ctx, input, resolved.Request, executionRunResult{
		Backend: "background_process",
	}, nil, formatBackgroundExecutionMessage(resolved.Request, handle.PID, nil))
}

func (s *ExecutionService) monitorBackgroundExecution(ctx context.Context, input ExecuteToolInput, readyHandled bool, handle executionBackgroundHandle, observer *backgroundExecutionObserver) {
	defer s.finishBackgroundRun(executionID(input.ToolCallID))
	readyCh := handle.Ready
	if readyHandled {
		readyCh = nil
	}
	exitCh := handle.Exited
	for readyCh != nil || exitCh != nil {
		select {
		case <-ctx.Done():
			if err := s.flushBackgroundObserverWithFinalize(ctx, observer); err != nil {
				s.logBackgroundEventAppendFailure(input, events.TypeExecutionBackgroundObserved, err)
			}
			return
		case ready, ok := <-readyCh:
			if !ok {
				readyCh = nil
				continue
			}
			if err := s.appendBackgroundReadyEventWithFinalize(ctx, input, ready, observer); err != nil {
				s.logBackgroundEventAppendFailure(input, events.TypeExecutionBackgroundReady, err)
			}
			readyCh = nil
		case exited, ok := <-exitCh:
			if !ok {
				if err := s.flushBackgroundObserverWithFinalize(ctx, observer); err != nil {
					s.logBackgroundEventAppendFailure(input, events.TypeExecutionBackgroundObserved, err)
				}
				return
			}
			if err := s.appendBackgroundExitedEventWithFinalize(ctx, input, exited, observer); err != nil {
				s.logBackgroundEventAppendFailure(input, events.TypeExecutionBackgroundExited, err)
			}
			return
		}
	}
}

// Background supervision runs under a detached monitor context, so final durable
// writes use a bounded finalize context instead of inheriting cancellation.
func (s *ExecutionService) flushBackgroundObserverWithFinalize(ctx context.Context, observer *backgroundExecutionObserver) error {
	if observer == nil {
		return nil
	}
	finalizeCtx, cancel := executionFinalizeContext(ctx)
	defer cancel()
	return observer.Flush(finalizeCtx)
}

func (s *ExecutionService) appendBackgroundReadyEventWithFinalize(ctx context.Context, input ExecuteToolInput, ready executionBackgroundReadyEvent, observer *backgroundExecutionObserver) error {
	finalizeCtx, cancel := executionFinalizeContext(ctx)
	defer cancel()
	return s.appendBackgroundReadyEvent(finalizeCtx, input, ready, observer)
}

func (s *ExecutionService) appendBackgroundExitedEventWithFinalize(ctx context.Context, input ExecuteToolInput, exited executionBackgroundExitEvent, observer *backgroundExecutionObserver) error {
	finalizeCtx, cancel := executionFinalizeContext(ctx)
	defer cancel()
	return s.appendBackgroundExitedEvent(finalizeCtx, input, exited, observer)
}

func (s *ExecutionService) logBackgroundEventAppendFailure(input ExecuteToolInput, eventType events.Type, err error) {
	if s == nil || s.logger == nil || err == nil {
		return
	}
	s.logger.Error("background execution event append failed", err,
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
		"execution_id", executionID(input.ToolCallID),
		"event_type", eventType,
	)
}

func (s *ExecutionService) appendBackgroundReadyEvent(ctx context.Context, input ExecuteToolInput, ready executionBackgroundReadyEvent, observer *backgroundExecutionObserver) error {
	if strings.TrimSpace(ready.Message) == "" {
		return nil
	}
	if observer != nil {
		if err := observer.Flush(ctx); err != nil {
			return err
		}
	}
	_, err := s.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeExecutionBackgroundReady,
		Payload: events.ExecutionBackgroundReadyPayload{
			ExecutionID: executionID(input.ToolCallID),
			ToolCallID:  input.ToolCallID,
			ToolName:    input.ToolName,
			Message:     ready.Message,
			Port:        ready.Port,
		},
	})
	return err
}

func (s *ExecutionService) appendBackgroundExitedEvent(ctx context.Context, input ExecuteToolInput, exited executionBackgroundExitEvent, observer *backgroundExecutionObserver) error {
	if observer != nil {
		if err := observer.Flush(ctx); err != nil {
			return err
		}
	}
	_, err := s.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeExecutionBackgroundExited,
		Payload: events.ExecutionBackgroundExitedPayload{
			ExecutionID: executionID(input.ToolCallID),
			ToolCallID:  input.ToolCallID,
			ToolName:    input.ToolName,
			ExitCode:    cloneExecutionRuntimeExitCode(exited.RunResult.ExitCode),
			Error:       backgroundExitSummary(exited),
		},
	})
	return err
}

func (s *ExecutionService) completeBackgroundExecution(
	ctx context.Context,
	input ExecuteToolInput,
	request tool.ExecutionRequest,
	runResult executionRunResult,
	runErr error,
	overrideOutput ...string,
) (ToolExecutionResult, error) {
	status := executionStatusFromRunError(runErr)
	output := formatExecutionResult(request, string(runResult.Output), runResult.Truncated, runErr)
	errorText := ""
	if len(overrideOutput) > 0 && strings.TrimSpace(overrideOutput[0]) != "" {
		output = strings.TrimSpace(overrideOutput[0])
		runErr = nil
		status = events.ExecutionStatusCompleted
	}
	if runErr != nil {
		output = ""
		errorText = formatExecutionResult(request, string(runResult.Output), runResult.Truncated, runErr)
	}

	finalizeCtx, cancelFinalize := executionFinalizeContext(ctx)
	defer cancelFinalize()

	payload, err := executionToolExecEndPayload(finalizeCtx, s.sessions.blobs, request, input, status, output, errorText, executionRuntimeFromRunResult(runResult))
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if err := s.appendExecutionToolEnd(finalizeCtx, input, payload); err != nil {
		return ToolExecutionResult{}, err
	}
	return ToolExecutionResult{
		Status:      ToolExecutionStatusExecuted,
		Output:      output,
		Error:       errorText,
		ErrorDetail: payload.ErrorDetail,
	}, nil
}

func formatBackgroundExecutionMessage(request tool.ExecutionRequest, pid int, ready *executionBackgroundReadyEvent) string {
	switch request.Intent {
	case tool.ExecutionIntentWatcher:
		return fmt.Sprintf("Started watch process in background (pid %d).", pid)
	case tool.ExecutionIntentServer:
		if ready == nil {
			return fmt.Sprintf("Started server in background (pid %d). Readiness not yet confirmed.", pid)
		}
		message := fmt.Sprintf("Started server in background (pid %d).", pid)
		if strings.TrimSpace(ready.Message) != "" {
			message += "\nReady: " + strings.TrimSpace(ready.Message)
		}
		return message
	default:
		return fmt.Sprintf("Started background execution (pid %d).", pid)
	}
}

func backgroundExitSummary(exited executionBackgroundExitEvent) string {
	if exited.Err != nil {
		return strings.TrimSpace(formatExecutionResult(tool.ExecutionRequest{}, string(exited.RunResult.Output), exited.RunResult.Truncated, exited.Err))
	}
	return ""
}

func (s *ExecutionService) startBackgroundRun(executionID string) (context.Context, bool) {
	s.backgroundMu.Lock()
	defer s.backgroundMu.Unlock()
	if _, ok := s.backgroundRuns[executionID]; ok {
		return nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.backgroundRuns[executionID] = cancel
	return ctx, true
}

func (s *ExecutionService) finishBackgroundRun(executionID string) {
	s.backgroundMu.Lock()
	cancel := s.backgroundRuns[executionID]
	delete(s.backgroundRuns, executionID)
	s.backgroundMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
