package appservice

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/service"
)

type ctxKey string

type stubSessionService struct {
	session    repository.Session
	sendFn     func(context.Context, string, string, []service.FileAttachment, sandbox.Origin, ...string) error
	spawnFn    func(context.Context, string, string, string, service.ProgressFunc) (string, error)
	answerFn   func(context.Context, string, string, service.AnswerResponse) error
	cancelFn   func(context.Context, string) error
	statusFn   func(context.Context, string) (service.TurnStatus, error)
	statusOpFn func(context.Context, string, string) (service.TurnStatus, error)
	prepareFn  func(context.Context, string, []service.FileAttachment) (*service.SendReservation, error)
	reserveFn  func(string) (*service.SendReservation, error)
	updateFn   func(context.Context, repository.Session) error
}

func (s *stubSessionService) Create(context.Context, string, string, ...service.CreateOption) (repository.Session, error) {
	return s.session, nil
}

func (s *stubSessionService) Get(_ context.Context, id string) (repository.Session, error) {
	if s.session.ID == "" || s.session.ID != id {
		return repository.Session{}, repository.ErrNotFound
	}
	return s.session, nil
}

func (s *stubSessionService) List(context.Context) ([]repository.Session, error) {
	return []repository.Session{s.session}, nil
}

func (s *stubSessionService) UpdateSession(ctx context.Context, sess repository.Session) error {
	if s.updateFn != nil {
		if err := s.updateFn(ctx, sess); err != nil {
			return err
		}
	}
	s.session = sess
	return nil
}
func (s *stubSessionService) Delete(context.Context, string) error { return nil }
func (s *stubSessionService) Branch(context.Context, string, string) (repository.Session, error) {
	return repository.Session{}, nil
}
func (s *stubSessionService) CancelTurn(ctx context.Context, sessionID string) error {
	if s.cancelFn != nil {
		return s.cancelFn(ctx, sessionID)
	}
	return nil
}
func (s *stubSessionService) TurnStatus(ctx context.Context, sessionID string) (service.TurnStatus, error) {
	if s.statusFn != nil {
		return s.statusFn(ctx, sessionID)
	}
	return service.TurnStatus{SessionID: sessionID, State: service.TurnStateIdle}, nil
}
func (s *stubSessionService) TurnStatusByOperation(ctx context.Context, sessionID, operationID string) (service.TurnStatus, error) {
	if s.statusOpFn != nil {
		return s.statusOpFn(ctx, sessionID, operationID)
	}
	return s.TurnStatus(ctx, sessionID)
}
func (s *stubSessionService) Send(ctx context.Context, sessionID, prompt string, attachments []service.FileAttachment, origin sandbox.Origin, variant ...string) error {
	if s.sendFn != nil {
		return s.sendFn(ctx, sessionID, prompt, attachments, origin, variant...)
	}
	return nil
}
func (s *stubSessionService) PrepareSend(ctx context.Context, sessionID string, attachments []service.FileAttachment) (*service.SendReservation, error) {
	if s.prepareFn != nil {
		return s.prepareFn(ctx, sessionID, attachments)
	}
	return nil, nil
}
func (s *stubSessionService) ReserveSend(sessionID string) (*service.SendReservation, error) {
	if s.reserveFn != nil {
		return s.reserveFn(sessionID)
	}
	return nil, nil
}
func (s *stubSessionService) Subscribe(string) (<-chan service.SSEEvent, func()) {
	ch := make(chan service.SSEEvent)
	close(ch)
	return ch, func() {}
}
func (s *stubSessionService) Answer(ctx context.Context, sessionID, questionID string, response service.AnswerResponse) error {
	if s.answerFn != nil {
		return s.answerFn(ctx, sessionID, questionID, response)
	}
	return nil
}
func (s *stubSessionService) ListMessages(context.Context, string) ([]repository.Message, error) {
	return nil, nil
}
func (s *stubSessionService) SpawnSubagent(ctx context.Context, parentSessionID, agentID, task string, onProgress service.ProgressFunc) (string, error) {
	if s.spawnFn != nil {
		return s.spawnFn(ctx, parentSessionID, agentID, task, onProgress)
	}
	return "", nil
}
func (s *stubSessionService) GetSessionTraces(string) [][]service.StepTrace { return nil }

