package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type testBackend struct {
	cancelTurnFn     func(context.Context, string) error
	getSessionFn     func(context.Context, string) (APISession, error)
	hasActiveTurnsFn func(context.Context) (bool, error)
	listModelsFn     func(context.Context) ([]APIProviderModels, error)
	syncProvidersFn  func(context.Context) ([]string, error)
	listSessionsFn   func(context.Context) ([]APISession, error)
	listAgentsFn     func(context.Context) ([]APIAgent, error)
	getSettingFn     func(context.Context, string) (string, error)
	setSettingFn     func(context.Context, string, string) error
}

func (b testBackend) CreateSession(context.Context, string, string) (APISession, error) {
	return APISession{}, nil
}
func (b testBackend) ListModels(ctx context.Context) ([]APIProviderModels, error) {
	if b.listModelsFn != nil {
		return b.listModelsFn(ctx)
	}
	return nil, nil
}
func (b testBackend) RefreshModels(context.Context) ([]APIProviderModels, error) { return nil, nil }
func (b testBackend) SyncProviders(ctx context.Context) ([]string, error) {
	if b.syncProvidersFn != nil {
		return b.syncProvidersFn(ctx)
	}
	return nil, nil
}
func (b testBackend) HasActiveTurns(ctx context.Context) (bool, error) {
	if b.hasActiveTurnsFn != nil {
		return b.hasActiveTurnsFn(ctx)
	}
	return false, nil
}
func (b testBackend) RefreshMCPTools(context.Context) (int, error)             { return 0, nil }
func (b testBackend) UpdateSessionModel(context.Context, string, string) error { return nil }
func (b testBackend) UpdateSessionAgent(context.Context, string, string) error { return nil }
func (b testBackend) ListSessions(ctx context.Context) ([]APISession, error) {
	if b.listSessionsFn != nil {
		return b.listSessionsFn(ctx)
	}
	return nil, nil
}
func (b testBackend) GetSession(ctx context.Context, id string) (APISession, error) {
	if b.getSessionFn != nil {
		return b.getSessionFn(ctx, id)
	}
	return APISession{}, nil
}
func (b testBackend) ListMessages(context.Context, string) ([]APIMessage, error) { return nil, nil }
func (b testBackend) DeleteSession(context.Context, string) error                { return nil }
func (b testBackend) SendMessage(context.Context, string, string, []Attachment, string, ...string) error {
	return nil
}
func (b testBackend) CancelTurn(ctx context.Context, sessionID string) error {
	if b.cancelTurnFn != nil {
		return b.cancelTurnFn(ctx, sessionID)
	}
	return nil
}
func (b testBackend) SpawnSubagent(context.Context, string, string, string) error { return nil }
func (b testBackend) ListAgents(ctx context.Context) ([]APIAgent, error) {
	if b.listAgentsFn != nil {
		return b.listAgentsFn(ctx)
	}
	return nil, nil
}
func (b testBackend) AnswerQuestion(context.Context, string, string, string) error { return nil }
func (b testBackend) GetConfig(context.Context) (APIConfig, error)                 { return APIConfig{}, nil }
func (b testBackend) GetSetting(ctx context.Context, key string) (string, error) {
	if b.getSettingFn != nil {
		return b.getSettingFn(ctx, key)
	}
	return "", nil
}
func (b testBackend) SetSetting(ctx context.Context, key, value string) error {
	if b.setSettingFn != nil {
		return b.setSettingFn(ctx, key, value)
	}
	return nil
}
func (b testBackend) ListSnapshots(context.Context, string) ([]APISnapshot, error) { return nil, nil }
func (b testBackend) RestoreSnapshot(context.Context, string, int) error           { return nil }
func (b testBackend) OpenStream(context.Context, string) (sseConn, error)          { return sseConn{}, nil }

func TestSessionViewAltScreen(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession

	view := app.View()
	if !view.AltScreen {
		t.Error("app.View().AltScreen = false, want true")
	}
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("app.View().MouseMode = %v, want SGR (click-to-expand tool blocks)", view.MouseMode)
	}
}

// TestStaleDoneEventDoesNotCancelResumedSession verifies that a "done" SSE
// event from a previous session that is still in the bubbletea queue when the
// user switches to a new session does not affect the new session's state.
//
// Before the fix, handleSSEEvent("done") called a.sseCancel() unconditionally,
// which would cancel the fresh context created by startSSE if a stale "done"
// arrived after the switch.
func TestStaleDoneEventDoesNotCancelResumedSession(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-old"

	// Inject an active SSE context as if a stream is running for "sess-old".
	oldCtx, oldCancel := context.WithCancel(context.Background())
	app.sse.ctx = oldCtx
	app.sse.cancel = oldCancel

	// Step 1: the "done" event for the old session arrives as a normal completion.
	donePayload := SSEDonePayload{}
	donePayload.Usage.InputTokens = 100
	donePayload.ContextSize = 8000
	data, _ := json.Marshal(donePayload)

	updated, _ := app.Update(SSEEventMsg{
		SessionID: "sess-old",
		Type:      "done",
		Data:      data,
	})
	app = updated.(App)

	// Step 2: User switches to a previous session "sess-new".
	updated, _ = app.Update(sessionCreatedMsg{
		session: APISession{ID: "sess-new", AgentID: "agent-1", ModelID: "model-1"},
	})
	app = updated.(App)

	newCtx := app.sse.ctx
	if newCtx == nil {
		t.Fatal("sse.ctx is nil after sessionCreatedMsg; expected a fresh context")
	}

	// Step 3: A stale "done" for "sess-old" arrives after the session switch.
	updated, _ = app.Update(SSEEventMsg{
		SessionID: "sess-old",
		Type:      "done",
		Data:      data,
	})
	app = updated.(App)

	// The new session's context must NOT be cancelled.
	if app.sse.ctx != nil && app.sse.ctx.Err() != nil {
		t.Errorf("sse.ctx.Err() = %v after stale done event, want nil", app.sse.ctx.Err())
	}
}

