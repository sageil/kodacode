package provider

import (
	"context"
	"strings"
	"testing"
)

func TestPromptingClientPrependsPromptForConcreteModel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	downstream := &stubClient{stream: NewSliceStream([]Event{{Kind: EventKindAssistantDelta, AssistantDelta: "ok"}})}
	client := NewPromptingClient(downstream)

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openrouter", ModelID: "deepseek/deepseek-chat"},
		Instructions: "Base instructions.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}

	if len(downstream.requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(downstream.requests))
	}
	got := downstream.requests[0].Instructions
	if !strings.Contains(got, "reasoning_content") {
		t.Fatalf("instructions = %q, want deepseek prompt", got)
	}
	if !strings.HasSuffix(got, "Base instructions.") {
		t.Fatalf("instructions = %q, want base instructions suffix", got)
	}
}

func TestPromptingClientAppliesPromptThroughRoutedClient(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	primary := &stubClient{stream: NewSliceStream([]Event{{Kind: EventKindAssistantDelta, AssistantDelta: "ok"}})}
	router, err := NewRoutedClient(map[string]Client{
		"github-copilot": NewPromptingClient(primary),
	})
	if err != nil {
		t.Fatalf("NewRoutedClient() error = %v", err)
	}

	stream, err := router.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "claude-sonnet-4"},
		Instructions: "Base instructions.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}

	if got := primary.requests[0].Instructions; !strings.Contains(got, "<anthropic_guidance>") {
		t.Fatalf("instructions = %q, want anthropic prompt", got)
	}
}

func TestPromptingClientPrependsPromptIntoCacheablePrefix(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	downstream := &stubClient{stream: NewSliceStream([]Event{{Kind: EventKindAssistantDelta, AssistantDelta: "ok"}})}
	client := NewPromptingClient(downstream)

	stream, err := client.Stream(context.Background(), Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions:    "Stable section.\n\nDynamic section.",
		CacheablePrefix: "Stable section.",
		DynamicSuffix:   "Dynamic section.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}

	if len(downstream.requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(downstream.requests))
	}
	got := downstream.requests[0]
	if !strings.Contains(got.CacheablePrefix, "Keep reasoning private.") {
		t.Fatalf("cacheable prefix = %q, want openai prompt", got.CacheablePrefix)
	}
	if got.DynamicSuffix != "Dynamic section." {
		t.Fatalf("dynamic suffix = %q, want preserved dynamic section", got.DynamicSuffix)
	}
	if got.Instructions != strings.TrimSpace(got.CacheablePrefix)+"\n\nDynamic section." {
		t.Fatalf("instructions = %q", got.Instructions)
	}
}
