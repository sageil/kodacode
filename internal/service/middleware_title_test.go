package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/service"
)

type titleFakeProvider struct {
	id     string
	name   string
	models []provider.Model
	chunks []provider.StreamChunk
	chatFn func(ctx context.Context, model string) (<-chan provider.StreamChunk, error)
	calls  int
	mu     sync.Mutex
}

func (f *titleFakeProvider) ID() string {
	if f.id != "" {
		return f.id
	}
	return "titlefake"
}
func (f *titleFakeProvider) Name() string {
	if f.name != "" {
		return f.name
	}
	return "TitleFake"
}
func (f *titleFakeProvider) Models(_ context.Context) ([]provider.Model, error) {
	if len(f.models) > 0 {
		out := make([]provider.Model, len(f.models))
		copy(out, f.models)
		return out, nil
	}
	return []provider.Model{{ID: "fake-model", ContextSize: 8192}}, nil
}

func (f *titleFakeProvider) StaticModels() []provider.Model {
	models, _ := f.Models(context.Background())
	return models
}

func (f *titleFakeProvider) Chat(ctx context.Context, model string, _ []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.chatFn != nil {
		return f.chatFn(ctx, model)
	}
	ch := make(chan provider.StreamChunk, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}
func (f *titleFakeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newTitleTestRegistry(t *testing.T, fps ...*titleFakeProvider) *provider.Registry {
	t.Helper()
	reg := provider.NewRegistry()
	for _, fp := range fps {
		if err := reg.Register(fp); err != nil {
			t.Fatal(err)
		}
	}
	return reg
}

func TestTitleMiddleware_FiresOnFirstTurn(t *testing.T) {
	fp := &titleFakeProvider{chunks: []provider.StreamChunk{
		{Delta: "Build KodaCode"},
		{FinishReason: "stop"},
	}}
	var mu sync.Mutex
	var titleGenerated string
	updateTitle := func(_ context.Context, _, title string) {
		mu.Lock()
		titleGenerated = title
		mu.Unlock()
	}
	reg := newTitleTestRegistry(t, fp)
	mw := service.NewTitleMiddleware(reg, &config.Config{}, updateTitle, nil, nil)
	req := &pipeline.TurnRequest{
		SessionID:  "s1",
		ProviderID: "titlefake",
		Model:      provider.Model{ID: "fake-model", ContextSize: 8192},
		Step:       0,
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{
				provider.TextPart{Text: "Build me an agent framework"},
			}},
		},
	}
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		r.Step = 1
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	got := titleGenerated
	mu.Unlock()
	if got == "" {
		t.Error("NewTitleMiddleware() title should be generated on first turn")
	}
}

func TestTitleMiddleware_FiresOnFirstTurnWithToolCalls(t *testing.T) {
	fp := &titleFakeProvider{chunks: []provider.StreamChunk{
		{Delta: "Build KodaCode"},
		{FinishReason: "stop"},
	}}
	var mu sync.Mutex
	var titleGenerated string
	reg := newTitleTestRegistry(t, fp)
	mw := service.NewTitleMiddleware(reg, &config.Config{}, func(_ context.Context, _, title string) {
		mu.Lock()
		titleGenerated = title
		mu.Unlock()
	}, nil, nil)
	req := &pipeline.TurnRequest{
		SessionID:  "s1",
		ProviderID: "titlefake",
		Model:      provider.Model{ID: "fake-model", ContextSize: 8192},
		Step:       0,
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{
				provider.TextPart{Text: "Read the file foo.go"},
			}},
		},
	}
	err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		r.Step = 3
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	got := titleGenerated
	mu.Unlock()
	if got == "" {
		t.Error("NewTitleMiddleware() title should be generated on first turn even with tool calls")
	}
}

func TestTitleMiddleware_SkipsSubsequentTurns(t *testing.T) {
	called := false
	fp := &titleFakeProvider{}
	reg := newTitleTestRegistry(t, fp)
	mw := service.NewTitleMiddleware(reg, &config.Config{}, func(_ context.Context, _, _ string) { called = true }, nil, nil)
	req := &pipeline.TurnRequest{
		SessionID:  "s1",
		ProviderID: "titlefake",
		Model:      provider.Model{ID: "fake-model", ContextSize: 8192},
		Step:       1,
	}
	_ = mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		r.Step = 2
		return nil
	})
	time.Sleep(50 * time.Millisecond)
	if called {
		t.Error("NewTitleMiddleware() should not generate title on subsequent turns")
	}
}

