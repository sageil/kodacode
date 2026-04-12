package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

type invalidToolArgsProvider struct {
	calls int
}

func (p *invalidToolArgsProvider) ID() string   { return "fake-openai" }
func (p *invalidToolArgsProvider) Name() string { return "Fake OpenAI" }

func (p *invalidToolArgsProvider) Models(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model", ContextSize: 8192}}, nil
}

func (p *invalidToolArgsProvider) Chat(_ context.Context, _ string, messages []provider.Message, opts provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	p.calls++

	known := make(map[string]bool, len(opts.Tools))
	for _, t := range opts.Tools {
		known[t.Name] = true
	}
	for _, m := range messages {
		for _, part := range m.Parts {
			if tc, ok := part.(provider.ToolCallPart); ok && !known[tc.Name] {
				return nil, fmt.Errorf(`openai: stream: POST "http://localhost:11434/v1/chat/completions": 400 Bad Request {"message":"invalid tool call arguments"}`)
			}
		}
	}

	ch := make(chan provider.StreamChunk, 1)
	ch <- provider.StreamChunk{Delta: "ok", FinishReason: "stop"}
	close(ch)
	return ch, nil
}

func TestRetryChat_StripsUnknownHistoricalToolCallsOnInvalidToolArgs(t *testing.T) {
	req := &pipeline.TurnRequest{
		SessionID: "sess-1",
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "build it"}}},
			{Role: "assistant", Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "planner-1", Name: "subagent", Arguments: `{"agent_id":"planner"}`},
			}},
			{Role: "user", Parts: []provider.MessagePart{
				provider.ToolResultPart{ToolCallID: "planner-1", Output: "plan ready"},
			}},
		},
	}

	prov := &invalidToolArgsProvider{}
	var streamed bool
	err := retryChat(
		context.Background(),
		req,
		1,
		prov,
		"fake-model",
		[]provider.Tool{{Name: "bash"}},
		func(string, SSEEvent) {},
		func(ch <-chan provider.StreamChunk) {
			for range ch {
			}
			streamed = true
		},
	)
	if err != nil {
		t.Fatalf("retryChat() error = %v, want nil", err)
	}
	if !streamed {
		t.Fatal("retryChat() did not invoke onStream after sanitizing history")
	}
	if prov.calls != 2 {
		t.Fatalf("provider Chat() calls = %d, want 2", prov.calls)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("messages len after sanitize = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Fatalf("remaining message role = %q, want user", req.Messages[0].Role)
	}
}
