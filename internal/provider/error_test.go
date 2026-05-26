package provider

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfterHeaderSupportsFractionalSeconds(t *testing.T) {
	header := http.Header{
		"Retry-After": []string{"0.1"},
	}
	if got := parseRetryAfterHeader(header); got != 100*time.Millisecond {
		t.Fatalf("parseRetryAfterHeader() = %s, want 100ms", got)
	}
}

func TestRetryHintForErrorTreatsHarmonyParserHeaderFailuresAsRetryable(t *testing.T) {
	err := errors.New(`unexpected tokens remaining in message header: Some("...<|end|><|start|>assistant<|channel|>commentary")`)

	if hint := RetryHintForError(err); !hint.Retryable || hint.RetryAfter != 0 {
		t.Fatalf("retry hint = %#v, want retryable with zero delay", hint)
	}
}

func TestRetryHintForErrorTreatsHTTP2StreamInternalErrorAsRetryable(t *testing.T) {
	err := errors.New("stream error: stream ID 19; INTERNAL_ERROR; received from peer")

	if hint := RetryHintForError(err); !hint.Retryable || hint.RetryAfter != 0 {
		t.Fatalf("retry hint = %#v, want retryable with zero delay", hint)
	}
}

func TestProviderErrorClassifiesInputLimitFailuresProviderAgnostically(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
	}{
		{
			name:       "request entity too large",
			statusCode: http.StatusRequestEntityTooLarge,
			message:    "request body too large",
		},
		{
			name:    "prompt token count exceeds limit",
			message: "prompt token count of 130431 exceeds the limit of 128000",
		},
		{
			name:    "maximum context length",
			message: "maximum context length is 128000 tokens",
		},
		{
			name:    "input token count too large",
			message: "input token count is too large for this model",
		},
		{
			name:    "too many tokens",
			message: "too many tokens in request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newProviderError(tt.message, tt.statusCode, false, 0, nil)
			if !errors.Is(err, ErrProviderInputLimitExceeded) {
				t.Fatalf("errors.Is(..., ErrProviderInputLimitExceeded) = false for %v", err)
			}
			if !IsInputLimitExceeded(err) {
				t.Fatalf("IsInputLimitExceeded(%v) = false", err)
			}
		})
	}
}

func TestProviderErrorDoesNotClassifyUnrelatedProviderFailuresAsInputLimit(t *testing.T) {
	err := newProviderError("provider unavailable after rate limit", http.StatusTooManyRequests, true, time.Second, nil)
	if errors.Is(err, ErrProviderInputLimitExceeded) {
		t.Fatalf("errors.Is(..., ErrProviderInputLimitExceeded) = true for %v", err)
	}
	if IsInputLimitExceeded(err) {
		t.Fatalf("IsInputLimitExceeded(%v) = true", err)
	}
}
