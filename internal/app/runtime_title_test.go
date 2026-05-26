package app

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
)

func TestRuntimeRunSessionTurnGeneratesSessionTitleWithRawUtilityRequest(t *testing.T) {
	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	titleClient := &recordingStreamProvider{
		stream: provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Title: terminal shell · split operator"},
		}),
	}
	runtime := newRuntimeWithClient(t, mainClient)
	runtime.enableSessionTitles = true
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return titleClient, nil
	}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {
				{ID: "gpt-5-mini", Name: "GPT-5 mini", ContextSize: 128000, CostInput: 0.25, CostOutput: 2},
				{ID: "gpt-5", Name: "GPT-5", ContextSize: 128000, CostInput: 1.25, CostOutput: 10},
			},
		},
	}

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "split a shell command",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state := waitForSessionTitle(t, runtime, result.SessionID)
	if got := state.Title; got != "terminal shell split operator" {
		t.Fatalf("title = %q", got)
	}
	if len(mainClient.requests) != 1 {
		t.Fatalf("main request count = %d, want 1", len(mainClient.requests))
	}
	if len(titleClient.requests) != 1 {
		t.Fatalf("title request count = %d, want 1", len(titleClient.requests))
	}

	titleReq := titleClient.requests[0]
	if got := titleReq.Model.String(); got != "openai/gpt-5" {
		t.Fatalf("title model = %q, want openai/gpt-5", got)
	}
	if got := titleReq.AgentID; got != sessionTitleAgentID {
		t.Fatalf("title agent_id = %q", got)
	}
	if got := titleReq.Instructions; got != sessionTitlePrompt {
		t.Fatalf("title instructions = %q", got)
	}
	if len(titleReq.Tools) != 0 {
		t.Fatalf("title tools = %#v, want none", titleReq.Tools)
	}
	if len(titleReq.Inputs) != 1 || titleReq.Inputs[0].Content != "split a shell command" {
		t.Fatalf("title inputs = %#v", titleReq.Inputs)
	}
}

func TestRuntimeRunSessionTurnPrefersExplicitUtilityModelForTitleGeneration(t *testing.T) {
	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	titleClient := &recordingStreamProvider{
		stream: provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "shell split operator"},
		}),
	}
	runtime := newRuntimeWithClient(t, mainClient)
	runtime.enableSessionTitles = true
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return titleClient, nil
	}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {
				{ID: "gpt-4.1-mini", Name: "GPT-4.1 mini", ContextSize: 128000, CostInput: 0.15, CostOutput: 0.6},
			},
		},
	}

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "split a shell command",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	_ = waitForSessionTitle(t, runtime, result.SessionID)

	if len(titleClient.requests) != 1 {
		t.Fatalf("title request count = %d, want 1", len(titleClient.requests))
	}
	if got := titleClient.requests[0].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("title model = %q, want explicit utility model", got)
	}
}

func TestRuntimeRunSessionTurnFallsBackToPrimaryModelWhenUtilityTitleRequestFails(t *testing.T) {
	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	titleClient := &fakeProvider{
		errs: []error{
			&provider.ProviderError{Message: "rate limited", Retryable: true, RetryAfter: 72 * time.Hour},
			nil,
		},
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Recovered title"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, mainClient)
	runtime.enableSessionTitles = true
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return titleClient, nil
	}

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "split a shell command",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state := waitForSessionTitle(t, runtime, result.SessionID)
	if got := state.Title; got != "Recovered title" {
		t.Fatalf("title = %q", got)
	}
	if len(titleClient.requests) != 2 {
		t.Fatalf("title request count = %d, want 2", len(titleClient.requests))
	}
	if got := titleClient.requests[0].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("first title model = %q, want utility model", got)
	}
	if got := titleClient.requests[1].Model.String(); got != "openai/gpt-5" {
		t.Fatalf("second title model = %q, want primary fallback", got)
	}
}

func TestRuntimeRunSessionTurnStartsSessionTitleGenerationBeforeTurnCompletes(t *testing.T) {
	mainGate := make(chan struct{})
	titleRequested := make(chan struct{})

	mainClient := &fakeProvider{
		streams: []provider.Stream{
			&gatedEventStream{
				allow: mainGate,
				events: []provider.Event{
					{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
				},
			},
		},
	}
	titleClient := &recordingStreamProvider{
		stream: provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Shell split operator"},
		}),
		onRequest: func(provider.Request) {
			select {
			case <-titleRequested:
			default:
				close(titleRequested)
			}
		},
	}
	runtime := newRuntimeWithClient(t, mainClient)
	runtime.enableSessionTitles = true
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return titleClient, nil
	}

	type runResult struct {
		result RunSessionResult
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
			WorkspaceRoot: t.TempDir(),
			UserText:      "split a shell command",
		})
		done <- runResult{result: result, err: err}
	}()

	select {
	case <-titleRequested:
	case finished := <-done:
		t.Fatalf("turn completed before title request started: %#v", finished)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for title request to start")
	}

	close(mainGate)

	finished := <-done
	if finished.err != nil {
		t.Fatalf("RunSessionTurn() error = %v", finished.err)
	}
	if finished.result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", finished.result)
	}
	state := waitForSessionTitle(t, runtime, finished.result.SessionID)
	if got := state.Title; got != "Shell split operator" {
		t.Fatalf("title = %q", got)
	}
}

