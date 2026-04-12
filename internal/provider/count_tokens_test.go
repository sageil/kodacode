package provider_test

import (
	"context"
	"os"
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestEstimateTokens(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "Hello world"},
		}},
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.TextPart{Text: "Hi there!"},
			provider.ToolCallPart{ID: "1", Name: "read", Arguments: `{"path":"/tmp/x"}`},
		}},
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{ToolCallID: "1", Output: "file contents here"},
		}},
	}
	opts := provider.ChatOptions{
		SystemParts: []string{"You are a helpful assistant.", ""},
		Tools: []provider.Tool{
			{Name: "read", Description: "Read a file", Parameters: []byte(`{"type":"object"}`)},
		},
	}

	got := provider.EstimateTokens(msgs, opts)
	if got <= 0 {
		t.Fatalf("EstimateTokens() = %d, want > 0", got)
	}

	// Verify adding content increases the estimate.
	msgs2 := append(msgs, provider.Message{
		Role: "user",
		Parts: []provider.MessagePart{
			provider.TextPart{Text: "Tell me more about this file please."},
		},
	})
	got2 := provider.EstimateTokens(msgs2, opts)
	if got2 <= got {
		t.Errorf("EstimateTokens() with more messages = %d, want > %d", got2, got)
	}

	// Empty input returns 0.
	if z := provider.EstimateTokens(nil, provider.ChatOptions{}); z != 0 {
		t.Errorf("EstimateTokens(nil, {}) = %d, want 0", z)
	}
}

func TestCountTokens_FallbackForNonTokenCounter(t *testing.T) {
	// fakeProvider (from registry_test.go) does NOT implement TokenCounter.
	fp := &fakeProvider{id: "test", name: "Test"}
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "Hello world test message"},
		}},
	}
	opts := provider.ChatOptions{
		SystemParts: []string{"System prompt here.", ""},
	}

	count, err := provider.CountTokens(context.Background(), fp, "model", msgs, opts)
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	expected := provider.EstimateTokens(msgs, opts)
	if count != expected {
		t.Errorf("CountTokens() = %d, want %d (EstimateTokens fallback)", count, expected)
	}
}

func TestTokenCounter_AnthropicImplements(t *testing.T) {
	p := provider.NewAnthropicProvider("test-key")
	if _, ok := provider.Provider(p).(provider.TokenCounter); !ok {
		t.Error("AnthropicProvider should implement TokenCounter")
	}
}

func TestTokenCounter_GoogleImplements(t *testing.T) {
	key := os.Getenv("GOOGLE_API_KEY")
	if key == "" {
		t.Skip("GOOGLE_API_KEY not set (need valid key to construct GoogleProvider)")
	}
	p, err := provider.NewGoogleProvider(context.Background(), key)
	if err != nil {
		t.Fatalf("NewGoogleProvider() error = %v", err)
	}
	if _, ok := provider.Provider(p).(provider.TokenCounter); !ok {
		t.Error("GoogleProvider should implement TokenCounter")
	}
}

func TestAnthropicProvider_CountTokens(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	p := provider.NewAnthropicProvider(key)
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "What is 2+2?"},
		}},
	}
	opts := provider.ChatOptions{
		SystemParts: []string{"You are a math tutor.", "", ""},
	}

	count, err := p.CountTokens(context.Background(), "claude-sonnet-4-20250514", msgs, opts)
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	if count <= 0 {
		t.Errorf("CountTokens() = %d, want > 0", count)
	}
	t.Logf("Anthropic CountTokens = %d", count)
}

func TestGoogleProvider_CountTokens(t *testing.T) {
	key := os.Getenv("GOOGLE_API_KEY")
	if key == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}

	p, err := provider.NewGoogleProvider(context.Background(), key)
	if err != nil {
		t.Fatalf("NewGoogleProvider() error = %v", err)
	}
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "What is 2+2?"},
		}},
	}
	opts := provider.ChatOptions{
		SystemParts: []string{"You are a math tutor.", "", ""},
	}

	count, err := p.CountTokens(context.Background(), "gemini-2.5-flash", msgs, opts)
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	if count <= 0 {
		t.Errorf("CountTokens() = %d, want > 0", count)
	}
	t.Logf("Google CountTokens = %d", count)
}
