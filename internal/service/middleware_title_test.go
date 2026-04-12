package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/service"
)

type titleFakeProvider struct {
	chunks []provider.StreamChunk
	chatFn func(ctx context.Context) (<-chan provider.StreamChunk, error)
}

func (f *titleFakeProvider) ID() string   { return "titlefake" }
func (f *titleFakeProvider) Name() string { return "TitleFake" }
func (f *titleFakeProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model", ContextSize: 8192}}, nil
}
func (f *titleFakeProvider) Chat(ctx context.Context, _ string, _ []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	if f.chatFn != nil {
		return f.chatFn(ctx)
	}
	ch := make(chan provider.StreamChunk, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}
func newTitleTestRegistry(t *testing.T, fp *titleFakeProvider) *provider.Registry {
	t.Helper()
	reg := provider.NewRegistry()
	if err := reg.Register(fp); err != nil {
		t.Fatal(err)
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
	mw := service.NewTitleMiddleware(reg, &config.Config{}, updateTitle, nil)
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
	}, nil)
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
	mw := service.NewTitleMiddleware(reg, &config.Config{}, func(_ context.Context, _, _ string) { called = true }, nil)
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

func TestTitleMiddleware_CancelledContextStopsGeneration(t *testing.T) {
	blocked := make(chan struct{})
	fp := &titleFakeProvider{chunks: []provider.StreamChunk{}}
	fp.chatFn = func(ctx context.Context) (<-chan provider.StreamChunk, error) {
		// Block until context is cancelled.
		<-ctx.Done()
		close(blocked)
		return nil, ctx.Err()
	}
	var mu sync.Mutex
	var titleGenerated string
	reg := newTitleTestRegistry(t, fp)
	mw := service.NewTitleMiddleware(reg, &config.Config{}, func(_ context.Context, _, title string) {
		mu.Lock()
		titleGenerated = title
		mu.Unlock()
	}, nil)

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

	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("title generation did not stop after context cancellation")
	}

	mu.Lock()
	got := titleGenerated
	mu.Unlock()
	if got != "" {
		t.Errorf("title should not be generated after cancellation, got %q", got)
	}
}
