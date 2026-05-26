package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

const backgroundResumePollInterval = time.Second

type resumableBackgroundExecution struct {
	SessionID       string
	TurnID          string
	ExecutionID     string
	ToolCallID      string
	ToolName        string
	PID             int
	ProcessIdentity string
	LogRef          string
	ReadyPatterns   []string
	Ready           bool
	OutputTail      string
	OutputBytes     int64
}

func (s *ExecutionService) ResumeBackgroundExecutions(ctx context.Context, sessionID string) error {
	if s == nil {
		return nil
	}
	state, err := s.sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, candidate := range resumableBackgroundExecutionsFromState(state) {
		if err := s.resumeBackgroundExecution(ctx, candidate); err != nil {
			return err
		}
	}
	return nil
}

func resumableBackgroundExecutionsFromState(state events.SessionState) []resumableBackgroundExecution {
	var out []resumableBackgroundExecution
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		for _, callID := range turn.ToolCallOrder {
			call := turn.ToolCalls[callID]
			if call == nil || call.Execution == nil || call.Execution.Background == nil {
				continue
			}
			background := call.Execution.Background
			switch background.Status {
			case events.ExecutionBackgroundStatusExited, events.ExecutionBackgroundStatusSupervisionLost:
				continue
			}
			if background.Exited || background.PID <= 0 {
				continue
			}
			out = append(out, resumableBackgroundExecution{
				SessionID:       state.SessionID,
				TurnID:          turnID,
				ExecutionID:     call.Execution.ExecutionID,
				ToolCallID:      call.CallID,
				ToolName:        call.ToolName,
				PID:             background.PID,
				ProcessIdentity: background.ProcessIdentity,
				LogRef:          background.LogRef,
				ReadyPatterns:   append([]string(nil), background.ReadyPatterns...),
				Ready:           background.Ready,
				OutputTail:      background.OutputTail,
				OutputBytes:     background.OutputBytes,
			})
		}
	}
	return out
}

func (s *ExecutionService) resumeBackgroundExecution(ctx context.Context, candidate resumableBackgroundExecution) error {
	monitorCtx, started := s.startBackgroundRun(candidate.ExecutionID)
	if !started {
		return nil
	}

	input := ExecuteToolInput{
		SessionID:  candidate.SessionID,
		TurnID:     candidate.TurnID,
		ToolCallID: candidate.ToolCallID,
		ToolName:   candidate.ToolName,
	}
	observer := newSeededBackgroundExecutionObserver(s.sessions, input, candidate.OutputTail, candidate.OutputBytes)
	state, err := loadBackgroundProcessStateFunc(candidate.PID)
	if err != nil {
		s.finishBackgroundRun(candidate.ExecutionID)
		return s.appendBackgroundLostEvent(
			ctx,
			input,
			fmt.Sprintf("background process state could not be inspected during resume: %v", err),
			observer,
		)
	}
	if !state.Running {
		s.finishBackgroundRun(candidate.ExecutionID)
		return s.resumeExitedBackgroundExecution(ctx, input, candidate, observer, "background process is no longer running; exit was detected during resumed supervision")
	}
	if strings.TrimSpace(candidate.ProcessIdentity) == "" {
		s.finishBackgroundRun(candidate.ExecutionID)
		return s.appendBackgroundLostEvent(ctx, input, "background process is still running, but its persisted identity is missing and safe resumed supervision is unavailable", observer)
	}
	if strings.TrimSpace(state.Identity) == "" {
		s.finishBackgroundRun(candidate.ExecutionID)
		return s.appendBackgroundLostEvent(ctx, input, "background process is still running, but the runtime could not verify its identity during resume", observer)
	}
	if state.Identity != candidate.ProcessIdentity {
		s.finishBackgroundRun(candidate.ExecutionID)
		return s.appendBackgroundLostEvent(ctx, input, "background process identity no longer matches the persisted session state; pid reuse or process replacement was detected", observer)
	}
	if strings.TrimSpace(candidate.LogRef) == "" {
		s.finishBackgroundRun(candidate.ExecutionID)
		return s.appendBackgroundLostEvent(ctx, input, "background process is still running, but its durable log reference is missing and supervision cannot be resumed", observer)
	}

	go s.monitorResumedBackgroundExecution(monitorCtx, input, candidate, observer)
	return nil
}

func (s *ExecutionService) monitorResumedBackgroundExecution(ctx context.Context, input ExecuteToolInput, candidate resumableBackgroundExecution, observer *backgroundExecutionObserver) {
	defer s.finishBackgroundRun(candidate.ExecutionID)
	if err := s.runResumedBackgroundExecution(ctx, input, candidate, observer); err != nil {
		s.logResumedBackgroundFailure(input, err)
	}
}

