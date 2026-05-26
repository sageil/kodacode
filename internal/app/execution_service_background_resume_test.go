package app

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/observability"
)

func TestMonitorResumedBackgroundExecutionCancellationFlushesWithCleanupContext(t *testing.T) {
	store := &contextAwareExecutionServiceStore{
		MemoryStore: events.NewMemoryStore(),
		failIfCanceledTypes: map[events.Type]bool{
			events.TypeExecutionBackgroundObserved: true,
		},
	}
	sessions, service := newExecutionServiceTestService(t, store)

	readStarted := make(chan struct{}, 1)
	service.SetBackgroundLogStore(&scriptedBackgroundExecutionLogStore{
		readFrom: func(ctx context.Context, ref string, offset int64, limit int) (string, int64, error) {
			select {
			case readStarted <- struct{}{}:
			default:
			}
			return "", 0, nil
		},
	})

	input := ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
	}
	observer := newBackgroundExecutionObserver(sessions, input)
	observer.mu.Lock()
	observer.outputTail = "tail"
	observer.outputBytes = 4
	observer.dirty = true
	observer.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		service.monitorResumedBackgroundExecution(ctx, input, resumableBackgroundExecution{
			SessionID:   input.SessionID,
			TurnID:      input.TurnID,
			ExecutionID: executionID(input.ToolCallID),
			ToolCallID:  input.ToolCallID,
			ToolName:    input.ToolName,
			LogRef:      filepath.ToSlash(filepath.Join(input.SessionID, input.TurnID, executionID(input.ToolCallID)+".log")),
			Ready:       true,
		}, observer)
		close(done)
	}()

	waitForChannel(t, readStarted, "initial resumed log sync")
	cancel()
	waitForChannel(t, done, "resumed background monitor shutdown")

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: input.SessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	observed := 0
	lost := 0
	for _, event := range replayed {
		switch event.Type {
		case events.TypeExecutionBackgroundObserved:
			observed++
		case events.TypeExecutionBackgroundLost:
			lost++
		}
	}
	if observed != 1 {
		t.Fatalf("background observed events = %d, want 1", observed)
	}
	if lost != 0 {
		t.Fatalf("background lost events = %d, want 0", lost)
	}
}

func TestMonitorResumedBackgroundExecutionLogsFlushFailureOnCancellation(t *testing.T) {
	store := &executionServiceFailingStore{
		MemoryStore: events.NewMemoryStore(),
		failTypes: map[events.Type]error{
			events.TypeExecutionBackgroundObserved: errors.New("flush append failed"),
		},
	}
	_, service := newExecutionServiceTestService(t, store)

	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	service.SetLogger(logger.With("component", "execution_service"))

	readStarted := make(chan struct{}, 1)
	service.SetBackgroundLogStore(&scriptedBackgroundExecutionLogStore{
		readFrom: func(ctx context.Context, ref string, offset int64, limit int) (string, int64, error) {
			select {
			case readStarted <- struct{}{}:
			default:
			}
			return "", 0, nil
		},
	})

	input := ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
	}
	observer := newBackgroundExecutionObserver(service.sessions, input)
	observer.mu.Lock()
	observer.outputTail = "tail"
	observer.outputBytes = 4
	observer.dirty = true
	observer.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		service.monitorResumedBackgroundExecution(ctx, input, resumableBackgroundExecution{
			SessionID:   input.SessionID,
			TurnID:      input.TurnID,
			ExecutionID: executionID(input.ToolCallID),
			ToolCallID:  input.ToolCallID,
			ToolName:    input.ToolName,
			LogRef:      filepath.ToSlash(filepath.Join(input.SessionID, input.TurnID, executionID(input.ToolCallID)+".log")),
			Ready:       true,
		}, observer)
		close(done)
	}()

	waitForChannel(t, readStarted, "initial resumed log sync")
	cancel()
	waitForChannel(t, done, "resumed background monitor shutdown")

	logBody := readOperationsLog(t, logDir)
	if !strings.Contains(logBody, "resumed background supervision failed") {
		t.Fatalf("ops log = %q", logBody)
	}
	if !strings.Contains(logBody, "flush append failed") {
		t.Fatalf("ops log = %q", logBody)
	}
}

