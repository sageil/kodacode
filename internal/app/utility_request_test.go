package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

type utilityRetryBlockingStream struct {
	ctx context.Context
}

func (s *utilityRetryBlockingStream) Recv() (provider.Event, error) {
	<-s.ctx.Done()
	return provider.Event{}, s.ctx.Err()
}

func (s *utilityRetryBlockingStream) Close() error {
	return nil
}

type utilityRetryStreamProvider struct {
	attempts int
}

func (p *utilityRetryStreamProvider) Stream(ctx context.Context, _ provider.Request) (provider.Stream, error) {
	p.attempts++
	if p.attempts == 1 {
		return &utilityRetryBlockingStream{ctx: ctx}, nil
	}
	return provider.NewSliceStream([]provider.Event{
		{Kind: provider.EventKindAssistantDelta, AssistantDelta: "retried summary"},
	}), nil
}

func (p *utilityRetryStreamProvider) CountTokens(context.Context, provider.Request) (int, provider.TokenCountSource, error) {
	return 0, "", errors.New("not implemented")
}

type utilityRetryTimeoutProvider struct {
	attempts int
}

func (p *utilityRetryTimeoutProvider) Stream(ctx context.Context, _ provider.Request) (provider.Stream, error) {
	p.attempts++
	return &utilityRetryBlockingStream{ctx: ctx}, nil
}

func (p *utilityRetryTimeoutProvider) CountTokens(context.Context, provider.Request) (int, provider.TokenCountSource, error) {
	return 0, "", errors.New("not implemented")
}

func TestRequestUtilityTextRetriesAfterAttemptTimeout(t *testing.T) {
	client := &utilityRetryStreamProvider{}
	text, err := requestUtilityText(context.Background(), client, provider.Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "session-compaction",
		Model:           provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		MaxOutputTokens: 128,
		Instructions:    "summarize",
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "continue",
		}},
	}, 20*time.Millisecond, defaultUtilityRetryPolicy())
	if err != nil {
		t.Fatalf("requestUtilityText() error = %v", err)
	}
	if text != "retried summary" {
		t.Fatalf("text = %q", text)
	}
	if client.attempts != 2 {
		t.Fatalf("attempts = %d, want 2", client.attempts)
	}
}

func TestRequestUtilityTextRetriesAfterEmptyResponse(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream(nil),
			provider.NewSliceStream([]provider.Event{{
				Kind:           provider.EventKindAssistantDelta,
				AssistantDelta: "retried summary",
			}}),
		},
	}
	text, err := requestUtilityText(context.Background(), client, provider.Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "session-compaction",
		Model:           provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		MaxOutputTokens: 128,
		Instructions:    "summarize",
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "continue",
		}},
	}, 0, defaultUtilityRetryPolicy())
	if err != nil {
		t.Fatalf("requestUtilityText() error = %v", err)
	}
	if text != "retried summary" {
		t.Fatalf("text = %q", text)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
}

func TestRequestUtilityTextDisablesUtilityReasoning(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{{
			Kind:           provider.EventKindAssistantDelta,
			AssistantDelta: "summary",
		}})},
	}
	_, err := requestUtilityText(context.Background(), client, provider.Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "session-compaction",
		Model:           provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-reasoner"},
		MaxOutputTokens: 128,
		ThinkingEnabled: true,
		ThinkingMode:    provider.ReasoningVariantXHigh,
		Instructions:    "summarize",
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "continue",
		}},
	}, 0, defaultUtilityRetryPolicy())
	if err != nil {
		t.Fatalf("requestUtilityText() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(client.requests))
	}
	request := client.requests[0]
	if !request.ThinkingSupported {
		t.Fatal("ThinkingSupported = false, want true so providers can receive explicit disablement")
	}
	if request.ThinkingEnabled {
		t.Fatal("ThinkingEnabled = true, want false for utility requests")
	}
	if request.ThinkingMode != "" {
		t.Fatalf("ThinkingMode = %q, want empty for deepseek utility requests", request.ThinkingMode)
	}
}

func TestRequestUtilityTextUsesNoneReasoningVariantWhenSupported(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{{
			Kind:           provider.EventKindAssistantDelta,
			AssistantDelta: "summary",
		}})},
	}
	_, err := requestUtilityText(context.Background(), client, provider.Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "session-compaction",
		Model:           provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
		MaxOutputTokens: 128,
		ThinkingEnabled: true,
		ThinkingMode:    provider.ReasoningVariantHigh,
		Instructions:    "summarize",
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "continue",
		}},
	}, 0, defaultUtilityRetryPolicy())
	if err != nil {
		t.Fatalf("requestUtilityText() error = %v", err)
	}
	request := client.requests[0]
	if request.ThinkingEnabled {
		t.Fatal("ThinkingEnabled = true, want false for utility requests")
	}
	if request.ThinkingMode != provider.ReasoningVariantNone {
		t.Fatalf("ThinkingMode = %q, want none", request.ThinkingMode)
	}
}