// TestSwitchSessionResetsStateAndLoadsHistory verifies that switching to an
// existing session clears the previous session's messages and token stats, and
// populates the view with the loaded message history.
func TestSwitchSessionResetsStateAndLoadsHistory(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-old"

	// Simulate old session having messages and token stats.
	app.session.AppendUserMessage("old user message")
	app.session.SetTokens(500, 0, 8000, 0, 0)

	// Switch to an existing session with a loaded message history.
	history := []APIMessage{
		{ID: "m1", SessionID: "sess-prev", Role: "user", Content: "hello from history"},
		{ID: "m2", SessionID: "sess-prev", Role: "assistant", Content: "hi there from history"},
	}
	updated, _ := app.Update(sessionCreatedMsg{
		session:  APISession{ID: "sess-prev", AgentID: "agent-2", ModelID: "model-2"},
		messages: history,
	})
	app = updated.(App)

	if app.sessionID != "sess-prev" {
		t.Errorf("sessionID = %q, want %q", app.sessionID, "sess-prev")
	}

	msgs := app.session.msgs.messages
	if len(msgs) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(msgs))
	}
	if msgs[0].Content != "hello from history" {
		t.Errorf("messages[0].Content = %q, want %q", msgs[0].Content, "hello from history")
	}
	if msgs[1].Content != "hi there from history" {
		t.Errorf("messages[1].Content = %q, want %q", msgs[1].Content, "hi there from history")
	}

	if got := app.session.statusBar.contextSize; got != 0 {
		t.Errorf("statusBar.contextSize = %d after session switch, want 0", got)
	}
}

func TestDoneEventRefreshesTitleAfterCompletion(t *testing.T) {
	prevDelay := sessionTitleRefreshDelay
	prevAttempts := sessionTitleRefreshMaxAttempts
	sessionTitleRefreshDelay = 0
	sessionTitleRefreshMaxAttempts = 3
	defer func() {
		sessionTitleRefreshDelay = prevDelay
		sessionTitleRefreshMaxAttempts = prevAttempts
	}()

	var calls int
	app := NewAppWithBackend(testBackend{
		getSessionFn: func(_ context.Context, sessionID string) (APISession, error) {
			calls++
			if sessionID != "sess-1" {
				t.Fatalf("GetSession() sessionID = %q, want %q", sessionID, "sess-1")
			}
			if calls == 1 {
				return APISession{ID: sessionID}, nil
			}
			return APISession{ID: sessionID, Title: "Hello"}, nil
		},
	}, nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	data, _ := json.Marshal(SSEDonePayload{})
	updated, cmd := app.Update(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "done",
		Data:      data,
	})
	app = updated.(App)

	if cmd == nil {
		t.Fatal("done event should schedule a title refresh")
	}

	msg := cmd()
	updated, cmd = app.Update(msg)
	app = updated.(App)
	if app.session.header.title != "" {
		t.Fatalf("title after first refresh = %q, want empty before retry", app.session.header.title)
	}
	if cmd == nil {
		t.Fatal("empty title should schedule a retry")
	}

	msg = cmd()
	updated, _ = app.Update(msg)
	app = updated.(App)
	if app.session.header.title != "Hello" {
		t.Fatalf("title after retry = %q, want %q", app.session.header.title, "Hello")
	}
	if calls != 2 {
		t.Fatalf("GetSession() calls = %d, want 2", calls)
	}
}

func TestTurnQueueEventUpdatesQueuedIndicator(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	data, _ := json.Marshal(struct {
		Count int `json:"count"`
	}{Count: 1})
	updated, _ := app.Update(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "turn_queue",
		Data:      data,
	})
	app = updated.(App)

	if app.queuedTurns != 1 {
		t.Fatalf("queuedTurns = %d, want 1", app.queuedTurns)
	}
	if app.session.statusBar.queuedTurns != 1 {
		t.Fatalf("statusBar.queuedTurns = %d, want 1", app.session.statusBar.queuedTurns)
	}

	clearData, _ := json.Marshal(struct {
		Count int `json:"count"`
	}{Count: 0})
	updated, _ = app.Update(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "turn_queue",
		Data:      clearData,
	})
	app = updated.(App)

	if app.queuedTurns != 0 {
		t.Fatalf("queuedTurns = %d, want 0", app.queuedTurns)
	}
	if app.session.statusBar.queuedTurns != 0 {
		t.Fatalf("statusBar.queuedTurns = %d, want 0", app.session.statusBar.queuedTurns)
	}
}