func TestServiceSendMessageUsesBackgroundContext(t *testing.T) {
	key := ctxKey("bg")
	gotCtx := make(chan any, 1)

	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			sendFn: func(ctx context.Context, _ string, _ string, _ []service.FileAttachment, _ sandbox.Origin, _ ...string) error {
				gotCtx <- ctx.Value(key)
				return nil
			},
		},
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.WithValue(context.Background(), key, "attached"),
	})

	if err := svc.SendMessage(context.Background(), "s1", "hello", nil, "", ""); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case got := <-gotCtx:
		if got != "attached" {
			t.Fatalf("background ctx value = %v, want %q", got, "attached")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async send")
	}
}

func TestServiceSendMessagePublishesErrorOnAsyncFailure(t *testing.T) {
	evCh := make(chan service.SSEEvent, 2)

	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			sendFn: func(context.Context, string, string, []service.FileAttachment, sandbox.Origin, ...string) error {
				return errors.New("send failed")
			},
		},
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.Background(),
		Publish: func(sessionID string, ev service.SSEEvent) {
			if sessionID == "s1" {
				evCh <- ev
			}
		},
	})

	if err := svc.SendMessage(context.Background(), "s1", "hello", nil, "", ""); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case ev := <-evCh:
		if ev.Type != "error" {
			t.Fatalf("event type = %q, want error", ev.Type)
		}
		data, ok := ev.Data.(service.SSEErrorData)
		if !ok {
			t.Fatalf("event data type = %T, want SSEErrorData", ev.Data)
		}
		if data.Message != "send failed" {
			t.Fatalf("error message = %q, want %q", data.Message, "send failed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error event")
	}
}

func TestServiceSendMessageRejectsCanceledBackgroundContext(t *testing.T) {
	bg, cancel := context.WithCancel(context.Background())
	cancel()

	svc := New(Config{
		Sessions:      &stubSessionService{session: repository.Session{ID: "s1"}},
		ProjectDir:    t.TempDir(),
		BackgroundCtx: bg,
	})

	if err := svc.SendMessage(context.Background(), "s1", "hello", nil, "", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("SendMessage() error = %v, want context.Canceled", err)
	}
}

func TestServiceSendMessageReturnsBusyErrorBeforeAsyncDispatch(t *testing.T) {
	called := false
	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			reserveFn: func(string) (*service.SendReservation, error) {
				return nil, service.ErrSessionBusy
			},
			sendFn: func(context.Context, string, string, []service.FileAttachment, sandbox.Origin, ...string) error {
				called = true
				return nil
			},
		},
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.Background(),
	})

	err := svc.SendMessage(context.Background(), "s1", "hello", nil, "", "")
	if !errors.Is(err, service.ErrSessionBusy) {
		t.Fatalf("SendMessage() error = %v, want ErrSessionBusy", err)
	}
	if called {
		t.Fatal("Send() should not be dispatched when ReserveSend returns busy")
	}
}

func TestServiceSendMessageReturnsPrepareErrorBeforeAsyncDispatch(t *testing.T) {
	called := false
	wantErr := errors.New("attachments not supported")
	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			prepareFn: func(context.Context, string, []service.FileAttachment) (*service.SendReservation, error) {
				return nil, wantErr
			},
			sendFn: func(context.Context, string, string, []service.FileAttachment, sandbox.Origin, ...string) error {
				called = true
				return nil
			},
		},
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.Background(),
	})

	err := svc.SendMessage(context.Background(), "s1", "hello", nil, "", "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("SendMessage() error = %v, want %v", err, wantErr)
	}
	if called {
		t.Fatal("Send() should not be dispatched when PrepareSend fails")
	}
}