func TestMonitorResumedBackgroundExecutionLogsReadyAndLostAppendFailures(t *testing.T) {
	store := &executionServiceFailingStore{
		MemoryStore: events.NewMemoryStore(),
		failTypes: map[events.Type]error{
			events.TypeExecutionBackgroundReady: errors.New("ready append failed"),
			events.TypeExecutionBackgroundLost:  errors.New("lost append failed"),
		},
	}
	_, service := newExecutionServiceTestService(t, store)

	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	service.SetLogger(logger.With("component", "execution_service"))

	service.SetBackgroundLogStore(&scriptedBackgroundExecutionLogStore{
		readFrom: func(ctx context.Context, ref string, offset int64, limit int) (string, int64, error) {
			return "server ready\n", int64(len("server ready\n")), nil
		},
	})

	done := make(chan struct{})
	go func() {
		service.monitorResumedBackgroundExecution(context.Background(), ExecuteToolInput{
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "call-1",
			ToolName:   "bash",
		}, resumableBackgroundExecution{
			SessionID:     "session-1",
			TurnID:        "turn-1",
			ExecutionID:   "exec-call-1",
			ToolCallID:    "call-1",
			ToolName:      "bash",
			LogRef:        filepath.ToSlash(filepath.Join("session-1", "turn-1", "exec-call-1.log")),
			ReadyPatterns: []string{"ready"},
		}, newBackgroundExecutionObserver(service.sessions, ExecuteToolInput{
			SessionID:  "session-1",
			TurnID:     "turn-1",
			ToolCallID: "call-1",
			ToolName:   "bash",
		}))
		close(done)
	}()
	waitForChannel(t, done, "resumed background monitor failure path")

	logBody := readOperationsLog(t, logDir)
	if !strings.Contains(logBody, "resumed background supervision failed") {
		t.Fatalf("ops log = %q", logBody)
	}
	if !strings.Contains(logBody, "ready append failed") || !strings.Contains(logBody, "lost append failed") {
		t.Fatalf("ops log = %q", logBody)
	}
}

func TestMonitorResumedBackgroundExecutionMarksLostOnIdentityMismatch(t *testing.T) {
	store := events.NewMemoryStore()
	_, service := newExecutionServiceTestService(t, store)

	service.SetBackgroundLogStore(&scriptedBackgroundExecutionLogStore{
		readFrom: func(ctx context.Context, ref string, offset int64, limit int) (string, int64, error) {
			return "", 0, nil
		},
	})

	prevInspect := loadBackgroundProcessStateFunc
	loadBackgroundProcessStateFunc = func(pid int) (backgroundProcessState, error) {
		return backgroundProcessState{Running: true, Identity: "identity-new"}, nil
	}
	t.Cleanup(func() {
		loadBackgroundProcessStateFunc = prevInspect
	})

	input := ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   "bash",
	}

	done := make(chan struct{})
	go func() {
		service.monitorResumedBackgroundExecution(context.Background(), input, resumableBackgroundExecution{
			SessionID:       input.SessionID,
			TurnID:          input.TurnID,
			ExecutionID:     executionID(input.ToolCallID),
			ToolCallID:      input.ToolCallID,
			ToolName:        input.ToolName,
			PID:             4242,
			ProcessIdentity: "identity-old",
			LogRef:          filepath.ToSlash(filepath.Join(input.SessionID, input.TurnID, executionID(input.ToolCallID)+".log")),
			Ready:           true,
		}, newBackgroundExecutionObserver(service.sessions, input))
		close(done)
	}()

	waitForChannel(t, done, "resumed background identity mismatch")

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: input.SessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	lostEvents := 0
	for _, event := range replayed {
		if event.Type != events.TypeExecutionBackgroundLost {
			continue
		}
		lostEvents++
		payload, ok := event.Payload.(events.ExecutionBackgroundLostPayload)
		if !ok {
			t.Fatalf("background lost payload type = %T, want events.ExecutionBackgroundLostPayload", event.Payload)
		}
		if !strings.Contains(payload.Error, "identity changed while resumed supervision was active") {
			t.Fatalf("background lost error = %q", payload.Error)
		}
	}
	if lostEvents != 1 {
		t.Fatalf("background lost events = %d, want 1", lostEvents)
	}
}

type scriptedBackgroundExecutionLogStore struct {
	readFrom func(context.Context, string, int64, int) (string, int64, error)
}

func (s *scriptedBackgroundExecutionLogStore) Create(context.Context, BackgroundExecutionLogKey) (BackgroundExecutionLogHandle, error) {
	return BackgroundExecutionLogHandle{Writer: noopWriteCloser{}}, nil
}

func (s *scriptedBackgroundExecutionLogStore) ReadTail(context.Context, string, int) (string, int64, error) {
	return "", 0, nil
}

func (s *scriptedBackgroundExecutionLogStore) ReadPrefix(context.Context, string, int) (string, int64, error) {
	return "", 0, nil
}

func (s *scriptedBackgroundExecutionLogStore) ReadFrom(ctx context.Context, ref string, offset int64, limit int) (string, int64, error) {
	if s != nil && s.readFrom != nil {
		return s.readFrom(ctx, ref, offset, limit)
	}
	return "", 0, nil
}

type noopWriteCloser struct{}

func (noopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (noopWriteCloser) Close() error                { return nil }

type contextAwareExecutionServiceStore struct {
	*events.MemoryStore
	failIfCanceledTypes map[events.Type]bool
}

func (s *contextAwareExecutionServiceStore) Append(ctx context.Context, draft events.Draft) (events.Event, error) {
	if s.failIfCanceledTypes[draft.Type] && ctx != nil && ctx.Err() != nil {
		return events.Event{}, ctx.Err()
	}
	return s.MemoryStore.Append(ctx, draft)
}

func newExecutionServiceTestService(t *testing.T, store events.ReplayStore) (*SessionService, *ExecutionService) {
	t.Helper()

	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	return sessions, NewExecutionService(sessions)
}

func waitForChannel(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

var _ BackgroundExecutionLogStore = (*scriptedBackgroundExecutionLogStore)(nil)
var _ io.WriteCloser = noopWriteCloser{}