func TestDoneEventKeepsStreamOpenWhenQueuedTurnPending(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"
	app.queuedTurns = 1
	app.session.SetQueuedTurns(1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan SSEEventMsg, 1)
	done := make(chan struct{})
	app.sse.ctx = ctx
	app.sse.cancel = cancel
	app.sse.conn = &sseConn{sessionID: "sess-1", events: events, done: done}

	data, _ := json.Marshal(SSEDonePayload{})
	updated, cmd := app.Update(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "done",
		Data:      data,
	})
	result := updated.(App)

	if !result.sse.IsConnected() {
		t.Fatal("sse should stay connected while a queued turn is pending")
	}
	if result.session.footer.streaming {
		t.Fatal("session should stop showing the current turn as streaming after done")
	}
	if cmd == nil {
		t.Fatal("done event should keep the SSE pump alive while a queued turn is pending")
	}
}

func TestProviderConnectedMsgUpdatesModelCatalog(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.cfg.Model = "openai/gpt-4.1"
	app.cfg.Models = map[string]ModelItem{
		"openai/gpt-4.1": {
			ProviderID:   "openai",
			ProviderName: "OpenAI",
			ModelID:      "gpt-4.1",
			ModelName:    "GPT-4.1",
		},
	}

	updated, _ := app.Update(providerConnectedMsg{
		providerID: "github-copilot",
		message:    `Provider "github-copilot" saved and activated.`,
		models: []APIProviderModels{
			{
				ProviderID:   "openai",
				ProviderName: "OpenAI",
				Models:       []APIModel{{ID: "gpt-4.1", Name: "GPT-4.1"}},
			},
			{
				ProviderID:   "github-copilot",
				ProviderName: "GitHub Copilot",
				Models:       []APIModel{{ID: "gpt-5.4", Name: "GPT-5.4"}},
			},
		},
	})
	result := updated.(App)

	if result.infoBanner != `Provider "github-copilot" saved and activated.` {
		t.Fatalf("infoBanner = %q", result.infoBanner)
	}
	if _, ok := result.cfg.Models["github-copilot/gpt-5.4"]; !ok {
		t.Fatal("github-copilot model missing from cfg.Models")
	}
}

func TestHandleSlashCommand_ConnectBlockedWhenTurnsActive(t *testing.T) {
	app := NewAppWithBackend(testBackend{
		hasActiveTurnsFn: func(context.Context) (bool, error) { return true, nil },
	}, nil)

	updated, cmd, handled := app.handleSlashCommand("/connect")
	if !handled {
		t.Fatal("/connect was not handled")
	}
	if cmd != nil {
		_ = cmd()
	}
	result := updated
	if result.errorBanner != providerConnectBusyMessage {
		t.Fatalf("errorBanner = %q, want %q", result.errorBanner, providerConnectBusyMessage)
	}
}

// TestSwitchSessionOldMessagesNotVisible verifies that after switching to a
// different session, messages from the old session are gone from the view.
func TestSwitchSessionOldMessagesNotVisible(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-old"

	// Old session has a message.
	app.session.AppendUserMessage("old session message")

	// Switch to a new session with no prior history, such as a freshly created one.
	updated, _ := app.Update(sessionCreatedMsg{
		session:  APISession{ID: "sess-new", AgentID: "agent-1", ModelID: "model-1"},
		text:     "first message",
		messages: nil,
	})
	app = updated.(App)

	for _, m := range app.session.msgs.messages {
		if strings.Contains(m.Content, "old session message") {
			t.Errorf("old session message visible after switch: %q", m.Content)
		}
	}
}

// silently discarded and not appended to the current session's message list.
//
// This is the root cause of "[error: sse connect: … context canceled]"
// appearing when the user opens a previous session: startSSE cancels the old
// context; if the old goroutine was still in client.Do it emits an error event;
// that stale event must be discarded, not rendered.
func TestStaleSSEErrorFromCancelledSessionNotDisplayed(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-new"

	errorPayload, _ := json.Marshal(struct {
		Message string `json:"message"`
	}{Message: "sse connect: Get \"http://[::]:57356/sessions/sess-old/stream\": context canceled"})

	// Deliver a stale error event that belongs to "sess-old", not "sess-new".
	updated, _ := app.Update(SSEEventMsg{
		SessionID: "sess-old",
		Type:      "error",
		Data:      errorPayload,
	})
	app = updated.(App)

	// The stale error must not have been appended to the messages list.
	for _, m := range app.session.msgs.messages {
		if strings.Contains(m.Content, "context canceled") {
			t.Errorf("stale error from old session was appended to messages, want it discarded; content: %q", m.Content)
		}
	}
}