func TestServiceSendMessageOperationDoesNotChangeAgentWhenAttachmentLoadFails(t *testing.T) {
	sessions := &stubSessionService{
		session: repository.Session{ID: "s1", AgentID: "engineer"},
	}
	svc := New(Config{
		Sessions:      sessions,
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.Background(),
	})

	_, err := svc.SendMessageOperation(context.Background(), "s1", "hello", []AttachmentInput{{
		Path:     filepath.Join(t.TempDir(), "missing.png"),
		MimeType: "image/png",
	}}, "reviewer", "")
	if err == nil {
		t.Fatal("SendMessageOperation() error = nil, want attachment load failure")
	}
	if sessions.session.AgentID != "engineer" {
		t.Fatalf("session AgentID = %q, want unchanged %q", sessions.session.AgentID, "engineer")
	}
}

func TestServiceSendMessageOperationDoesNotChangeAgentWhenPrepareFails(t *testing.T) {
	sessions := &stubSessionService{
		session: repository.Session{ID: "s1", AgentID: "engineer"},
		prepareFn: func(context.Context, string, []service.FileAttachment) (*service.SendReservation, error) {
			return nil, errors.New("prepare failed")
		},
	}
	svc := New(Config{
		Sessions:      sessions,
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.Background(),
	})

	_, err := svc.SendMessageOperation(context.Background(), "s1", "hello", nil, "reviewer", "")
	if err == nil {
		t.Fatal("SendMessageOperation() error = nil, want prepare failure")
	}
	if sessions.session.AgentID != "engineer" {
		t.Fatalf("session AgentID = %q, want unchanged %q", sessions.session.AgentID, "engineer")
	}
}

func TestServiceSendMessageOperationUpdatesAgentAfterPrepareSucceeds(t *testing.T) {
	sendCalled := make(chan struct{}, 1)
	sessions := &stubSessionService{
		session: repository.Session{ID: "s1", AgentID: "engineer"},
		prepareFn: func(context.Context, string, []service.FileAttachment) (*service.SendReservation, error) {
			return nil, nil
		},
		sendFn: func(context.Context, string, string, []service.FileAttachment, sandbox.Origin, ...string) error {
			sendCalled <- struct{}{}
			return nil
		},
	}
	svc := New(Config{
		Sessions:      sessions,
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.Background(),
	})

	if _, err := svc.SendMessageOperation(context.Background(), "s1", "hello", nil, "reviewer", ""); err != nil {
		t.Fatalf("SendMessageOperation() error = %v", err)
	}
	if sessions.session.AgentID != "reviewer" {
		t.Fatalf("session AgentID = %q, want %q", sessions.session.AgentID, "reviewer")
	}
	select {
	case <-sendCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async send")
	}
}

func TestServiceCancelTurnDelegatesToSessionService(t *testing.T) {
	called := false
	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			cancelFn: func(_ context.Context, sessionID string) error {
				called = sessionID == "s1"
				return nil
			},
			statusFn: func(_ context.Context, sessionID string) (service.TurnStatus, error) {
				return service.TurnStatus{
					SessionID:       sessionID,
					OperationID:     "op-1",
					State:           service.TurnStateCancelling,
					Active:          true,
					CancelRequested: true,
				}, nil
			},
		},
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.Background(),
	})

	if err := svc.CancelTurn(context.Background(), "s1"); err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if !called {
		t.Fatal("CancelTurn() did not delegate to session service")
	}
}

func TestServiceTurnStatusDelegatesToSessionService(t *testing.T) {
	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			statusFn: func(_ context.Context, sessionID string) (service.TurnStatus, error) {
				return service.TurnStatus{
					SessionID:   sessionID,
					OperationID: "op-123",
					State:       service.TurnStateRunning,
					Active:      true,
				}, nil
			},
		},
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.Background(),
	})

	status, err := svc.TurnStatus(context.Background(), "s1")
	if err != nil {
		t.Fatalf("TurnStatus() error = %v", err)
	}
	if status.OperationID != "op-123" {
		t.Fatalf("OperationID = %q, want %q", status.OperationID, "op-123")
	}
	if status.State != service.TurnStateRunning {
		t.Fatalf("State = %q, want %q", status.State, service.TurnStateRunning)
	}
}