func TestRequestUtilityTextUsesMinimalReasoningVariantWhenNoneUnsupported(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{{
			Kind:           provider.EventKindAssistantDelta,
			AssistantDelta: "summary",
		}})},
	}
	_, err := requestUtilityText(context.Background(), client, provider.Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "session-title",
		Model:           provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5-mini"},
		MaxOutputTokens: 128,
		ThinkingEnabled: true,
		ThinkingMode:    provider.ReasoningVariantHigh,
		Instructions:    "title",
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "continue",
		}},
	}, 0, defaultUtilityRetryPolicy())
	if err != nil {
		t.Fatalf("requestUtilityText() error = %v", err)
	}
	request := client.requests[0]
	if request.ThinkingEnabled {
		t.Fatal("ThinkingEnabled = true, want false for utility requests")
	}
	if request.ThinkingMode != provider.ReasoningVariantMinimal {
		t.Fatalf("ThinkingMode = %q, want minimal", request.ThinkingMode)
	}
}

func TestRequestUtilityTextStopsAfterRetryTimeout(t *testing.T) {
	client := &utilityRetryTimeoutProvider{}
	text, err := requestUtilityText(context.Background(), client, provider.Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "session-compaction",
		Model:           provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		MaxOutputTokens: 128,
		Instructions:    "summarize",
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "continue",
		}},
	}, 20*time.Millisecond, defaultUtilityRetryPolicy())
	if err == nil || !errors.Is(err, errUtilityModelTimedOut) {
		t.Fatalf("error = %v, want utility model timeout", err)
	}
	if err.Error() != "connection to utility model timed out" {
		t.Fatalf("error text = %q", err.Error())
	}
	if text != "" {
		t.Fatalf("text = %q, want empty", text)
	}
	if client.attempts != 2 {
		t.Fatalf("attempts = %d, want 1 retry path to stop after total 2 attempts", client.attempts)
	}
}

func TestUtilityRetryDecisionUsesProviderRetryHint(t *testing.T) {
	delay, retryable := utilityRetryDecision(context.Background(), &provider.ProviderError{
		Message:    "rate limited",
		Retryable:  true,
		RetryAfter: 150 * time.Millisecond,
	}, 1, 0, defaultUtilityRetryPolicy())
	if !retryable {
		t.Fatal("retryable = false, want true")
	}
	if delay != 150*time.Millisecond {
		t.Fatalf("delay = %s, want 150ms", delay)
	}
}

func TestUtilityRetryDecisionDoesNotRetryLongProviderBackoff(t *testing.T) {
	delay, retryable := utilityRetryDecision(context.Background(), &provider.ProviderError{
		Message:    "rate limited",
		Retryable:  true,
		RetryAfter: 72 * time.Hour,
	}, 1, 0, defaultUtilityRetryPolicy())
	if retryable {
		t.Fatalf("retryable = true delay=%s, want false", delay)
	}
}

func TestRequestUtilityTextDoesNotRetryWhenConfiguredAttemptsZero(t *testing.T) {
	client := &utilityRetryTimeoutProvider{}
	text, err := requestUtilityText(context.Background(), client, provider.Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "session-compaction",
		Model:           provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		MaxOutputTokens: 128,
		Instructions:    "summarize",
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "continue",
		}},
	}, 20*time.Millisecond, utilityRetryPolicy{
		Attempts:           0,
		RetryAfterMaxDelay: defaultUtilityRetryAfterMaxDelay,
	})
	if err == nil || !errors.Is(err, errUtilityModelTimedOut) {
		t.Fatalf("error = %v, want utility model timeout", err)
	}
	if err.Error() != "connection to utility model timed out" {
		t.Fatalf("error text = %q", err.Error())
	}
	if text != "" {
		t.Fatalf("text = %q, want empty", text)
	}
	if client.attempts != 1 {
		t.Fatalf("attempts = %d, want 1 with retries disabled", client.attempts)
	}
}

func TestUtilityRetryDecisionUsesConfiguredRetryAfterMaxDelay(t *testing.T) {
	delay, retryable := utilityRetryDecision(context.Background(), &provider.ProviderError{
		Message:    "rate limited",
		Retryable:  true,
		RetryAfter: 150 * time.Millisecond,
	}, 1, 0, utilityRetryPolicy{
		Attempts:           1,
		RetryAfterMaxDelay: 100 * time.Millisecond,
	})
	if retryable {
		t.Fatalf("retryable = true delay=%s, want false", delay)
	}
}

func TestUtilityRetryDecisionDoesNotRetryParentContextDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	delay, retryable := utilityRetryDecision(ctx, context.DeadlineExceeded, 1, 20*time.Millisecond, defaultUtilityRetryPolicy())
	if retryable {
		t.Fatalf("retryable = true delay=%s, want false", delay)
	}
}