func TestEscRequestsTurnCancellation(t *testing.T) {
	cancelCalls := 0
	app := NewAppWithBackend(testBackend{
		cancelTurnFn: func(_ context.Context, sessionID string) error {
			cancelCalls++
			if sessionID != "sess-1" {
				t.Fatalf("CancelTurn sessionID = %q, want %q", sessionID, "sess-1")
			}
			return nil
		},
	}, nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	// Simulate an active SSE connection.
	ctx, cancel := context.WithCancel(context.Background())
	app.sse.ctx = ctx
	app.sse.cancel = cancel
	events := make(chan SSEEventMsg, 1)
	done := make(chan struct{})
	app.sse.conn = &sseConn{sessionID: "sess-1", events: events, done: done}

	// Add some streaming content.
	app.session.AppendDelta("I'll help you with that.")

	// Press ESC.
	updated, cmd := app.Update(tea.KeyPressMsg{Code: 27, Text: ""})
	result := updated.(App)

	if !result.sse.IsConnected() {
		t.Fatal("sse should stay connected until cancellation completes")
	}
	if result.infoBanner != "Cancelling turn..." {
		t.Fatalf("infoBanner = %q, want %q", result.infoBanner, "Cancelling turn...")
	}

	if cmd == nil {
		t.Fatal("ESC should dispatch a cancel command")
	}
	msg := cmd()
	updated, _ = result.Update(msg)
	result = updated.(App)
	if cancelCalls != 1 {
		t.Fatalf("cancelCalls = %d, want 1", cancelCalls)
	}

	payload, _ := json.Marshal(struct {
		Message string `json:"message"`
	}{Message: "turn cancelled"})
	updated, _ = result.Update(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "error",
		Data:      payload,
	})
	result = updated.(App)

	if result.sse.IsConnected() {
		t.Error("sse should not be connected after cancellation event")
	}

	if result.infoBanner != "Turn cancelled." {
		t.Fatalf("infoBanner = %q, want %q", result.infoBanner, "Turn cancelled.")
	}
	found := false
	for _, m := range result.session.msgs.messages {
		if strings.Contains(m.Content, "[cancelled]") {
			found = true
			break
		}
	}
	if !found {
		t.Error("should show [cancelled] message after turn cancellation")
	}
}

func TestEscCancelledAsyncSendErrorDoesNotShowErrorBanner(t *testing.T) {
	cancelCalls := 0
	app := NewAppWithBackend(testBackend{
		cancelTurnFn: func(_ context.Context, sessionID string) error {
			cancelCalls++
			if sessionID != "sess-1" {
				t.Fatalf("CancelTurn sessionID = %q, want %q", sessionID, "sess-1")
			}
			return nil
		},
	}, nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	ctx, cancel := context.WithCancel(context.Background())
	app.sse.ctx = ctx
	app.sse.cancel = cancel
	events := make(chan SSEEventMsg, 1)
	done := make(chan struct{})
	app.sse.conn = &sseConn{sessionID: "sess-1", events: events, done: done}

	app.session.AppendDelta("streaming...")

	updated, cmd := app.Update(tea.KeyPressMsg{Code: 27, Text: ""})
	result := updated.(App)
	if cmd == nil {
		t.Fatal("ESC should dispatch a cancel command")
	}
	updated, _ = result.Update(cmd())
	result = updated.(App)
	if cancelCalls != 1 {
		t.Fatalf("cancelCalls = %d, want 1", cancelCalls)
	}

	updated, _ = result.Update(SSEErrorMsg{
		SessionID: "sess-1",
		Err:       fmt.Errorf("send: pipeline: %w", context.Canceled),
	})
	result = updated.(App)

	if result.errorBanner != "" {
		t.Fatalf("errorBanner = %q, want empty", result.errorBanner)
	}
	if result.infoBanner != "Turn cancelled." {
		t.Fatalf("infoBanner = %q, want %q", result.infoBanner, "Turn cancelled.")
	}
	if result.sse.IsConnected() {
		t.Fatal("sse should not stay connected after a cancellation-shaped async error")
	}
}

func TestEscCancelledSSEErrorEventDoesNotShowErrorBanner(t *testing.T) {
	cancelCalls := 0
	app := NewAppWithBackend(testBackend{
		cancelTurnFn: func(_ context.Context, sessionID string) error {
			cancelCalls++
			if sessionID != "sess-1" {
				t.Fatalf("CancelTurn sessionID = %q, want %q", sessionID, "sess-1")
			}
			return nil
		},
	}, nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	ctx, cancel := context.WithCancel(context.Background())
	app.sse.ctx = ctx
	app.sse.cancel = cancel
	events := make(chan SSEEventMsg, 1)
	done := make(chan struct{})
	app.sse.conn = &sseConn{sessionID: "sess-1", events: events, done: done}

	updated, cmd := app.Update(tea.KeyPressMsg{Code: 27, Text: ""})
	result := updated.(App)
	if cmd == nil {
		t.Fatal("ESC should dispatch a cancel command")
	}
	updated, _ = result.Update(cmd())
	result = updated.(App)
	if cancelCalls != 1 {
		t.Fatalf("cancelCalls = %d, want 1", cancelCalls)
	}

	payload, _ := json.Marshal(struct {
		Message string `json:"message"`
	}{Message: "send: pipeline: context canceled"})
	updated, _ = result.Update(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "error",
		Data:      payload,
	})
	result = updated.(App)

	if result.errorBanner != "" {
		t.Fatalf("errorBanner = %q, want empty", result.errorBanner)
	}
	if result.infoBanner != "Turn cancelled." {
		t.Fatalf("infoBanner = %q, want %q", result.infoBanner, "Turn cancelled.")
	}
	if result.sse.IsConnected() {
		t.Fatal("sse should not stay connected after a cancellation-shaped SSE error event")
	}
}

func TestCancellationShapedSSEErrorMsgWithoutCancelRequestedDoesNotShowErrorBanner(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	ctx, cancel := context.WithCancel(context.Background())
	app.sse.ctx = ctx
	app.sse.cancel = cancel
	events := make(chan SSEEventMsg, 1)
	done := make(chan struct{})
	app.sse.conn = &sseConn{sessionID: "sess-1", events: events, done: done}

	updated, _ := app.Update(SSEErrorMsg{
		SessionID: "sess-1",
		Err:       fmt.Errorf("send: pipeline: context canceled"),
	})
	result := updated.(App)

	if result.errorBanner != "" {
		t.Fatalf("errorBanner = %q, want empty", result.errorBanner)
	}
	if result.infoBanner != "Turn cancelled." {
		t.Fatalf("infoBanner = %q, want %q", result.infoBanner, "Turn cancelled.")
	}
	if result.sse.IsConnected() {
		t.Fatal("sse should not stay connected after a cancellation-shaped error")
	}
}

func TestStaleSSEErrorMsgFromOtherSessionIgnored(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-new"

	updated, _ := app.Update(SSEErrorMsg{
		SessionID: "sess-old",
		Err:       fmt.Errorf("send: pipeline: context canceled"),
	})
	result := updated.(App)

	if result.errorBanner != "" {
		t.Fatalf("errorBanner = %q, want empty", result.errorBanner)
	}
	if result.infoBanner != "" {
		t.Fatalf("infoBanner = %q, want empty", result.infoBanner)
	}
}

func TestPasteFilePathAddsPendingAttachment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mypdf.pdf")
	if err := os.WriteFile(path, []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession

	updated, _ := app.Update(tea.PasteMsg{Content: path})
	result := updated.(App)

	if len(result.pendingAttachments) != 1 {
		t.Fatalf("len(pendingAttachments) = %d, want 1", len(result.pendingAttachments))
	}
	if result.pendingAttachments[0].Path != path {
		t.Fatalf("attachment path = %q, want %q", result.pendingAttachments[0].Path, path)
	}
	if got := result.session.footer.input.Value(); got != "" {
		t.Fatalf("input value = %q, want empty", got)
	}
	if got := result.session.footer.attachmentPrompt(); got != "  [pdf mypdf.pdf #1] " {
		t.Fatalf("attachment prompt = %q, want %q", got, "  [pdf mypdf.pdf #1] ")
	}
	if len(result.session.footer.pendingAttachments) != 1 {
		t.Fatalf("len(session footer pendingAttachments) = %d, want 1", len(result.session.footer.pendingAttachments))
	}
}

func TestClipboardPastedFilePathAddsPendingAttachment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeHome

	updated, _ := app.Update(clipboardPastedMsg{text: path})
	result := updated.(App)

	if len(result.pendingAttachments) != 1 {
		t.Fatalf("len(pendingAttachments) = %d, want 1", len(result.pendingAttachments))
	}
	if result.pendingAttachments[0].Path != path {
		t.Fatalf("attachment path = %q, want %q", result.pendingAttachments[0].Path, path)
	}
	if got := result.home.footer.input.Value(); got != "" {
		t.Fatalf("input value = %q, want empty", got)
	}
	if got := result.home.footer.attachmentPrompt(); got != "  [Image image.png #1] " {
		t.Fatalf("attachment prompt = %q, want %q", got, "  [Image image.png #1] ")
	}
	if len(result.home.footer.pendingAttachments) != 1 {
		t.Fatalf("len(home footer pendingAttachments) = %d, want 1", len(result.home.footer.pendingAttachments))
	}
}