func (s *ExecutionService) runResumedBackgroundExecution(ctx context.Context, input ExecuteToolInput, candidate resumableBackgroundExecution, observer *backgroundExecutionObserver) error {
	detector := newExecutionBackgroundReadyDetector(candidate.ReadyPatterns)
	if candidate.Ready || len(candidate.ReadyPatterns) == 0 {
		detector = nil
	}

	if ready, err := s.scanHistoricalBackgroundReady(ctx, candidate.LogRef, detector); err != nil {
		return s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background log could not be read during resume: %v", err), observer)
	} else if ready != nil {
		if err := s.appendBackgroundReadyEvent(ctx, input, *ready, observer); err != nil {
			return errors.Join(
				fmt.Errorf("append background ready during resume: %w", err),
				s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background readiness could not be recorded during resume: %v", err), observer),
			)
		}
		detector = nil
	}

	offset := max(candidate.OutputBytes, int64(0))
	if err := s.syncResumedBackgroundLog(ctx, candidate.LogRef, &offset, observer, detector); err != nil {
		return s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background log catch-up failed during resume: %v", err), observer)
	}
	if ready, ok := backgroundReadyFromDetector(detector); ok {
		if err := s.appendBackgroundReadyEvent(ctx, input, ready, observer); err != nil {
			return errors.Join(
				fmt.Errorf("append background ready during resume: %w", err),
				s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background readiness could not be recorded during resume: %v", err), observer),
			)
		}
		detector = nil
	}

	ticker := time.NewTicker(backgroundResumePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return s.flushResumedBackgroundObserver(ctx, observer)
		case <-ticker.C:
			if err := s.syncResumedBackgroundLog(ctx, candidate.LogRef, &offset, observer, detector); err != nil {
				return s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background log supervision failed after resume: %v", err), observer)
			}
			if ready, ok := backgroundReadyFromDetector(detector); ok {
				if err := s.appendBackgroundReadyEvent(ctx, input, ready, observer); err != nil {
					return errors.Join(
						fmt.Errorf("append background ready during resume: %w", err),
						s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background readiness could not be recorded during resume: %v", err), observer),
					)
				}
				detector = nil
			}

			state, err := loadBackgroundProcessStateFunc(candidate.PID)
			if err != nil {
				return s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background process state could not be refreshed during resumed supervision: %v", err), observer)
			}
			if !state.Running {
				return s.resumeExitedBackgroundExecution(ctx, input, candidate, observer, "background process is no longer running; exit was detected during resumed supervision")
			}
			if strings.TrimSpace(state.Identity) == "" {
				return s.appendBackgroundLostEvent(ctx, input, "background process is still running, but the runtime could not verify its identity during resumed supervision", observer)
			}
			if state.Identity != candidate.ProcessIdentity {
				return s.appendBackgroundLostEvent(ctx, input, "background process identity changed while resumed supervision was active; pid reuse or process replacement was detected", observer)
			}
		}
	}
}

func (s *ExecutionService) resumeExitedBackgroundExecution(ctx context.Context, input ExecuteToolInput, candidate resumableBackgroundExecution, observer *backgroundExecutionObserver, summary string) error {
	if !candidate.Ready && len(candidate.ReadyPatterns) > 0 {
		detector := newExecutionBackgroundReadyDetector(candidate.ReadyPatterns)
		if ready, err := s.scanHistoricalBackgroundReady(ctx, candidate.LogRef, detector); err != nil && !errors.Is(err, os.ErrNotExist) {
			return s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background process exited, but readiness could not be recovered from its durable log during resume: %v", err), observer)
		} else if ready != nil {
			if err := s.appendBackgroundReadyEvent(ctx, input, *ready, observer); err != nil {
				return errors.Join(
					fmt.Errorf("append recovered background ready during resume: %w", err),
					s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background process exited, but its recovered ready state could not be recorded during resume: %v", err), observer),
				)
			}
		}
	}
	offset := max(candidate.OutputBytes, int64(0))
	if err := s.syncResumedBackgroundLog(ctx, candidate.LogRef, &offset, observer, nil); err != nil && !errors.Is(err, os.ErrNotExist) {
		return s.appendBackgroundLostEvent(ctx, input, fmt.Sprintf("background process exited, but its durable log could not be finalized during resume: %v", err), observer)
	}
	return s.appendRecoveredBackgroundExitedEvent(ctx, input, summary, observer)
}

