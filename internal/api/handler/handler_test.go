package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/api/handler"
	"github.com/sageil/kodacode/v1/internal/apitypes"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/service"
)

// ---- fake session service --------------------------------------------------

type fakeSessionSvc struct {
	sessions              map[string]repository.Session
	messages              map[string][]repository.Message
	nextID                int
	sendErr               error
	cancelErr             error
	branchErr             error
	answerErr             error
	answerCalls           int
	cancelCalls           int
	turnStatus            service.TurnStatus
	turnStatusByOperation map[string]service.TurnStatus
}

func newFakeSvc() *fakeSessionSvc {
	return &fakeSessionSvc{
		sessions:              make(map[string]repository.Session),
		messages:              make(map[string][]repository.Message),
		turnStatusByOperation: make(map[string]service.TurnStatus),
	}
}

func (f *fakeSessionSvc) Create(_ context.Context, agentID, modelID string, _ ...service.CreateOption) (repository.Session, error) {
	f.nextID++
	s := repository.Session{
		ID:        "s" + itoa(f.nextID),
		AgentID:   agentID,
		ModelID:   modelID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	f.sessions[s.ID] = s
	return s, nil
}

func (f *fakeSessionSvc) Get(_ context.Context, id string) (repository.Session, error) {
	s, ok := f.sessions[id]
	if !ok {
		return repository.Session{}, repository.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessionSvc) List(_ context.Context) ([]repository.Session, error) {
	out := make([]repository.Session, 0, len(f.sessions))
	for _, s := range f.sessions {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeSessionSvc) UpdateSession(_ context.Context, sess repository.Session) error {
	if _, ok := f.sessions[sess.ID]; !ok {
		return repository.ErrNotFound
	}
	f.sessions[sess.ID] = sess
	return nil
}

func (f *fakeSessionSvc) Delete(_ context.Context, id string) error {
	if _, ok := f.sessions[id]; !ok {
		return repository.ErrNotFound
	}
	delete(f.sessions, id)
	return nil
}

func (f *fakeSessionSvc) Branch(_ context.Context, sessionID, messageID string) (repository.Session, error) {
	if _, ok := f.sessions[sessionID]; !ok {
		return repository.Session{}, repository.ErrNotFound
	}
	if f.branchErr != nil {
		return repository.Session{}, f.branchErr
	}
	f.nextID++
	branch := repository.Session{
		ID:                   "s" + itoa(f.nextID),
		ParentID:             sessionID,
		BranchPointMessageID: messageID,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	f.sessions[branch.ID] = branch
	return branch, nil
}

func (f *fakeSessionSvc) CancelTurn(_ context.Context, sessionID string) error {
	if _, ok := f.sessions[sessionID]; !ok {
		return repository.ErrNotFound
	}
	f.cancelCalls++
	return f.cancelErr
}

func (f *fakeSessionSvc) TurnStatus(_ context.Context, sessionID string) (service.TurnStatus, error) {
	if _, ok := f.sessions[sessionID]; !ok {
		return service.TurnStatus{}, repository.ErrNotFound
	}
	if f.turnStatus.SessionID == "" {
		return service.TurnStatus{SessionID: sessionID, State: service.TurnStateIdle}, nil
	}
	return f.turnStatus, nil
}

func (f *fakeSessionSvc) TurnStatusByOperation(_ context.Context, sessionID, operationID string) (service.TurnStatus, error) {
	if _, ok := f.sessions[sessionID]; !ok {
		return service.TurnStatus{}, repository.ErrNotFound
	}
	status, ok := f.turnStatusByOperation[operationID]
	if !ok {
		return service.TurnStatus{}, service.ErrTurnOperationNotFound
	}
	return status, nil
}

func (f *fakeSessionSvc) Send(_ context.Context, _ string, _ string, _ []service.FileAttachment, _ sandbox.Origin, _ ...string) error {
	return f.sendErr
}

func (f *fakeSessionSvc) Subscribe(sessionID string) (<-chan service.SSEEvent, func()) {
	ch := make(chan service.SSEEvent, 4)
	close(ch) // empty stream for tests
	return ch, func() {}
}

func (f *fakeSessionSvc) Answer(_ context.Context, _ string, _ string, _ service.AnswerResponse) error {
	f.answerCalls++
	return f.answerErr
}

func (f *fakeSessionSvc) ListMessages(_ context.Context, sessionID string) ([]repository.Message, error) {
	return f.messages[sessionID], nil
}

func (f *fakeSessionSvc) SpawnSubagent(_ context.Context, _, _, _ string, _ service.ProgressFunc) (string, error) {
	return "subagent result", nil
}

func (f *fakeSessionSvc) GetSessionTraces(_ string) [][]service.StepTrace {
	return nil
}

func itoa(n int) string {
	return string(rune('0' + n))
}

// ---- fake agent service ----------------------------------------------------

type fakeAgentSvc struct{}

func (fakeAgentSvc) List() []agent.Agent                                 { return nil }
func (fakeAgentSvc) Get(_ string) (agent.Agent, error)                   { return agent.Agent{}, agent.ErrNotFound }
func (fakeAgentSvc) Create(a agent.Agent) (agent.Agent, error)           { return a, nil }
func (fakeAgentSvc) Update(_ string, a agent.Agent) (agent.Agent, error) { return a, nil }
func (fakeAgentSvc) Delete(_ string) error                               { return nil }

// ---- helpers ---------------------------------------------------------------

func newEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	return e
}

func doRequest(t *testing.T, e *echo.Echo, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// ---- POST /sessions --------------------------------------------------------

func TestHandler_CreateSession(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	rec := doRequest(t, e, http.MethodPost, "/sessions", map[string]string{
		"agent_id": "default",
		"model_id": "openai/gpt-4o",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /sessions = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body)
	}

	var got repository.Session
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID == "" {
		t.Error("response has empty ID")
	}
	if got.AgentID != "default" {
		t.Errorf("AgentID = %q, want %q", got.AgentID, "default")
	}
	if got.ModelID != "openai/gpt-4o" {
		t.Errorf("ModelID = %q, want %q", got.ModelID, "openai/gpt-4o")
	}
}

// ---- GET /sessions ---------------------------------------------------------

func TestHandler_ListSessions(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	_, _ = svc.Create(context.Background(), "default", "openai/gpt-4o")
	_, _ = svc.Create(context.Background(), "builder", "openai/gpt-4o")

	rec := doRequest(t, e, http.MethodGet, "/sessions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /sessions = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []repository.Session
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(sessions) = %d, want 2", len(got))
	}
}

// ---- GET /sessions/:id -----------------------------------------------------

func TestHandler_GetSession(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")

	rec := doRequest(t, e, http.MethodGet, "/sessions/"+created.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /sessions/%s = %d, want %d; body=%s", created.ID, rec.Code, http.StatusOK, rec.Body)
	}

	var got repository.Session
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	ignoreTime := cmpopts.IgnoreFields(repository.Session{}, "CreatedAt", "UpdatedAt")
	if diff := cmp.Diff(created, got, ignoreTime); diff != "" {
		t.Errorf("GET /sessions/%s mismatch (-want +got):\n%s", created.ID, diff)
	}
}

func TestHandler_GetSession_notFound(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	rec := doRequest(t, e, http.MethodGet, "/sessions/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /sessions/nope = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ---- DELETE /sessions/:id --------------------------------------------------

func TestHandler_DeleteSession(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")

	rec := doRequest(t, e, http.MethodDelete, "/sessions/"+created.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /sessions/%s = %d, want %d; body=%s", created.ID, rec.Code, http.StatusNoContent, rec.Body)
	}

	// Verify deleted.
	rec2 := doRequest(t, e, http.MethodGet, "/sessions/"+created.ID, nil)
	if rec2.Code != http.StatusNotFound {
		t.Errorf("GET after DELETE = %d, want %d", rec2.Code, http.StatusNotFound)
	}
}

func TestHandler_DeleteSession_notFound(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	rec := doRequest(t, e, http.MethodDelete, "/sessions/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE /sessions/nope = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ---- POST /sessions/:id/messages -------------------------------------------

func TestHandler_SendMessage(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")
	rec := doRequest(t, e, http.MethodPost, "/sessions/"+created.ID+"/messages", map[string]string{
		"content": "hello",
	})
	if rec.Code != http.StatusAccepted {
		t.Errorf("POST /sessions/%s/messages = %d, want %d; body=%s", created.ID, rec.Code, http.StatusAccepted, rec.Body)
	}
	var got apitypes.TurnStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode send status: %v", err)
	}
	if got.State != string(service.TurnStateRunning) {
		t.Fatalf("state = %q, want %q", got.State, service.TurnStateRunning)
	}
	if !got.Active {
		t.Fatal("active = false, want true")
	}
}

func TestHandler_SendMessage_notFound(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	rec := doRequest(t, e, http.MethodPost, "/sessions/nope/messages", map[string]string{
		"content": "hello",
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST /sessions/nope/messages = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandler_SendMessage_sendError(t *testing.T) {
	// Since Send runs in a background goroutine, the handler always returns 202.
	// Errors are published as SSE "error" events, not as HTTP error responses.
	e := newEcho()
	svc := newFakeSvc()
	svc.sendErr = errors.New("provider offline")
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")

	rec := doRequest(t, e, http.MethodPost, "/sessions/"+created.ID+"/messages", map[string]string{
		"content": "hello",
	})
	if rec.Code != http.StatusAccepted {
		t.Errorf("POST .../messages with send error = %d, want %d (errors surface via SSE)", rec.Code, http.StatusAccepted)
	}
}

func TestHandler_CancelTurn(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")
	svc.turnStatus = service.TurnStatus{
		SessionID:       created.ID,
		OperationID:     "op-1",
		State:           service.TurnStateCancelling,
		Active:          true,
		CancelRequested: true,
	}

	rec := doRequest(t, e, http.MethodPost, "/sessions/"+created.ID+"/cancel", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /sessions/%s/cancel = %d, want %d", created.ID, rec.Code, http.StatusAccepted)
	}
	if svc.cancelCalls != 1 {
		t.Fatalf("cancelCalls = %d, want 1", svc.cancelCalls)
	}
	var got apitypes.TurnStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode cancel status: %v", err)
	}
	if !got.CancelRequested {
		t.Fatal("cancel_requested = false, want true")
	}
	if got.OperationID != "op-1" {
		t.Fatalf("operation_id = %q, want %q", got.OperationID, "op-1")
	}
}

func TestHandler_CancelTurn_noActiveTurn(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	svc.cancelErr = service.ErrNoActiveTurn
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")

	rec := doRequest(t, e, http.MethodPost, "/sessions/"+created.ID+"/cancel", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("POST /sessions/%s/cancel = %d, want %d", created.ID, rec.Code, http.StatusConflict)
	}
}

func TestHandler_GetTurnStatus(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")
	svc.turnStatus = service.TurnStatus{
		SessionID:   created.ID,
		OperationID: "op-9",
		State:       service.TurnStateSucceeded,
		StartedAt:   time.Now().Add(-time.Minute).UTC(),
		UpdatedAt:   time.Now().UTC(),
		FinishedAt:  time.Now().UTC(),
	}

	rec := doRequest(t, e, http.MethodGet, "/sessions/"+created.ID+"/turn", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /sessions/%s/turn = %d, want %d", created.ID, rec.Code, http.StatusOK)
	}
	var got apitypes.TurnStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode turn status: %v", err)
	}
	if got.OperationID != "op-9" {
		t.Fatalf("operation_id = %q, want %q", got.OperationID, "op-9")
	}
	if got.State != string(service.TurnStateSucceeded) {
		t.Fatalf("state = %q, want %q", got.State, service.TurnStateSucceeded)
	}
}

func TestHandler_GetTurnStatus_ByOperation(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")
	svc.turnStatusByOperation["op-42"] = service.TurnStatus{
		SessionID:   created.ID,
		OperationID: "op-42",
		State:       service.TurnStateFailed,
		Error:       "provider offline",
	}

	rec := doRequest(t, e, http.MethodGet, "/sessions/"+created.ID+"/turn?operation_id=op-42", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /sessions/%s/turn?operation_id=op-42 = %d, want %d", created.ID, rec.Code, http.StatusOK)
	}
	var got apitypes.TurnStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode turn status: %v", err)
	}
	if got.OperationID != "op-42" {
		t.Fatalf("operation_id = %q, want %q", got.OperationID, "op-42")
	}
	if got.State != string(service.TurnStateFailed) {
		t.Fatalf("state = %q, want %q", got.State, service.TurnStateFailed)
	}
}

// ---- GET /sessions/:id/messages --------------------------------------------

func TestHandler_ListMessages(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")
	svc.messages[created.ID] = []repository.Message{
		{ID: "m1", SessionID: created.ID, Role: "user", Parts: []repository.MessagePart{
			{Type: "text", Content: `{"text":"hello"}`},
		}},
		{ID: "m2", SessionID: created.ID, Role: "assistant", Parts: []repository.MessagePart{
			{Type: "text", Content: `{"text":"world"}`},
		}},
	}

	rec := doRequest(t, e, http.MethodGet, "/sessions/"+created.ID+"/messages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /sessions/%s/messages = %d, want %d; body=%s", created.ID, rec.Code, http.StatusOK, rec.Body)
	}

	var got []json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(messages) = %d, want 2", len(got))
	}
}

func TestHandler_ListMessages_AllPartTypes(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")
	reasoningJSON := `{"text":"let me think","tokens":5,"signature":"sig123"}`
	fileJSON := `{"path":"main.go","mime_type":"text/plain","url":"package main"}`
	toolCallJSON := `{"id":"tc1","name":"bash","arguments":"{\"cmd\":\"ls\"}"}`
	toolResultJSON := `{"tool_call_id":"tc1","output":"file.txt"}`

	svc.messages[created.ID] = []repository.Message{
		{ID: "m1", SessionID: created.ID, Role: "user", Parts: []repository.MessagePart{
			{Type: "text", Content: `{"text":"please read this file"}`},
			{Type: "file", Content: fileJSON},
		}},
		{ID: "m2", SessionID: created.ID, Role: "assistant", Parts: []repository.MessagePart{
			{Type: "reasoning", Content: reasoningJSON},
			{Type: "text", Content: `{"text":"here is the result"}`},
			{Type: "tool_call", Content: toolCallJSON},
		}},
		{ID: "m3", SessionID: created.ID, Role: "tool", Parts: []repository.MessagePart{
			{Type: "tool_result", Content: toolResultJSON},
		}},
		{ID: "m4", SessionID: created.ID, Role: "assistant", Parts: []repository.MessagePart{
			{Type: "reasoning", Content: reasoningJSON},
		}},
	}

	rec := doRequest(t, e, http.MethodGet, "/sessions/"+created.ID+"/messages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body)
	}

	var got []apitypes.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("len(messages) = %d, want 4", len(got))
	}

	// m1: text + file part
	if got[0].Content != "please read this file" {
		t.Errorf("m1 Content = %q, want %q", got[0].Content, "please read this file")
	}
	if len(got[0].Parts) != 1 || got[0].Parts[0].Type != "file" {
		t.Errorf("m1 Parts = %+v, want one file part", got[0].Parts)
	}

	// m2: reasoning + text + tool_call
	if got[1].Content != "here is the result" {
		t.Errorf("m2 Content = %q, want %q", got[1].Content, "here is the result")
	}
	wantTypes := []string{"reasoning", "tool_call"}
	if len(got[1].Parts) != len(wantTypes) {
		t.Fatalf("m2 Parts count = %d, want %d", len(got[1].Parts), len(wantTypes))
	}
	for i, wt := range wantTypes {
		if got[1].Parts[i].Type != wt {
			t.Errorf("m2 Parts[%d].Type = %q, want %q", i, got[1].Parts[i].Type, wt)
		}
	}

	// m3: tool_result only
	if len(got[2].Parts) != 1 || got[2].Parts[0].Type != "tool_result" {
		t.Errorf("m3 Parts = %+v, want one tool_result part", got[2].Parts)
	}

	// m4: reasoning only — must not be dropped
	if len(got[3].Parts) != 1 || got[3].Parts[0].Type != "reasoning" {
		t.Errorf("m4 Parts = %+v, want one reasoning part (must not be dropped)", got[3].Parts)
	}
	if got[3].Parts[0].Content != reasoningJSON {
		t.Errorf("m4 reasoning content = %q, want %q", got[3].Parts[0].Content, reasoningJSON)
	}
}

func TestHandler_ListMessages_SkipsSyntheticParts(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")
	svc.messages[created.ID] = []repository.Message{
		{ID: "m1", SessionID: created.ID, Role: "assistant", Parts: []repository.MessagePart{
			{Type: "text", Content: `{"text":"visible plan"}`},
			{Type: "text", Content: `{"text":"hidden marker"}`, Synthetic: true},
		}},
		{ID: "m2", SessionID: created.ID, Role: "user", Parts: []repository.MessagePart{
			{Type: "text", Content: `{"text":"hidden only"}`, Synthetic: true},
		}},
	}

	rec := doRequest(t, e, http.MethodGet, "/sessions/"+created.ID+"/messages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body)
	}

	var got []apitypes.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(got))
	}
	if got[0].Content != "visible plan" {
		t.Fatalf("message content = %q, want %q", got[0].Content, "visible plan")
	}
}

func TestHandler_ListMessages_notFound(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	rec := doRequest(t, e, http.MethodGet, "/sessions/nope/messages", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /sessions/nope/messages = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ---- POST /sessions/:id/branch ---------------------------------------------

func TestHandler_BranchSession(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	parent, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")

	rec := doRequest(t, e, http.MethodPost, "/sessions/"+parent.ID+"/branch", map[string]string{
		"message_id": "m1",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /sessions/%s/branch = %d, want %d; body=%s", parent.ID, rec.Code, http.StatusCreated, rec.Body)
	}

	var got repository.Session
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ParentID != parent.ID {
		t.Errorf("branch ParentID = %q, want %q", got.ParentID, parent.ID)
	}
}

func TestHandler_BranchSession_messageNotFound(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	svc.branchErr = repository.ErrNotFound
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	parent, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")

	rec := doRequest(t, e, http.MethodPost, "/sessions/"+parent.ID+"/branch", map[string]string{
		"message_id": "missing",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /sessions/%s/branch missing message = %d, want %d; body=%s", parent.ID, rec.Code, http.StatusNotFound, rec.Body)
	}
}

func TestHandler_AnswerQuestion_InvalidPermissionResponse(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	created, _ := svc.Create(context.Background(), "default", "openai/gpt-4o")
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	rec := doRequest(t, e, http.MethodPost, "/sessions/"+created.ID+"/answer", map[string]string{
		"question_id": "q-123",
		"response":    "maybe",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /sessions/%s/answer = %d, want %d; body=%s", created.ID, rec.Code, http.StatusBadRequest, rec.Body)
	}
	if svc.answerCalls != 0 {
		t.Fatalf("Answer() calls = %d, want 0 for invalid request", svc.answerCalls)
	}
}

func TestHandler_RestoreSnapshot_NotEnabled(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	rec := doRequest(t, e, http.MethodPost, "/sessions/s1/snapshots/1/restore", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /sessions/s1/snapshots/1/restore = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body)
	}
}

// ---- GET /health -----------------------------------------------------------

func TestHandler_Health(t *testing.T) {
	e := newEcho()
	svc := newFakeSvc()
	handler.RegisterRoutes(e, svc, fakeAgentSvc{}, nil, nil, nil)

	rec := doRequest(t, e, http.MethodGet, "/health", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /health = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ---- fake settings service -------------------------------------------------

type fakeSettingsSvc struct {
	data map[string]string
}

func newFakeSettingsSvc() *fakeSettingsSvc {
	return &fakeSettingsSvc{data: make(map[string]string)}
}

func (f *fakeSettingsSvc) GetSetting(_ context.Context, key string) (string, error) {
	v, ok := f.data[key]
	if !ok {
		return "", repository.ErrNotFound
	}
	return v, nil
}

func (f *fakeSettingsSvc) SetSetting(_ context.Context, key, value string) error {
	f.data[key] = value
	return nil
}

// ---- GET /settings/:key ----------------------------------------------------

func TestHandler_GetSetting(t *testing.T) {
	e := newEcho()
	settings := newFakeSettingsSvc()
	settings.data["tui.theme"] = "rose-pine"
	handler.RegisterRoutes(e, newFakeSvc(), fakeAgentSvc{}, settings, nil, nil)

	rec := doRequest(t, e, http.MethodGet, "/settings/tui.theme", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/tui.theme = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body)
	}

	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["value"] != "rose-pine" {
		t.Errorf("value = %q, want %q", got["value"], "rose-pine")
	}
}

func TestHandler_GetSetting_notFound(t *testing.T) {
	e := newEcho()
	handler.RegisterRoutes(e, newFakeSvc(), fakeAgentSvc{}, newFakeSettingsSvc(), nil, nil)

	rec := doRequest(t, e, http.MethodGet, "/settings/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /settings/missing = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ---- PUT /settings/:key ----------------------------------------------------

func TestHandler_PutSetting(t *testing.T) {
	e := newEcho()
	settings := newFakeSettingsSvc()
	handler.RegisterRoutes(e, newFakeSvc(), fakeAgentSvc{}, settings, nil, nil)

	rec := doRequest(t, e, http.MethodPut, "/settings/tui.theme", map[string]string{
		"value": "catppuccin",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /settings/tui.theme = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body)
	}

	if settings.data["tui.theme"] != "catppuccin" {
		t.Errorf("stored value = %q, want %q", settings.data["tui.theme"], "catppuccin")
	}
}