func TestPasteDuplicateFilePathDoesNotAddSecondAttachment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(path, []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession

	updated, _ := app.Update(tea.PasteMsg{Content: path})
	result := updated.(App)
	updated, _ = result.Update(tea.PasteMsg{Content: path})
	result = updated.(App)

	if len(result.pendingAttachments) != 1 {
		t.Fatalf("len(pendingAttachments) = %d, want 1", len(result.pendingAttachments))
	}
	if got := result.session.footer.input.Value(); got != "" {
		t.Fatalf("input value = %q, want empty", got)
	}
	if got := result.session.footer.attachmentPrompt(); got != "  [pdf doc.pdf #1] " {
		t.Fatalf("attachment prompt = %q, want %q", got, "  [pdf doc.pdf #1] ")
	}
}

func TestBackspaceRemovesLastInlineAttachment(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "mypdf.pdf")
	second := filepath.Join(dir, "image.png")
	if err := os.WriteFile(first, []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession

	updated, _ := app.Update(tea.PasteMsg{Content: first})
	result := updated.(App)
	updated, _ = result.Update(tea.PasteMsg{Content: second})
	result = updated.(App)

	if got := result.session.footer.input.Value(); got != "" {
		t.Fatalf("input value before backspace = %q, want empty", got)
	}
	if got := result.session.footer.attachmentPrompt(); got != "  [pdf mypdf.pdf #1] [Image image.png #2] " {
		t.Fatalf("attachment prompt before backspace = %q", got)
	}

	updated, _ = result.Update(attachmentRemoveMsg{index: 1})
	result = updated.(App)

	if len(result.pendingAttachments) != 1 {
		t.Fatalf("len(pendingAttachments) = %d, want 1", len(result.pendingAttachments))
	}
	if got := result.session.footer.input.Value(); got != "" {
		t.Fatalf("input value after backspace = %q, want empty", got)
	}
	if got := result.session.footer.attachmentPrompt(); got != "  [pdf mypdf.pdf #1] " {
		t.Fatalf("attachment prompt after backspace = %q, want %q", got, "  [pdf mypdf.pdf #1] ")
	}
}