func (s *ExecutionService) scanHistoricalBackgroundReady(ctx context.Context, logRef string, detector *executionBackgroundReadyDetector) (*executionBackgroundReadyEvent, error) {
	if s == nil || s.backgroundLogs == nil || detector == nil || strings.TrimSpace(logRef) == "" {
		return nil, nil
	}
	var offset int64
	for {
		chunk, size, err := s.backgroundLogs.ReadFrom(ctx, logRef, offset, backgroundLogReadLimit)
		if err != nil {
			return nil, err
		}
		if chunk != "" {
			detector.Observe(chunk)
			offset += int64(len(chunk))
			if ready, ok := backgroundReadyFromDetector(detector); ok {
				return &ready, nil
			}
		}
		if chunk == "" || offset >= size {
			return nil, nil
		}
	}
}

func (s *ExecutionService) syncResumedBackgroundLog(ctx context.Context, logRef string, offset *int64, observer *backgroundExecutionObserver, detector *executionBackgroundReadyDetector) error {
	if s == nil || s.backgroundLogs == nil {
		return fmt.Errorf("background log store is unavailable")
	}
	if observer == nil {
		return nil
	}
	if strings.TrimSpace(logRef) == "" {
		return fmt.Errorf("background log reference is missing")
	}
	currentOffset := int64(0)
	if offset != nil {
		currentOffset = max(*offset, int64(0))
	}
	for {
		chunk, size, err := s.backgroundLogs.ReadFrom(ctx, logRef, currentOffset, backgroundLogReadLimit)
		if err != nil {
			return err
		}
		if size < currentOffset {
			return fmt.Errorf("background log shrank from %d to %d bytes", currentOffset, size)
		}
		if chunk == "" {
			if offset != nil {
				*offset = currentOffset
			}
			return nil
		}
		if detector != nil {
			detector.Observe(chunk)
		}
		if err := observer.emitWithContext(ctx, executionOutputChunk{Stream: "combined", Chunk: chunk}); err != nil {
			return err
		}
		currentOffset += int64(len(chunk))
		if currentOffset >= size {
			if offset != nil {
				*offset = currentOffset
			}
			return nil
		}
	}
}

func backgroundReadyFromDetector(detector *executionBackgroundReadyDetector) (executionBackgroundReadyEvent, bool) {
	if detector == nil {
		return executionBackgroundReadyEvent{}, false
	}
	select {
	case ready, ok := <-detector.Ready():
		if !ok {
			return executionBackgroundReadyEvent{}, false
		}
		return ready, true
	default:
		return executionBackgroundReadyEvent{}, false
	}
}

func (s *ExecutionService) appendRecoveredBackgroundExitedEvent(ctx context.Context, input ExecuteToolInput, summary string, observer *backgroundExecutionObserver) error {
	return s.appendResumedBackgroundTerminalEvent(ctx, observer, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeExecutionBackgroundExited,
		Payload: events.ExecutionBackgroundExitedPayload{
			ExecutionID: executionID(input.ToolCallID),
			ToolCallID:  input.ToolCallID,
			ToolName:    input.ToolName,
			Error:       strings.TrimSpace(summary),
		},
	})
}

func (s *ExecutionService) appendBackgroundLostEvent(ctx context.Context, input ExecuteToolInput, message string, observer *backgroundExecutionObserver) error {
	return s.appendResumedBackgroundTerminalEvent(ctx, observer, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeExecutionBackgroundLost,
		Payload: events.ExecutionBackgroundLostPayload{
			ExecutionID: executionID(input.ToolCallID),
			ToolCallID:  input.ToolCallID,
			ToolName:    input.ToolName,
			Error:       strings.TrimSpace(message),
		},
	})
}

func (s *ExecutionService) flushResumedBackgroundObserver(ctx context.Context, observer *backgroundExecutionObserver) error {
	return s.flushBackgroundObserverWithFinalize(ctx, observer)
}

func (s *ExecutionService) appendResumedBackgroundTerminalEvent(ctx context.Context, observer *backgroundExecutionObserver, draft events.Draft) error {
	finalizeCtx, cancel := executionFinalizeContext(ctx)
	defer cancel()

	var errs []error
	if observer != nil {
		if err := observer.Flush(finalizeCtx); err != nil {
			errs = append(errs, fmt.Errorf("flush background observer: %w", err))
		}
	}
	if _, err := s.sessions.append(finalizeCtx, draft); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *ExecutionService) logResumedBackgroundFailure(input ExecuteToolInput, err error) {
	if s == nil || s.logger == nil || err == nil {
		return
	}
	s.logger.Error("resumed background supervision failed", err,
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
		"execution_id", executionID(input.ToolCallID),
	)
}