func TestRuntimeSessionTitlePersistsAfterSecondTurnStarts(t *testing.T) {
	titleGate := make(chan struct{})
	titleRequested := make(chan struct{})

	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second"},
			}),
		},
	}
	titleClient := &recordingStreamProvider{
		stream: &gatedEventStream{
			allow: titleGate,
			events: []provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Session starter title"},
			},
		},
		onRequest: func(provider.Request) {
			select {
			case <-titleRequested:
			default:
				close(titleRequested)
			}
		},
	}
	runtime := newRuntimeWithClient(t, mainClient)
	runtime.enableSessionTitles = true
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return titleClient, nil
	}

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "start the session",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn(first) error = %v", err)
	}
	if first.Status != TurnRunStatusCompleted {
		t.Fatalf("first result = %#v", first)
	}
	select {
	case <-titleRequested:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for title request to start")
	}

	second, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: first.SessionID,
		TurnID:    NewTurnID(),
		UserText:  "continue the session",
	})
	if err != nil {
		t.Fatalf("StartSessionTurn(second) error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second result = %#v", second)
	}
	if len(titleClient.requests) != 1 {
		t.Fatalf("title request count after second turn = %d, want 1", len(titleClient.requests))
	}

	close(titleGate)

	state := waitForSessionTitle(t, runtime, first.SessionID)
	if got := state.Title; got != "Session starter title" {
		t.Fatalf("title = %q", got)
	}
}

func TestRuntimeGenerateSessionTitleUsesCompletedTurnModelWhenUtilityUnset(t *testing.T) {
	fake := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Cache review"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, fake)
	ctx := context.Background()

	sessionID, err := runtime.CreateSession(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := runtime.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    "turn-1",
		Type:      events.TypeUserMessage,
		Payload:   events.UserMessagePayload{Content: "review cache behavior"},
	}); err != nil {
		t.Fatalf("append user message error = %v", err)
	}
	if _, err := runtime.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    "turn-1",
		Type:      events.TypeTurnConfigured,
		Payload: newTurnConfiguredPayload(
			TurnCapabilities{
				AgentID:    "builder",
				ModelRoute: provider.ModelRoute{Primary: provider.ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"}},
				AllowedTools: []string{
					"read",
				},
			},
			false,
			false,
			"",
			ResponseStyleDefault,
			false,
		),
	}); err != nil {
		t.Fatalf("append turn configured error = %v", err)
	}
	if _, err := runtime.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    "turn-1",
		Type:      events.TypeTurnDone,
		Payload:   events.TurnDonePayload{},
	}); err != nil {
		t.Fatalf("append turn done error = %v", err)
	}

	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}
	runtime.Config.CompatibleProviders = map[string]OpenAICompatibleProviderConfig{
		"proxy": {
			ProviderID: "proxy",
			APIKey:     "test-key",
			BaseURL:    "https://proxy.example/v1",
		},
	}
	runtime.generateSessionTitle(ctx, sessionID, "turn-1", "review cache behavior", provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"},
	})

	state := waitForSessionTitle(t, runtime, sessionID)
	if got := state.Title; got != "Cache review" {
		t.Fatalf("title = %q", got)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(fake.requests))
	}
	if got := fake.requests[0].Model.String(); got != "proxy/gpt-4.1" {
		t.Fatalf("title model = %q, want active turn model", got)
	}
}

func TestRuntimeGenerateSessionTitleStopsWhenParentContextIsCanceled(t *testing.T) {
	mainClient := &fakeProvider{}
	titleStarted := make(chan struct{})
	titleCanceled := make(chan struct{})
	runtime := newRuntimeWithClient(t, mainClient)
	runtime.enableSessionTitles = true
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return &cancelAwareTitleProvider{
			started:  titleStarted,
			canceled: titleCanceled,
		}, nil
	}

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.Sessions.append(context.Background(), events.Draft{
		SessionID: sessionID,
		TurnID:    "turn-1",
		Type:      events.TypeUserMessage,
		Payload:   events.UserMessagePayload{Content: "review cache behavior"},
	}); err != nil {
		t.Fatalf("append user message error = %v", err)
	}
	if _, err := runtime.Sessions.append(context.Background(), events.Draft{
		SessionID: sessionID,
		TurnID:    "turn-1",
		Type:      events.TypeTurnConfigured,
		Payload: newTurnConfiguredPayload(
			TurnCapabilities{
				AgentID:    "builder",
				ModelRoute: baseModelRoute(),
			},
			false,
			false,
			"",
			ResponseStyleDefault,
			false,
		),
	}); err != nil {
		t.Fatalf("append turn configured error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runtime.generateSessionTitle(ctx, sessionID, "turn-1", "review cache behavior", baseModelRoute())
		close(done)
	}()

	select {
	case <-titleStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for title request to start")
	}
	cancel()

	select {
	case <-titleCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for title request cancellation")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for title generation to stop")
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := strings.TrimSpace(state.Title); got != "" {
		t.Fatalf("title = %q, want empty after cancellation", got)
	}
}