func TestAssistantMessageEventAppendsCompletedAssistantMessage(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	payload, _ := json.Marshal(struct {
		Content string `json:"content"`
	}{Content: "## Goal\nShip the feature"})

	updated, _ := app.Update(SSEEventMsg{
		SessionID: "sess-1",
		Type:      "assistant_message",
		Data:      payload,
	})
	result := updated.(App)

	if len(result.session.msgs.messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(result.session.msgs.messages))
	}
	msg := result.session.msgs.messages[0]
	if msg.Role != "assistant" {
		t.Fatalf("message role = %q, want assistant", msg.Role)
	}
	if msg.Content != "## Goal\nShip the feature" {
		t.Fatalf("message content = %q", msg.Content)
	}
	if msg.Streaming {
		t.Fatal("assistant_message should append a completed message")
	}
}

func TestCompletePlannerApprovalAfterDone_RestoresAgentAndQueuesExecution(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"
	app.session.SetSize(80, 24)
	app.cfg.AgentNames = map[string]string{
		"engineer": "Engineer",
		"planner":  "Planner",
	}
	app.cfg.Agent = "planner"
	app.cfg.AgentName = "Planner"
	app.session.SetAgent("planner", "Planner")
	app.cfg.PreplanAgent = "engineer"
	app.cfg.PlannerPending = true
	app.cfg.PlannerChoice = planApprovalGoLabel

	model, cmd := app.completePlannerApprovalAfterDone()
	result := model.(App)

	if result.cfg.Agent != "engineer" {
		t.Fatalf("defaultAgent = %q, want %q", result.cfg.Agent, "engineer")
	}
	if result.session.header.agentID != "engineer" {
		t.Fatalf("session agent = %q, want %q", result.session.header.agentID, "engineer")
	}
	if result.cfg.PlannerPending {
		t.Fatal("plannerApprovalPending should be cleared")
	}
	if cmd == nil {
		t.Fatal("expected follow-up execution command")
	}
}

func TestHandleAgentsLoaded_FallsBackToFormattedDefaultAgentName(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.cfg.Agent = "engineer"

	result, _ := app.handleAgentsLoaded(agentsLoadedMsg{
		agents: []APIAgent{
			{ID: "builder", Name: "builder"},
		},
	})

	if result.cfg.AgentName != "Engineer" {
		t.Fatalf("AgentName = %q, want %q", result.cfg.AgentName, "Engineer")
	}
}

func TestCycleAgent_DebouncesLastAgentPersistence(t *testing.T) {
	prevDelay := agentPersistDebounce
	agentPersistDebounce = 0
	defer func() {
		agentPersistDebounce = prevDelay
	}()

	var writes []string
	app := NewAppWithBackend(testBackend{
		setSettingFn: func(_ context.Context, key, value string) error {
			if key != "last_agent" {
				t.Fatalf("SetSetting() key = %q, want %q", key, "last_agent")
			}
			writes = append(writes, value)
			return nil
		},
	}, nil)
	app.cfg.Agent = "engineer"
	app.cfg.AgentNames = map[string]string{
		"engineer": "Engineer",
		"reviewer": "Reviewer",
		"writer":   "Writer",
	}
	app.cfg.PrimaryAgentIDs = []string{"engineer", "reviewer", "writer"}

	cmd1 := app.cycleAgent()
	cmd2 := app.cycleAgent()
	if app.cfg.Agent != "writer" {
		t.Fatalf("Agent after two cycles = %q, want %q", app.cfg.Agent, "writer")
	}

	msg := cmd1()
	updated, cmd := app.Update(msg)
	app = updated.(App)
	if cmd != nil {
		t.Fatal("stale debounce tick should not trigger persistence")
	}

	msg = cmd2()
	updated, cmd = app.Update(msg)
	app = updated.(App)
	if cmd == nil {
		t.Fatal("latest debounce tick should trigger persistence")
	}

	msg = cmd()
	updated, _ = app.Update(msg)
	app = updated.(App)

	if len(writes) != 1 {
		t.Fatalf("SetSetting() writes = %d, want 1", len(writes))
	}
	if writes[0] != "writer" {
		t.Fatalf("persisted agent = %q, want %q", writes[0], "writer")
	}
	if app.agentPersistDirty {
		t.Fatal("agentPersistDirty should be cleared after successful persist")
	}
}

func TestHandleConfigLoaded_DoesNotInferAgentFromPreviousSessions(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.api = testBackend{
		listAgentsFn: func(context.Context) ([]APIAgent, error) {
			return []APIAgent{
				{ID: "builder", Name: "builder"},
				{ID: "engineer", Name: "engineer"},
				{ID: "planner", Name: "Planner", Mode: "subagent"},
			}, nil
		},
		listSessionsFn: func(context.Context) ([]APISession, error) {
			return []APISession{
				{ID: "old", AgentID: "builder"},
			}, nil
		},
		getSettingFn: func(_ context.Context, key string) (string, error) {
			return "", nil
		},
	}

	_, cmd := app.handleConfigLoaded(configLoadedMsg{
		cfg: APIConfig{DefaultAgent: "builder"},
	})
	if cmd == nil {
		t.Fatal("handleConfigLoaded returned nil cmd")
	}

	msg, ok := cmd().(agentsLoadedMsg)
	if !ok {
		t.Fatalf("cmd() type = %T, want agentsLoadedMsg", cmd())
	}
	if msg.lastAgentID != "" {
		t.Fatalf("lastAgentID = %q, want empty", msg.lastAgentID)
	}
}

