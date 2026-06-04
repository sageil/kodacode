package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

func (s *SessionService) Snapshot(ctx context.Context, sessionID string) (events.SessionState, error) {
	if strings.TrimSpace(sessionID) == "" {
		return events.SessionState{}, ErrSessionIDRequired
	}

	runtime := s.runtimeForSession(sessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if err := s.ensureProjectorLocked(ctx, sessionID, runtime); err != nil {
		return events.SessionState{}, err
	}
	if runtime.snapshotSequence < 0 {
		_ = s.appendSessionSnapshotLocked(ctx, runtime, sessionID, runtime.projector.CurrentState().LastSequence)
	}
	return runtime.projector.Snapshot(), nil
}

// Inspect exposes the current session state for short, read-only inspection
// without deep-cloning the entire session. The callback must not retain or
// mutate maps, slices, or nested pointers from the provided state.
func (s *SessionService) Inspect(ctx context.Context, sessionID string, inspect func(events.SessionState) error) error {
	if strings.TrimSpace(sessionID) == "" {
		return ErrSessionIDRequired
	}
	if inspect == nil {
		return nil
	}

	runtime := s.runtimeForSession(sessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if err := s.ensureProjectorLocked(ctx, sessionID, runtime); err != nil {
		return err
	}
	if runtime.snapshotSequence < 0 {
		_ = s.appendSessionSnapshotLocked(ctx, runtime, sessionID, runtime.projector.CurrentState().LastSequence)
	}
	return inspect(runtime.projector.CurrentState())
}

func (s *SessionService) Watch(ctx context.Context, sessionID string, afterSequence int64) (<-chan events.Event, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, ErrSessionIDRequired
	}

	runtime := s.runtimeForSession(sessionID)
	runtime.mu.Lock()
	replayed, err := s.store.Replay(ctx, events.Query{
		SessionID:     sessionID,
		AfterSequence: afterSequence,
		ExcludeTypes:  []events.Type{events.TypeSessionStateSnapshot},
	})
	if err != nil {
		runtime.mu.Unlock()
		return nil, err
	}
	if len(replayed) > 0 {
		runtime.lastDurable = max(runtime.lastDurable, replayed[len(replayed)-1].Sequence)
	}
	watcher := newSessionWatcher(replayed)
	watcherCount := len(runtime.watchers) + 1
	runtime.watchers[watcher] = struct{}{}
	runtime.mu.Unlock()

	if s.logger != nil {
		s.logger.Debug("session watch opened",
			"session_id", sessionID,
			"after_sequence", afterSequence,
			"replayed_count", len(replayed),
			"watcher_count", watcherCount,
		)
	}

	go func() {
		defer func() {
			remaining := 0
			runtime.mu.Lock()
			delete(runtime.watchers, watcher)
			remaining = len(runtime.watchers)
			runtime.mu.Unlock()
			if s.logger != nil {
				s.logger.Debug("session watch closed",
					"session_id", sessionID,
					"after_sequence", afterSequence,
					"watcher_count", remaining,
				)
			}
		}()
		watcher.run(ctx)
	}()

	return watcher.out, nil
}

func (s *SessionService) append(ctx context.Context, draft events.Draft) (events.Event, error) {
	runtime := s.runtimeForSession(draft.SessionID)
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.projector == nil {
		if draft.Type == events.TypeSessionConfigured {
			runtime.projector = events.NewProjector(draft.SessionID)
		} else if err := s.ensureProjectorLocked(ctx, draft.SessionID, runtime); err != nil {
			return events.Event{}, err
		}
	}
	s.ensureBudgetSummaryLocked(runtime)

	event, err := s.store.Append(ctx, draft)
	if err != nil {
		return events.Event{}, err
	}
	runtime.lastDurable = event.Sequence
	projector := runtime.projector
	if projector == nil {
		projector = events.NewProjector(draft.SessionID)
		runtime.projector = projector
	}
	if err := projector.Apply(event); err != nil {
		return events.Event{}, err
	}
	s.updateBudgetSummaryLocked(runtime)
	watcherCount := runtime.pushLocked(event)
	_ = s.appendSessionSnapshotLocked(ctx, runtime, draft.SessionID, event.Sequence)
	logger := s.logger
	logArgs, shouldLog := sessionEventLogArgs(event, projector, watcherCount)
	if shouldLog && logger != nil {
		logger.Debug("session event appended", logArgs...)
	}
	return event, nil
}

func (s *SessionService) publishEphemeral(sessionID, turnID string, typ events.Type, payload events.Payload) error {
	runtime := s.runtimeForSession(sessionID)
	event := events.Event{
		ID:        fmt.Sprintf("%s:live:%d", sessionID, time.Now().UTC().UnixNano()),
		SessionID: sessionID,
		TurnID:    turnID,
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      typ,
		Payload:   payload,
		Ephemeral: true,
	}

	runtime.mu.Lock()
	if err := s.ensureProjectorLocked(context.Background(), sessionID, runtime); err != nil {
		runtime.mu.Unlock()
		return err
	}
	event.Sequence = max(runtime.lastDurable, 0)
	runtime.mu.Unlock()
	if err := event.Validate(); err != nil {
		return err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	event.Sequence = max(runtime.lastDurable, 0)
	projector := runtime.projector
	watcherCount := runtime.pushLocked(event)
	logger := s.logger
	logArgs, shouldLog := sessionEventLogArgs(event, projector, watcherCount)
	if shouldLog && logger != nil {
		logger.Debug("session event published", logArgs...)
	}
	return nil
}

func (s *SessionService) loadProjector(ctx context.Context, sessionID string) (*events.Projector, int64, int64, error) {
	projector := events.NewProjector(sessionID)
	lastDurable := int64(-1)
	snapshotSequence := int64(-1)
	replayAfter := int64(-1)

	latestSnapshot, ok, err := s.store.Latest(ctx, events.LatestQuery{
		SessionID: sessionID,
		Types:     []events.Type{events.TypeSessionStateSnapshot},
	})
	if err != nil {
		return nil, -1, -1, err
	}
	if ok {
		payload, ok := latestSnapshot.Payload.(events.SessionStateSnapshotPayload)
		if !ok {
			return nil, -1, -1, fmt.Errorf("session snapshot payload = %T", latestSnapshot.Payload)
		}
		projector = events.NewProjectorFromSnapshotAt(payload.State, latestSnapshot.Sequence)
		lastDurable = latestSnapshot.Sequence
		snapshotSequence = latestSnapshot.Sequence
		replayAfter = payload.BaseSequence
	}

	replayed, err := s.store.Replay(ctx, events.Query{
		SessionID:     sessionID,
		AfterSequence: replayAfter,
		ExcludeTypes:  []events.Type{events.TypeSessionStateSnapshot},
	})
	if err != nil {
		return nil, -1, -1, err
	}
	for _, event := range replayed {
		if err := projector.Apply(event); err != nil {
			return nil, -1, -1, err
		}
		lastDurable = event.Sequence
	}
	if lastDurable < 0 {
		lastDurable = projector.CurrentState().LastSequence
	}
	return projector, lastDurable, snapshotSequence, nil
}

func sessionEventLogArgs(event events.Event, projector *events.Projector, watcherCount int) ([]any, bool) {
	args := []any{
		"session_id", event.SessionID,
		"turn_id", event.TurnID,
		"event_type", event.Type,
		"sequence", event.Sequence,
		"ephemeral", event.Ephemeral,
		"watcher_count", watcherCount,
	}
	switch payload := event.Payload.(type) {
	case events.ToolCallDeclaredPayload:
		if strings.TrimSpace(payload.ToolName) != "test" {
			return nil, false
		}
		args = append(args,
			"tool_name", payload.ToolName,
			"tool_call_id", payload.CallID,
			"input_len", len(payload.Input),
		)
		return args, true
	case events.ExecutionDeclaredPayload:
		if strings.TrimSpace(payload.ToolName) != "test" {
			return nil, false
		}
		args = append(args,
			"tool_name", payload.ToolName,
			"tool_call_id", payload.ToolCallID,
			"execution_id", payload.ExecutionID,
			"intent", payload.Intent,
			"command_preview", payload.CommandPreview,
			"working_directory", payload.WorkingDirectory,
			"timeout_ms", payload.TimeoutMS,
		)
		return args, true
	case events.ExecutionStartedPayload:
		if strings.TrimSpace(payload.ToolName) != "test" {
			return nil, false
		}
		args = append(args,
			"tool_name", payload.ToolName,
			"tool_call_id", payload.ToolCallID,
			"execution_id", payload.ExecutionID,
			"input_len", len(payload.Input),
		)
		return args, true
	case events.ExecutionOutputPayload:
		if !executionOutputBelongsToTest(projector, payload.ToolCallID) {
			return nil, false
		}
		args = append(args,
			"tool_call_id", payload.ToolCallID,
			"execution_id", payload.ExecutionID,
			"stream", payload.Stream,
			"chunk_len", len(payload.Chunk),
		)
		return args, true
	case events.ToolExecEndPayload:
		if strings.TrimSpace(payload.ToolName) != "test" {
			return nil, false
		}
		args = append(args,
			"tool_name", payload.ToolName,
			"tool_call_id", payload.CallID,
			"execution_id", payload.ExecutionID,
			"execution_status", payload.ExecutionStatus,
			"succeeded", payload.Succeeded,
			"exit_code", payload.ExitCode,
			"duration_ms", payload.DurationMS,
			"command_actions", payload.CommandActions,
			"output_len", len(payload.Output),
			"error_len", len(payload.Error),
			"reused_from_call_id", payload.ReusedFromCallID,
		)
		return args, true
	case events.TurnCanceledPayload:
		args = append(args, "message", payload.Message)
		return args, true
	default:
		return nil, false
	}
}

func executionOutputBelongsToTest(projector *events.Projector, callID string) bool {
	if projector == nil || strings.TrimSpace(callID) == "" {
		return false
	}
	call := projector.ToolCall(callID)
	return call != nil && strings.TrimSpace(call.ToolName) == "test"
}

func (s *SessionService) appendSessionSnapshotLocked(ctx context.Context, runtime *sessionRuntime, sessionID string, latestSequence int64) error {
	if latestSequence < 0 {
		return nil
	}
	if runtime == nil {
		return nil
	}
	lastSnapshotSequence := runtime.snapshotSequence
	if lastSnapshotSequence >= 0 && latestSequence-lastSnapshotSequence < sessionStateSnapshotIntervalEvents {
		return nil
	}
	if lastSnapshotSequence < 0 && latestSequence+1 < sessionStateSnapshotIntervalEvents {
		return nil
	}

	projector := runtime.projector
	if projector == nil {
		return nil
	}
	state := events.SnapshotSessionState(projector.CurrentState())
	snapshotEvent, err := s.store.Append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionStateSnapshot,
		Payload: events.SessionStateSnapshotPayload{
			BaseSequence: latestSequence,
			State:        state,
		},
	})
	if err != nil {
		return err
	}
	runtime.lastDurable = snapshotEvent.Sequence
	runtime.snapshotSequence = snapshotEvent.Sequence
	return projector.Apply(snapshotEvent)
}