func TestRuntimeSessionTitleLogsRawSSEForGitHubCopilot(t *testing.T) {
	t.Setenv("KODACODE_LOG_RAW_SSE", "1")
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	titleClient := &recordingStreamProvider{
		stream: provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Cache review"},
		}),
		onRequest: func(req provider.Request) {
			if req.RawSSEObserver == nil {
				t.Fatal("RawSSEObserver = nil, want GitHub Copilot title raw response logging")
			}
			req.RawSSEObserver(provider.RawSSEFrame{
				APIMode:  "responses",
				Sequence: 1,
				Event:    "response.completed",
				Data:     []byte(`{"type":"response.completed","response":{"output_text":"Cache review"}}`),
			})
		},
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.SetLogger(logger)

	title, err := runtime.requestSessionTitle(context.Background(), titleClient, provider.ModelRef{
		ProviderID: "github-copilot",
		ModelID:    "gpt-5.3-codex",
	}, "session-1", "turn-1", "review cache behavior")
	if err != nil {
		t.Fatalf("requestSessionTitle() error = %v", err)
	}
	if title != "Cache review" {
		t.Fatalf("title = %q", title)
	}
	if len(titleClient.requests) != 1 {
		t.Fatalf("title requests = %d, want 1", len(titleClient.requests))
	}

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	if !strings.Contains(debugLog, "session title raw sse frame") ||
		!strings.Contains(debugLog, "model=github-copilot/gpt-5.3-codex") ||
		!strings.Contains(debugLog, "sse_event=response.completed") ||
		!strings.Contains(debugLog, "output_text") {
		t.Fatalf("debug log missing GitHub Copilot title raw SSE frame: %q", debugLog)
	}
}

func TestSummarizeSessionStatePrefersGeneratedTitle(t *testing.T) {
	title, status := summarizeSessionState(events.SessionState{
		Title:     "terminal shell · split operator",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "open a new split",
			},
		},
	})

	if title != "terminal shell · split operator" {
		t.Fatalf("title = %q", title)
	}
	if status != events.TurnStatusCompleted {
		t.Fatalf("status = %q", status)
	}
}

func TestSummarizeSessionStateDoesNotUseTurnPromptAsSessionTitle(t *testing.T) {
	title, status := summarizeSessionState(events.SessionState{
		WorkspaceRoot: "/tmp/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "review cache middleware",
			},
		},
	})

	if title != "repo" {
		t.Fatalf("title = %q", title)
	}
	if status != events.TurnStatusCompleted {
		t.Fatalf("status = %q", status)
	}
}

func waitForSessionTitle(t *testing.T, runtime *Runtime, sessionID string) events.SessionState {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("Snapshot() error = %v", err)
		}
		if strings.TrimSpace(state.Title) != "" {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	t.Fatalf("session title was not generated: %#v", state)
	return events.SessionState{}
}

func TestSanitizeSessionTitleStripsMarkdownAndQuotes(t *testing.T) {
	if got := sanitizeSessionTitle(`**"Optimizing Your Project’s Performance"**`); got != "Optimizing Your Projects Performance" {
		t.Fatalf("sanitizeSessionTitle() = %q", got)
	}
}

type recordingStreamProvider struct {
	stream    provider.Stream
	onRequest func(provider.Request)
	requests  []provider.Request
}

func (p *recordingStreamProvider) Stream(_ context.Context, req provider.Request) (provider.Stream, error) {
	p.requests = append(p.requests, req)
	if p.onRequest != nil {
		p.onRequest(req)
	}
	return p.stream, nil
}

type cancelAwareTitleProvider struct {
	started  chan struct{}
	canceled chan struct{}
}

func (p *cancelAwareTitleProvider) Stream(ctx context.Context, _ provider.Request) (provider.Stream, error) {
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	<-ctx.Done()
	select {
	case <-p.canceled:
	default:
		close(p.canceled)
	}
	return nil, ctx.Err()
}

type gatedEventStream struct {
	allow  <-chan struct{}
	events []provider.Event
	index  int
}

func (s *gatedEventStream) Recv() (provider.Event, error) {
	if s.index >= len(s.events) {
		return provider.Event{}, io.EOF
	}
	if s.index == 0 && s.allow != nil {
		<-s.allow
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *gatedEventStream) Close() error {
	return nil
}