func TestOpenSessionDialog_IncludesUntitledSessions(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.api = testBackend{
		listSessionsFn: func(context.Context) ([]APISession, error) {
			return []APISession{
				{ID: "sess-1", Title: "", AgentID: "engineer"},
				{ID: "sess-2", Title: "Named", AgentID: "builder"},
			}, nil
		},
	}

	msg := app.openSessionDialog()()
	openMsg, ok := msg.(openDialogMsg)
	if !ok {
		t.Fatalf("msg type = %T, want openDialogMsg", msg)
	}
	dialog, ok := openMsg.dialog.(SessionDialog)
	if !ok {
		t.Fatalf("dialog type = %T, want SessionDialog", openMsg.dialog)
	}
	if len(dialog.sessions) != 2 {
		t.Fatalf("len(dialog.sessions) = %d, want 2", len(dialog.sessions))
	}
	if got := sessionLabel(dialog.sessions[0]); got != "Untitled" {
		t.Fatalf("sessionLabel(empty title) = %q, want %q", got, "Untitled")
	}
}

func TestBuildSlashCommands_ContainsRefresh(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	cmds := app.buildSlashCommands()
	found := false
	for _, c := range cmds {
		if c.Name == "/refresh" {
			found = true
			break
		}
	}
	if !found {
		t.Error("buildSlashCommands should include /refresh")
	}
}

func TestHandleSlashCommand_KnownCommands(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	app.session.SetSize(80, 24)

	// Commands that should be handled (return true).
	handled := []string{
		"/help", "/new", "/pins", "/palette",
		"/search test", "/variant",
	}
	for _, cmd := range handled {
		_, _, ok := app.handleSlashCommand(cmd)
		if !ok {
			t.Errorf("handleSlashCommand(%q) not handled, want handled", cmd)
		}
	}

	// Non-slash text should not be handled.
	_, _, ok := app.handleSlashCommand("hello world")
	if ok {
		t.Error("handleSlashCommand should not handle non-slash text")
	}

	// Unknown command should still be handled (shows error toast).
	_, _, ok = app.handleSlashCommand("/nonexistent")
	if !ok {
		t.Error("handleSlashCommand should handle unknown /commands (to show error)")
	}
}

func TestHandleSlashCommand_PinRequiresActiveSession(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeHome

	updated, cmd, handled := app.handleSlashCommand("/pin keep this")
	if !handled {
		t.Fatal("/pin should be handled")
	}
	if cmd != nil {
		t.Fatal("/pin without an active session should not dispatch a command")
	}
	if !strings.Contains(updated.errorBanner, "Pins are session-scoped") {
		t.Fatalf("errorBanner = %q, want session-scoped guidance", updated.errorBanner)
	}
}

