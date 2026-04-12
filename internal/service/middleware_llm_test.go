package service_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/tool"
)

type fakeStreamProvider struct {
	chunks []provider.StreamChunk
}

func (f *fakeStreamProvider) ID() string   { return "fake" }
func (f *fakeStreamProvider) Name() string { return "Fake" }
func (f *fakeStreamProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model", ContextSize: 8192}}, nil
}
func (f *fakeStreamProvider) Chat(_ context.Context, _ string, _ []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

type failNStreamProvider struct {
	mu       sync.Mutex
	failures int
	chatErr  error
	chunks   []provider.StreamChunk
}

func (f *failNStreamProvider) ID() string   { return "failn" }
func (f *failNStreamProvider) Name() string { return "FailN" }
func (f *failNStreamProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model", ContextSize: 8192}}, nil
}
func (f *failNStreamProvider) Chat(_ context.Context, _ string, _ []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures > 0 {
		f.failures--
		return nil, f.chatErr
	}
	ch := make(chan provider.StreamChunk, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

type streamFailProvider struct {
	mu        sync.Mutex
	failures  int
	streamErr error
	chunks    []provider.StreamChunk
}

func (f *streamFailProvider) ID() string   { return "streamfail" }
func (f *streamFailProvider) Name() string { return "StreamFail" }
func (f *streamFailProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model", ContextSize: 8192}}, nil
}
func (f *streamFailProvider) Chat(_ context.Context, _ string, _ []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures > 0 {
		f.failures--
		ch := make(chan provider.StreamChunk, 1)
		ch <- provider.StreamChunk{Err: f.streamErr}
		close(ch)
		return ch, nil
	}
	ch := make(chan provider.StreamChunk, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func newTestRegistry(t *testing.T, p provider.Provider) *provider.Registry {
	t.Helper()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	return reg
}

func newTestReq(providerID string) *pipeline.TurnRequest {
	return &pipeline.TurnRequest{
		SessionID:   "s1",
		ProviderID:  providerID,
		Model:       provider.Model{ID: "fake-model", ContextSize: 8192},
		SystemParts: []string{"", ""},
	}
}

func testChainConfig(reg *provider.Registry, cfg *config.Config, publish func(string, service.SSEEvent)) service.ChainConfig {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if publish == nil {
		publish = func(string, service.SSEEvent) {}
	}
	return service.ChainConfig{
		Registry: reg,
		Config:   cfg,
		Publish:  publish,
	}
}

func TestLLMMiddleware_PublishesDeltaEvents(t *testing.T) {
	fp := &fakeStreamProvider{chunks: []provider.StreamChunk{
		{Delta: "hello "},
		{Delta: "world"},
		{FinishReason: "stop"},
	}}
	var events []service.SSEEvent
	publish := func(_ string, ev service.SSEEvent) { events = append(events, ev) }
	reg := newTestRegistry(t, fp)
	cc := testChainConfig(reg, nil, publish)
	mw := service.NewLLMMiddleware(&cc)
	req := newTestReq("fake")
	if err := mw(context.Background(), req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var deltas []string
	for _, ev := range events {
		if ev.Type == "delta" {
			if d, ok := ev.Data.(service.SSEDeltaData); ok {
				deltas = append(deltas, d.Content)
			}
		}
	}
	if len(deltas) != 2 {
		t.Errorf("delta events = %d, want 2", len(deltas))
	}
}

func TestLLMMiddleware_UnknownTool_SuppressesToolStart(t *testing.T) {
	fp := &fakeStreamProvider{chunks: []provider.StreamChunk{
		{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "tc1", Name: "ls"}},
		{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{}`}},
		{ToolCalls: []provider.ToolCall{{ID: "tc1", Name: "ls", Arguments: `{}`}}},
		{FinishReason: "tool_calls"},
		{FinishReason: "stop"},
	}}

	toolReg := tool.NewRegistry()
	toolReg.Register(&tool.Tool{
		Name: "bash",
		Execute: func(_ context.Context, _ tool.ExecutionContext, _ []byte) (*tool.Result, error) {
			return &tool.Result{Output: "ok"}, nil
		},
	})
	dir := t.TempDir()
	sndbx := sandbox.New(dir, sandbox.OriginTUI, toolReg, nil)

	var events []service.SSEEvent
	publish := func(_ string, ev service.SSEEvent) { events = append(events, ev) }

	reg := newTestRegistry(t, fp)
	cc := testChainConfig(reg, nil, publish)
	cc.Sandbox = sndbx
	mw := service.NewLLMMiddleware(&cc)
	req := newTestReq("fake")
	if err := mw(context.Background(), req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, ev := range events {
		if ev.Type == "tool_start" {
			if d, ok := ev.Data.(service.SSEToolStartData); ok {
				t.Errorf("tool_start emitted for unknown tool %q, want none", d.Tool)
			}
		}
	}
}

func TestLLMMiddleware_ChatError_RetriesAndSucceeds(t *testing.T) {
	fp := &failNStreamProvider{
		failures: 2,
		chatErr:  fmt.Errorf("POST https://api.example.com/v1/chat: 429 Too Many Requests"),
		chunks: []provider.StreamChunk{
			{Delta: "recovered"},
			{FinishReason: "stop"},
		},
	}

	var gotContent bool
	publish := func(_ string, ev service.SSEEvent) {
		if ev.Type == "delta" {
			if d, ok := ev.Data.(service.SSEDeltaData); ok && d.Content == "recovered" {
				gotContent = true
			}
		}
	}

	reg := newTestRegistry(t, fp)
	cc := testChainConfig(reg, nil, publish)
	mw := service.NewLLMMiddleware(&cc)
	req := newTestReq("failn")
	err := mw(context.Background(), req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if !gotContent {
		t.Error("expected 'recovered' content after retry")
	}
}

func TestLLMMiddleware_ChatError_ExhaustsRetries(t *testing.T) {
	fp := &failNStreamProvider{
		failures: 100,
		chatErr:  fmt.Errorf("502 Bad Gateway"),
	}

	reg := newTestRegistry(t, fp)
	cfg := &config.Config{Session: config.SessionConfig{MaxRetries: 1}}
	cc := testChainConfig(reg, cfg, nil)
	mw := service.NewLLMMiddleware(&cc)
	req := newTestReq("failn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := mw(ctx, req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

func TestLLMMiddleware_ChatError_NonRetryable_NoRetry(t *testing.T) {
	fp := &failNStreamProvider{
		failures: 1,
		chatErr:  fmt.Errorf("401 Unauthorized"),
	}

	reg := newTestRegistry(t, fp)
	cc := testChainConfig(reg, nil, nil)
	mw := service.NewLLMMiddleware(&cc)
	req := newTestReq("failn")
	err := mw(context.Background(), req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for non-retryable failure")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error containing '401', got: %v", err)
	}
}

func TestLLMMiddleware_StreamError_RetriesAndSucceeds(t *testing.T) {
	fp := &streamFailProvider{
		failures:  2,
		streamErr: fmt.Errorf("openai: stream interrupted: unexpected EOF"),
		chunks: []provider.StreamChunk{
			{Delta: "success after stream retry"},
			{FinishReason: "stop"},
		},
	}

	var gotContent bool
	publish := func(_ string, ev service.SSEEvent) {
		if ev.Type == "delta" {
			if d, ok := ev.Data.(service.SSEDeltaData); ok && strings.Contains(d.Content, "success") {
				gotContent = true
			}
		}
	}

	reg := newTestRegistry(t, fp)
	cc := testChainConfig(reg, nil, publish)
	mw := service.NewLLMMiddleware(&cc)
	req := newTestReq("streamfail")
	err := mw(context.Background(), req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after stream retries, got: %v", err)
	}
	if !gotContent {
		t.Error("expected success content after stream retries")
	}
}

func TestLLMMiddleware_StreamError_ExhaustsRetries(t *testing.T) {
	fp := &streamFailProvider{
		failures:  100,
		streamErr: fmt.Errorf("openai: stream: 502 Bad Gateway"),
	}

	reg := newTestRegistry(t, fp)
	cfg := &config.Config{Session: config.SessionConfig{MaxRetries: 1}}
	cc := testChainConfig(reg, cfg, nil)
	mw := service.NewLLMMiddleware(&cc)
	req := newTestReq("streamfail")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := mw(ctx, req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error after exhausting stream retries")
	}
}

func TestLLMMiddleware_StreamError_NonRetryable_NoRetry(t *testing.T) {
	fp := &streamFailProvider{
		failures:  1,
		streamErr: fmt.Errorf("context_length_exceeded: reduce input"),
	}

	reg := newTestRegistry(t, fp)
	cc := testChainConfig(reg, nil, nil)
	mw := service.NewLLMMiddleware(&cc)
	req := newTestReq("streamfail")
	err := mw(context.Background(), req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for non-retryable stream failure")
	}
	if strings.Contains(err.Error(), "all 5 retry attempts failed") {
		t.Errorf("non-retryable error should not show retry exhaustion message, got: %v", err)
	}
}

func TestLLMMiddleware_StreamError_PartialContent_Recovers(t *testing.T) {
	fp := &fakeStreamProvider{chunks: []provider.StreamChunk{
		{Delta: "partial "},
		{Delta: "content"},
		{Err: fmt.Errorf("openai: stream interrupted: connection reset")},
	}}

	var deltas []string
	var retryCount int
	publish := func(_ string, ev service.SSEEvent) {
		if ev.Type == "retry" {
			retryCount++
		}
		if ev.Type == "delta" {
			if d, ok := ev.Data.(service.SSEDeltaData); ok {
				deltas = append(deltas, d.Content)
			}
		}
	}

	reg := newTestRegistry(t, fp)
	cc := testChainConfig(reg, nil, publish)
	mw := service.NewLLMMiddleware(&cc)
	req := newTestReq("fake")
	err := mw(context.Background(), req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected recovery from stream error with partial content, got: %v", err)
	}
	if retryCount != 0 {
		t.Errorf("expected no retry events when recovering with content, got %d", retryCount)
	}
	if len(deltas) != 2 || deltas[0] != "partial " || deltas[1] != "content" {
		t.Errorf("expected [partial , content] deltas, got %v", deltas)
	}
}