func TestTitleMiddleware_DetachesFromTurnContext(t *testing.T) {
	fp := &titleFakeProvider{chunks: []provider.StreamChunk{}}
	fp.chatFn = func(ctx context.Context, _ string) (<-chan provider.StreamChunk, error) {
		ch := make(chan provider.StreamChunk, 2)
		go func() {
			defer close(ch)
			select {
			case <-time.After(50 * time.Millisecond):
				ch <- provider.StreamChunk{Delta: "Detached Title"}
				ch <- provider.StreamChunk{FinishReason: "stop"}
			case <-ctx.Done():
			}
		}()
		return ch, nil
	}
	var mu sync.Mutex
	var titleGenerated string
	reg := newTitleTestRegistry(t, fp)
	mw := service.NewTitleMiddleware(reg, &config.Config{}, func(_ context.Context, _, title string) {
		mu.Lock()
		titleGenerated = title
		mu.Unlock()
	}, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	req := &pipeline.TurnRequest{
		SessionID:  "s1",
		ProviderID: "titlefake",
		Model:      provider.Model{ID: "fake-model", ContextSize: 8192},
		Step:       0,
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{
				provider.TextPart{Text: "Build something"},
			}},
		},
	}
	err := mw(ctx, req, func(_ context.Context, r *pipeline.TurnRequest) error {
		r.Step = 1
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := titleGenerated
		mu.Unlock()
		if got != "" {
			if got != "Detached Title" {
				t.Fatalf("title = %q, want %q", got, "Detached Title")
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("expected title generation to continue after turn context cancellation")
}

func TestTitleMiddleware_FallsBackToNextUtilityCandidate(t *testing.T) {
	bad := &titleFakeProvider{
		id: "cheap",
		models: []provider.Model{{
			ID:               "cheap-text",
			ContextSize:      32000,
			CostInput:        0.01,
			CostOutput:       0.01,
			OutputModalities: []string{"text"},
		}},
		chatFn: func(context.Context, string) (<-chan provider.StreamChunk, error) {
			return nil, errors.New("404 model not found")
		},
	}
	good := &titleFakeProvider{
		id: "good",
		models: []provider.Model{{
			ID:               "good-text",
			ContextSize:      32000,
			CostInput:        0.02,
			CostOutput:       0.02,
			OutputModalities: []string{"text"},
		}},
		chunks: []provider.StreamChunk{{Delta: "Recovered Title"}, {FinishReason: "stop"}},
	}
	primary := &titleFakeProvider{
		id: "primary",
		models: []provider.Model{{
			ID:               "primary-model",
			ContextSize:      32000,
			CostInput:        2,
			CostOutput:       4,
			OutputModalities: []string{"text"},
		}},
	}

	var mu sync.Mutex
	var titleGenerated string
	reg := newTitleTestRegistry(t, bad, good, primary)
	mw := service.NewTitleMiddleware(reg, &config.Config{}, func(_ context.Context, _, title string) {
		mu.Lock()
		titleGenerated = title
		mu.Unlock()
	}, nil, nil)

	req := &pipeline.TurnRequest{
		SessionID:  "s1",
		ProviderID: "primary",
		Model:      provider.Model{ID: "primary-model", ContextSize: 32000},
		Step:       0,
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{
				provider.TextPart{Text: "Summarize this thread"},
			}},
		},
	}

	if err := mw(context.Background(), req, func(_ context.Context, r *pipeline.TurnRequest) error {
		r.Step = 1
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := titleGenerated
		mu.Unlock()
		if got != "" {
			if got != "Recovered Title" {
				t.Fatalf("generated title = %q, want %q", got, "Recovered Title")
			}
			if bad.callCount() == 0 {
				t.Fatal("expected the cheapest failing utility candidate to be tried first")
			}
			if good.callCount() == 0 {
				t.Fatal("expected the next utility candidate to be used after failure")
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("expected title generation to fall back to a working utility candidate")
}