func TestHandleSlashCommand_PinPersistsBeforeUpdatingUIState(t *testing.T) {
	var savedKey, savedValue string
	app := NewAppWithBackend(testBackend{
		setSettingFn: func(_ context.Context, key, value string) error {
			savedKey = key
			savedValue = value
			return nil
		},
	}, nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"
	app.session.SetSize(80, 24)

	updated, cmd, handled := app.handleSlashCommand("/pin keep this")
	if !handled {
		t.Fatal("/pin should be handled")
	}
	if cmd == nil {
		t.Fatal("/pin should return a persistence command")
	}
	if len(updated.pins) != 0 {
		t.Fatalf("pins updated optimistically = %#v, want no local change before persistence result", updated.pins)
	}

	rawMsg := cmd()
	msg, ok := rawMsg.(pinsPersistedMsg)
	if !ok {
		t.Fatalf("cmd() type = %T, want pinsPersistedMsg", rawMsg)
	}
	if savedKey != "pins:sess-1" {
		t.Fatalf("saved key = %q, want pins:sess-1", savedKey)
	}
	if !strings.Contains(savedValue, "keep this") {
		t.Fatalf("saved value = %q, want pinned instruction", savedValue)
	}

	model, _ := updated.Update(msg)
	result := model.(App)
	if len(result.pins) != 1 || result.pins[0] != "keep this" {
		t.Fatalf("pins after persistence = %#v, want [keep this]", result.pins)
	}
	if got := result.session.statusBar.pinCount; got != 1 {
		t.Fatalf("pinCount = %d, want 1", got)
	}
}

func TestHandleSlashCommand_PinPersistenceFailureDoesNotMutatePins(t *testing.T) {
	app := NewAppWithBackend(testBackend{
		setSettingFn: func(_ context.Context, key, value string) error {
			return errors.New("settings write failed")
		},
	}, nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"
	app.session.SetSize(80, 24)
	app.pins = []string{"existing"}
	app.session.SetPinCount(1)

	updated, cmd, handled := app.handleSlashCommand("/pin new")
	if !handled {
		t.Fatal("/pin should be handled")
	}
	if cmd == nil {
		t.Fatal("/pin should return a persistence command")
	}

	model, _ := updated.Update(cmd())
	result := model.(App)
	if !strings.Contains(result.errorBanner, "persist pins") {
		t.Fatalf("errorBanner = %q, want persistence failure", result.errorBanner)
	}
	if len(result.pins) != 1 || result.pins[0] != "existing" {
		t.Fatalf("pins after failure = %#v, want unchanged existing pin", result.pins)
	}
	if got := result.session.statusBar.pinCount; got != 1 {
		t.Fatalf("pinCount = %d, want unchanged count 1", got)
	}
}

func TestPinsPersistedMsg_ShowsFailureAfterSessionSwitch(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-2"
	app.session.SetSize(80, 24)
	app.pins = []string{"current"}
	app.session.SetPinCount(1)

	model, _ := app.Update(pinsPersistedMsg{
		sessionID: "sess-1",
		err:       errors.New("persist pins: settings write failed"),
	})
	result := model.(App)
	if !strings.Contains(result.errorBanner, "persist pins") {
		t.Fatalf("errorBanner = %q, want pin persistence failure", result.errorBanner)
	}
	if len(result.pins) != 1 || result.pins[0] != "current" {
		t.Fatalf("pins after stale failure = %#v, want current session pins unchanged", result.pins)
	}
	if got := result.session.statusBar.pinCount; got != 1 {
		t.Fatalf("pinCount = %d, want current session count unchanged", got)
	}
}

func TestReloadCommand_ReturnsDeterministicResult(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	_, cmd, handled := app.handleSlashCommand("/reload")
	if !handled {
		t.Fatal("/reload should be handled")
	}
	if cmd == nil {
		t.Fatal("/reload should return a tea.Cmd")
	}
	// Execute the command. It should return a reloadResultMsg and not trigger a model call.
	msg := cmd()
	if _, ok := msg.(reloadResultMsg); !ok {
		t.Fatalf("/reload returned %T, want reloadResultMsg", msg)
	}
}

func TestUndoCommand_ReturnsDeterministicResult(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	_, cmd, handled := app.handleSlashCommand("/undo")
	if !handled {
		t.Fatal("/undo should be handled")
	}
	if cmd == nil {
		t.Fatal("/undo should return a tea.Cmd")
	}
	msg := cmd()
	if _, ok := msg.(undoResultMsg); !ok {
		t.Fatalf("/undo returned %T, want undoResultMsg", msg)
	}
}

func TestUndoConfirmRequiresPreview(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	updated, cmd, handled := app.handleSlashCommand("/undo confirm foo.txt")
	if !handled {
		t.Fatal("/undo confirm should be handled")
	}
	if cmd != nil {
		t.Fatal("/undo confirm without preview should not dispatch a command")
	}
	if updated.errorBanner == "" || !strings.Contains(updated.errorBanner, "Preview required") {
		t.Fatalf("errorBanner = %q, want preview guidance", updated.errorBanner)
	}
}

func TestUndoResultMsgTracksPendingUndoFile(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"
	app.session.SetSize(app.width, app.height)

	updated, _ := app.Update(undoResultMsg{
		output:      "Preview",
		pendingFile: "foo.txt",
	})
	app = updated.(App)
	if app.pendingUndoFile != "foo.txt" {
		t.Fatalf("pendingUndoFile = %q, want foo.txt", app.pendingUndoFile)
	}

	updated, _ = app.Update(undoResultMsg{
		output:       "Reverted",
		clearPending: true,
	})
	app = updated.(App)
	if app.pendingUndoFile != "" {
		t.Fatalf("pendingUndoFile = %q, want empty after clear", app.pendingUndoFile)
	}
}

func TestAgentCommand_SpawnsSubagentDirectly(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"
	app.cfg.AgentIDs = []string{"explorer", "planner"}
	app.cfg.AgentNames = map[string]string{
		"explorer": "Explorer",
		"planner":  "Planner",
	}

	_, cmd, handled := app.handleSlashCommand("/explorer")
	if !handled {
		t.Fatal("/explorer should be handled")
	}
	if cmd == nil {
		t.Fatal("/explorer should return a tea.Cmd")
	}
	// The command returns a tea.BatchMsg containing the spawn command and
	// (optionally) an SSE connect command. Extract the spawn result.
	msg := cmd()
	batch, isBatch := msg.(tea.BatchMsg)
	if !isBatch {
		t.Fatalf("/explorer returned %T, want tea.BatchMsg", msg)
	}
	foundSpawn := false
	for _, c := range batch {
		if c == nil {
			continue
		}
		if inner := c(); inner != nil {
			if _, ok := inner.(subagentSpawnedMsg); ok {
				foundSpawn = true
			}
		}
	}
	if !foundSpawn {
		t.Fatal("/explorer batch did not contain subagentSpawnedMsg")
	}
}

func TestEscClearsErrorToast(t *testing.T) {
	app := NewApp("http://localhost:0", nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.route = routeSession
	app.sessionID = "sess-1"

	// Show an error banner.
	app.errorBanner = "all 5 retry attempts failed. Please try again."

	// Press ESC. It should clear the error banner.
	updated, _ := app.Update(tea.KeyPressMsg{Code: 27, Text: ""})
	result := updated.(App)

	if result.errorBanner != "" {
		t.Errorf("errorBanner = %q, want empty (ESC should clear error banner)", result.errorBanner)
	}
}