func TestServiceTurnStatusByOperationDelegatesToSessionService(t *testing.T) {
	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			statusOpFn: func(_ context.Context, sessionID, operationID string) (service.TurnStatus, error) {
				return service.TurnStatus{
					SessionID:   sessionID,
					OperationID: operationID,
					State:       service.TurnStateSucceeded,
				}, nil
			},
		},
		ProjectDir:    t.TempDir(),
		BackgroundCtx: context.Background(),
	})

	status, err := svc.TurnStatusByOperation(context.Background(), "s1", "op-9")
	if err != nil {
		t.Fatalf("TurnStatusByOperation() error = %v", err)
	}
	if status.OperationID != "op-9" {
		t.Fatalf("OperationID = %q, want %q", status.OperationID, "op-9")
	}
	if status.State != service.TurnStateSucceeded {
		t.Fatalf("State = %q, want %q", status.State, service.TurnStateSucceeded)
	}
}

func TestServiceSpawnSubagentPublishesAsyncError(t *testing.T) {
	evCh := make(chan service.SSEEvent, 4)

	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			spawnFn: func(context.Context, string, string, string, service.ProgressFunc) (string, error) {
				return "", errors.New("spawn failed")
			},
		},
		BackgroundCtx: context.Background(),
		Publish: func(sessionID string, ev service.SSEEvent) {
			if sessionID == "s1" {
				evCh <- ev
			}
		},
	})

	if err := svc.SpawnSubagent(context.Background(), "s1", "worker", "do work"); err != nil {
		t.Fatalf("SpawnSubagent() error = %v", err)
	}

	// First event should be tool_start (spinner).
	select {
	case ev := <-evCh:
		if ev.Type != "tool_start" {
			t.Fatalf("first event type = %q, want tool_start", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool_start event")
	}

	// Second event should be tool_end with the error.
	select {
	case ev := <-evCh:
		if ev.Type != "tool_end" {
			t.Fatalf("second event type = %q, want tool_end", ev.Type)
		}
		data, ok := ev.Data.(service.SSEToolEndData)
		if !ok {
			t.Fatalf("event data type = %T, want SSEToolEndData", ev.Data)
		}
		if data.Error == nil || *data.Error != "spawn failed" {
			t.Fatalf("error message = %v, want %q", data.Error, "spawn failed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async error event")
	}
}

func TestServiceAnswerQuestionValidatesPermissionResponses(t *testing.T) {
	called := false
	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			answerFn: func(context.Context, string, string, service.AnswerResponse) error {
				called = true
				return nil
			},
		},
	})

	err := svc.AnswerQuestion(context.Background(), "s1", "q-123", "maybe")
	if !errors.Is(err, ErrInvalidQuestionResponse) {
		t.Fatalf("AnswerQuestion() error = %v, want ErrInvalidQuestionResponse", err)
	}
	if called {
		t.Fatal("session Answer() should not be called for invalid permission response")
	}
}

func TestServiceAnswerQuestionAllowsFreeformUserResponses(t *testing.T) {
	var got service.AnswerResponse
	svc := New(Config{
		Sessions: &stubSessionService{
			session: repository.Session{ID: "s1"},
			answerFn: func(_ context.Context, _, _ string, response service.AnswerResponse) error {
				got = response
				return nil
			},
		},
	})

	if err := svc.AnswerQuestion(context.Background(), "s1", "uq-123", "freeform response"); err != nil {
		t.Fatalf("AnswerQuestion() error = %v", err)
	}
	if got != service.AnswerResponse("freeform response") {
		t.Fatalf("Answer() response = %q, want %q", got, "freeform response")
	}
}

func TestServiceRestoreSnapshotReturnsSentinelWhenDisabled(t *testing.T) {
	svc := New(Config{})
	if err := svc.RestoreSnapshot("s1", 1); !errors.Is(err, ErrSnapshotsDisabled) {
		t.Fatalf("RestoreSnapshot() error = %v, want ErrSnapshotsDisabled", err)
	}
}
