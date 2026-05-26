package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/tool"
)

func TestMonitorBackgroundExecutionCancellationFlushesWithCleanupContext(t *testing.T) {
	store := &contextAwareExecutionServiceStore{
		MemoryStore: events.NewMemoryStore(),
		failIfCanceledTypes: map[events.Type]bool{
			events.TypeExecutionBackgroundObserved: true,
		},
	}
	sessions, service := newExecutionServiceTestService(t, store)

	input := ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
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
		service.monitorBackgroundExecution(ctx, input, true, executionBackgroundHandle{
			Exited: make(chan executionBackgroundExitEvent),
		}, observer)
		close(done)
	}()

	cancel()
	waitForChannel(t, done, "background monitor shutdown")

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: input.SessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	observed := 0
	for _, event := range replayed {
		if event.Type == events.TypeExecutionBackgroundObserved {
			observed++
		}
	}
	if observed != 1 {
		t.Fatalf("background observed events = %d, want 1", observed)
	}
}

func TestMonitorBackgroundExecutionLogsFlushFailureOnCancellation(t *testing.T) {
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
	t.Cleanup(func() {
		_ = logger.Close()
	})
	service.SetLogger(logger.With("component", "execution_service"))

	input := ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
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
		service.monitorBackgroundExecution(ctx, input, true, executionBackgroundHandle{
			Exited: make(chan executionBackgroundExitEvent),
		}, observer)
		close(done)
	}()

	cancel()
	waitForChannel(t, done, "background monitor shutdown")

	logBody := readOperationsLog(t, logDir)
	if !strings.Contains(logBody, "background execution event append failed") {
		t.Fatalf("ops log = %q", logBody)
	}
	if !strings.Contains(logBody, "event_type=execution_background_observed") {
		t.Fatalf("ops log = %q", logBody)
	}
	if !strings.Contains(logBody, "flush append failed") {
		t.Fatalf("ops log = %q", logBody)
	}
}

func TestMonitorBackgroundExecutionLogsReadyAppendFailure(t *testing.T) {
	store := &executionServiceFailingStore{
		MemoryStore: events.NewMemoryStore(),
		failTypes: map[events.Type]error{
			events.TypeExecutionBackgroundReady: errors.New("ready append failed"),
		},
	}
	_, service := newExecutionServiceTestService(t, store)
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = logger.Close()
	})
	service.SetLogger(logger.With("component", "execution_service"))

	readyCh := make(chan executionBackgroundReadyEvent, 1)
	readyCh <- executionBackgroundReadyEvent{Message: "server ready", Port: 3000}
	close(readyCh)

	service.monitorBackgroundExecution(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
	}, false, executionBackgroundHandle{Ready: readyCh}, nil)

	logBody := readOperationsLog(t, logDir)
	if !strings.Contains(logBody, "background execution event append failed") {
		t.Fatalf("ops log = %q", logBody)
	}
	if !strings.Contains(logBody, "event_type=execution_background_ready") {
		t.Fatalf("ops log = %q", logBody)
	}
}

func TestMonitorBackgroundExecutionLogsExitAppendFailure(t *testing.T) {
	store := &executionServiceFailingStore{
		MemoryStore: events.NewMemoryStore(),
		failTypes: map[events.Type]error{
			events.TypeExecutionBackgroundExited: errors.New("exit append failed"),
		},
	}
	_, service := newExecutionServiceTestService(t, store)
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = logger.Close()
	})
	service.SetLogger(logger.With("component", "execution_service"))

	exitCh := make(chan executionBackgroundExitEvent, 1)
	exitCh <- executionBackgroundExitEvent{}
	close(exitCh)

	service.monitorBackgroundExecution(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.BashToolName,
	}, true, executionBackgroundHandle{Exited: exitCh}, nil)

	logBody := readOperationsLog(t, logDir)
	if !strings.Contains(logBody, "background execution event append failed") {
		t.Fatalf("ops log = %q", logBody)
	}
	if !strings.Contains(logBody, "event_type=execution_background_exited") {
		t.Fatalf("ops log = %q", logBody)
	}
}

type executionServiceFailingStore struct {
	*events.MemoryStore
	failTypes map[events.Type]error
}

func (s *executionServiceFailingStore) Append(ctx context.Context, draft events.Draft) (events.Event, error) {
	if err := s.failTypes[draft.Type]; err != nil {
		return events.Event{}, err
	}
	return s.MemoryStore.Append(ctx, draft)
}

func readOperationsLog(t *testing.T, logDir string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(logDir, observability.OperationsLogName))
	if err != nil {
		t.Fatalf("ReadFile(ops.log) error = %v", err)
	}
	return string(data)
}
