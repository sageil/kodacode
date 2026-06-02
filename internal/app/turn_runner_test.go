package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/sageil/kodacode/internal/engine"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func newTurnRunnerTestDeps(t *testing.T) (*SessionService, *ToolExecutor, *engine.Engine, prompt.Shaper) {
	t.Helper()
	return newTurnRunnerTestDepsWithStore(t, events.NewMemoryStore())
}

func newTurnRunnerTestDepsWithStore(t *testing.T, store events.ReplayStore) (*SessionService, *ToolExecutor, *engine.Engine, prompt.Shaper) {
	t.Helper()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	tools, err := NewToolExecutor(
		sessions,
		tool.DefaultRuntimeTools()...,
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if sqliteStore, ok := store.(*events.SQLiteStore); ok {
		tools.SetBackgroundLogStore(NewSQLiteBackgroundExecutionLogStore(sqliteStore))
	} else {
		tools.SetBackgroundLogStore(newTestSQLiteBackgroundLogStore(t))
	}
	eng, err := engine.New(engine.Dependencies{Compiler: prompt.NewStaticCompiler()})
	if err != nil {
		t.Fatalf("engine.New() error = %v", err)
	}
	shaper := prompt.NewShaper()
	return sessions, tools, eng, shaper
}

func baseFragments() []prompt.Fragment {
	return []prompt.Fragment{
		{Kind: prompt.KindPolicy, Source: prompt.SourceBuiltin, Stability: prompt.StabilityStable, Content: "base"},
		{Kind: prompt.KindRole, Source: prompt.SourceProject, Stability: prompt.StabilityStable, Content: "builder"},
	}
}

func baseModelRoute() provider.ModelRoute {
	return provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}
}

func TestTurnRunnerExposesApplyPatchAsFunctionForModelsWithoutCustomToolSupport(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "ok"},
		})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		AgentID:    "builder",
		UserText:   "hello",
		Fragments:  baseFragments(),
		ModelRoute: provider.ModelRoute{Primary: provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(client.requests))
	}
	var sawApplyPatch bool
	for _, tl := range client.requests[0].Tools {
		if tl.KindOrDefault() == provider.ToolKindCustom {
			t.Fatalf("request tools include custom tool for gpt-4.1: %#v", tl)
		}
		if tl.Name == tool.ApplyPatchToolName {
			sawApplyPatch = true
			if tl.KindOrDefault() != provider.ToolKindFunction {
				t.Fatalf("apply_patch kind = %q, want function", tl.KindOrDefault())
			}
		}
	}
	if !sawApplyPatch {
		t.Fatalf("request tools missing apply_patch for gpt-4.1: %#v", client.requests[0].Tools)
	}
}

type fakeProvider struct {
	mu            sync.Mutex
	streams       []provider.Stream
	requests      []provider.Request
	countRequests []provider.Request
	counts        []int
	countSources  []provider.TokenCountSource
	countErrs     []error
	errs          []error
	err           error
}

func (f *fakeProvider) Stream(_ context.Context, req provider.Request) (provider.Stream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return nil, err
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	if len(f.streams) == 0 {
		return nil, errors.New("no provider stream configured")
	}
	stream := f.streams[0]
	f.streams = f.streams[1:]
	return stream, nil
}

func (f *fakeProvider) CountTokens(_ context.Context, req provider.Request) (int, provider.TokenCountSource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.countRequests = append(f.countRequests, req)
	if len(f.countErrs) > 0 {
		err := f.countErrs[0]
		f.countErrs = f.countErrs[1:]
		if err != nil {
			return 0, "", err
		}
	}
	if len(f.counts) == 0 {
		return provider.EstimateRequestTokens(req), provider.TokenCountSourceEstimated, nil
	}
	count := f.counts[0]
	f.counts = f.counts[1:]
	source := provider.TokenCountSourceEstimated
	if len(f.countSources) > 0 {
		source = f.countSources[0]
		f.countSources = f.countSources[1:]
	}
	if source == "" {
		source = provider.TokenCountSourceEstimated
	}
	return count, source, nil
}
