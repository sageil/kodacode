package app

import (
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

func TestProviderRetryDecisionRetriesBeforeRequestProgress(t *testing.T) {
	runner := &TurnRunner{retries: 2}
	err := &provider.ProviderError{Message: "temporary", Retryable: true, RetryAfter: 250 * time.Millisecond}

	decision := runner.providerRetryDecision(providerRetryInput{
		Err:            err,
		Attempt:        1,
		RequestStarted: true,
	})

	if !decision.Retryable || decision.Delay != 250*time.Millisecond || decision.SkippedReason != "" {
		t.Fatalf("decision = %#v, want retry with provider delay", decision)
	}
}

func TestProviderRetryDecisionSkipsAfterCompletionTokens(t *testing.T) {
	runner := &TurnRunner{retries: 2}
	err := &provider.ProviderError{Message: "stream interrupted", Retryable: true}

	decision := runner.providerRetryDecision(providerRetryInput{
		Err:              err,
		Attempt:          1,
		RequestStarted:   true,
		CompletionTokens: 3,
	})

	if decision.Retryable || decision.SkippedReason != providerRetrySkippedCompletionTokens {
		t.Fatalf("decision = %#v, want skipped for completion tokens", decision)
	}
}

func TestProviderRetryDecisionSkipsAfterDurableProgress(t *testing.T) {
	runner := &TurnRunner{retries: 2}
	err := &provider.ProviderError{Message: "stream interrupted", Retryable: true}

	decision := runner.providerRetryDecision(providerRetryInput{
		Err:             err,
		Attempt:         1,
		RequestStarted:  true,
		DurableProgress: true,
	})

	if decision.Retryable || decision.SkippedReason != providerRetrySkippedDurableProgress {
		t.Fatalf("decision = %#v, want skipped for durable progress", decision)
	}
}

func TestProviderRetryDecisionSkipsAfterToolCallStarted(t *testing.T) {
	runner := &TurnRunner{retries: 2}
	err := &provider.ProviderError{Message: "stream interrupted", Retryable: true}

	decision := runner.providerRetryDecision(providerRetryInput{
		Err:             err,
		Attempt:         1,
		RequestStarted:  true,
		ToolCallStarted: true,
	})

	if decision.Retryable || decision.SkippedReason != providerRetrySkippedToolCallStarted {
		t.Fatalf("decision = %#v, want skipped for started tool call", decision)
	}
}

func TestProviderRetryDecisionRecordsAttemptExhaustion(t *testing.T) {
	runner := &TurnRunner{retries: 1}
	err := &provider.ProviderError{Message: "temporary", Retryable: true}

	decision := runner.providerRetryDecision(providerRetryInput{
		Err:     err,
		Attempt: 2,
	})

	if decision.Retryable || decision.SkippedReason != providerRetrySkippedAttemptsExhausted {
		t.Fatalf("decision = %#v, want skipped for attempts exhausted", decision)
	}
}
