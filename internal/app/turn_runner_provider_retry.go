package app

import (
	"context"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

type providerRetryDecision struct {
	Retryable     bool
	Delay         time.Duration
	SkippedReason string
}

type providerRetryInput struct {
	Err              error
	Attempt          int
	RequestStarted   bool
	DurableProgress  bool
	CompletionTokens int
	ToolCallStarted  bool
	ExecutedTools    int
	ReusedTools      int
}

const (
	providerRetrySkippedDurableProgress   = "durable_progress"
	providerRetrySkippedCompletionTokens  = "completion_tokens"
	providerRetrySkippedToolCallStarted   = "tool_call_started"
	providerRetrySkippedToolProgress      = "tool_progress"
	providerRetrySkippedAttemptsExhausted = "attempts_exhausted"
)

func (r *TurnRunner) providerRetryDecision(input providerRetryInput) providerRetryDecision {
	if input.Err == nil {
		return providerRetryDecision{}
	}
	hint := provider.RetryHintForError(input.Err)
	if !hint.Retryable {
		return providerRetryDecision{}
	}
	if input.RequestStarted {
		if input.DurableProgress {
			return providerRetryDecision{SkippedReason: providerRetrySkippedDurableProgress}
		}
		if input.CompletionTokens > 0 {
			return providerRetryDecision{SkippedReason: providerRetrySkippedCompletionTokens}
		}
		if input.ToolCallStarted {
			return providerRetryDecision{SkippedReason: providerRetrySkippedToolCallStarted}
		}
		if input.ExecutedTools > 0 || input.ReusedTools > 0 {
			return providerRetryDecision{SkippedReason: providerRetrySkippedToolProgress}
		}
	}
	if input.Attempt > r.maxProviderRetryAttempts() {
		return providerRetryDecision{SkippedReason: providerRetrySkippedAttemptsExhausted}
	}
	delay := hint.RetryAfter
	if delay <= 0 {
		delay = defaultProviderRetryDelay(input.Attempt)
	}
	return providerRetryDecision{
		Retryable: true,
		Delay:     delay,
	}
}

func (r *TurnRunner) maxProviderRetryAttempts() int {
	if r == nil || r.retries <= 0 {
		return 0
	}
	return r.retries
}

func defaultProviderRetryDelay(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return time.Second
	case attempt == 2:
		return 2 * time.Second
	default:
		return 4 * time.Second
	}
}

func (r *TurnRunner) waitForProviderRetry(ctx context.Context, delay time.Duration) error {
	if r == nil || r.wait == nil {
		return waitWithContext(ctx, delay)
	}
	return r.wait(ctx, delay)
}

func (r *TurnRunner) providerRetryAt(delay time.Duration) time.Time {
	if delay < 0 {
		delay = 0
	}
	now := time.Now
	if r != nil && r.now != nil {
		now = r.now
	}
	return now().Add(delay)
}
