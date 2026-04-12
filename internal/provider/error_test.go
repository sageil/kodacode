package provider

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ClassifyError ---

func TestClassifyError_ContextOverflow_ByBody(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		status   int
		body     string
	}{
		{"anthropic", "anthropic", 400, `{"error":{"message":"prompt is too long for this model"}}`},
		{"bedrock", "bedrock", 400, `{"message":"input is too long for requested model"}`},
		{"openai", "openai", 400, `{"error":{"message":"This model's maximum context length is 128000 tokens. However, your messages resulted in 200000 tokens. Please reduce the length of the messages. This exceeds the context window."}}`},
		{"google", "google", 400, `{"error":{"message":"input token count of 500000 exceeds the maximum of 128000"}}`},
		{"xai", "xai", 400, `{"error":"maximum prompt length is 131072"}`},
		{"groq", "groq", 400, `{"error":"Please reduce the length of the messages"}`},
		{"openrouter", "openrouter", 400, `{"error":"maximum context length is 128000 tokens"}`},
		{"copilot", "copilot", 400, `{"message":"exceeds the limit of 128000"}`},
		{"llamacpp", "llamacpp", 400, `{"error":"exceeds the available context size"}`},
		{"lmstudio", "lmstudio", 400, `{"error":"greater than the context length"}`},
		{"minimax", "minimax", 400, `{"error":"context window exceeds limit"}`},
		{"kimi", "kimi", 400, `{"error":"exceeded model token limit"}`},
		{"generic", "any", 400, `{"error":"context_length_exceeded"}`},
		{"generic_space", "any", 400, `{"error":"context length exceeded"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.provider, tt.status, tt.body, nil)
			assert.Equal(t, "context_overflow", result.Type, "provider: %s", tt.name)
			assert.False(t, result.IsRetryable)
		})
	}
}

func TestClassifyError_ContextOverflow_Status413(t *testing.T) {
	result := ClassifyError("any", 413, "", nil)
	assert.Equal(t, "context_overflow", result.Type)
	assert.False(t, result.IsRetryable)
}

func TestClassifyError_Retryable429(t *testing.T) {
	result := ClassifyError("openai", 429, `{"error":{"message":"rate limited"}}`, nil)
	assert.Equal(t, "api_error", result.Type)
	assert.True(t, result.IsRetryable)
	assert.Equal(t, "rate limited", result.Message)
	assert.Equal(t, 429, result.StatusCode)
}

func TestClassifyError_RetryableServerErrors(t *testing.T) {
	for _, status := range []int{500, 502, 503, 529} {
		result := ClassifyError("any", status, `{"message":"server error"}`, nil)
		assert.True(t, result.IsRetryable, "status: %d", status)
		assert.Equal(t, "api_error", result.Type, "status: %d", status)
	}
}

func TestClassifyError_OpenAI404Retryable(t *testing.T) {
	result := ClassifyError("openai", 404, `{"error":{"message":"not found"}}`, nil)
	assert.True(t, result.IsRetryable)
	assert.Equal(t, "api_error", result.Type)
}

func TestClassifyError_NonOpenAI404NotRetryable(t *testing.T) {
	result := ClassifyError("anthropic", 404, `{"error":{"message":"not found"}}`, nil)
	assert.False(t, result.IsRetryable)
}

func TestClassifyError_UnknownErrorNotRetryable(t *testing.T) {
	result := ClassifyError("any", 400, `{"error":{"message":"bad request"}}`, nil)
	assert.Equal(t, "api_error", result.Type)
	assert.False(t, result.IsRetryable)
	assert.Equal(t, "bad request", result.Message)
}

func TestClassifyError_PreservesResponseInfo(t *testing.T) {
	headers := map[string]string{"x-request-id": "abc"}
	result := ClassifyError("openai", 429, `{"error":{"message":"rate limited"}}`, headers)
	assert.Equal(t, `{"error":{"message":"rate limited"}}`, result.ResponseBody)
	assert.Equal(t, "abc", result.ResponseHeaders["x-request-id"])
}

func TestClassifyError_CerebrasNoBody(t *testing.T) {
	result := ClassifyError("cerebras", 400, "400 (no body)", nil)
	assert.Equal(t, "context_overflow", result.Type)
}

func TestClassifyError_MistralNoBody(t *testing.T) {
	result := ClassifyError("mistral", 413, "413 status code (no body)", nil)
	assert.Equal(t, "context_overflow", result.Type)
}

// --- IsContextOverflow ---

func TestIsContextOverflow_Status413AlwaysTrue(t *testing.T) {
	assert.True(t, IsContextOverflow("any", 413, ""))
	assert.True(t, IsContextOverflow("any", 413, "random text"))
}

func TestIsContextOverflow_PatternMatching(t *testing.T) {
	assert.True(t, IsContextOverflow("anthropic", 400, "prompt is too long"))
	assert.True(t, IsContextOverflow("openai", 400, "exceeds the context window"))
	assert.False(t, IsContextOverflow("openai", 400, "invalid api key"))
}

// --- ParseRetryAfter ---

func TestParseRetryAfter_Milliseconds(t *testing.T) {
	headers := map[string]string{"retry-after-ms": "5000"}
	assert.Equal(t, 5*time.Second, ParseRetryAfter(headers))
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	headers := map[string]string{"retry-after": "30"}
	assert.Equal(t, 30*time.Second, ParseRetryAfter(headers))
}

func TestParseRetryAfter_DateString(t *testing.T) {
	future := time.Now().Add(60 * time.Second).UTC().Format(time.RFC1123)
	headers := map[string]string{"retry-after": future}
	d := ParseRetryAfter(headers)
	// Should be roughly 60s (allow some tolerance)
	assert.True(t, d > 55*time.Second && d < 65*time.Second, "got %v", d)
}

func TestParseRetryAfter_NoHeader(t *testing.T) {
	assert.Equal(t, time.Duration(0), ParseRetryAfter(nil))
	assert.Equal(t, time.Duration(0), ParseRetryAfter(map[string]string{}))
}

func TestParseRetryAfter_InvalidValue(t *testing.T) {
	headers := map[string]string{"retry-after": "not-a-number"}
	assert.Equal(t, time.Duration(0), ParseRetryAfter(headers))
}

func TestParseRetryAfter_MillisecondsPriority(t *testing.T) {
	headers := map[string]string{
		"retry-after-ms": "1000",
		"retry-after":    "30",
	}
	// retry-after-ms should take priority
	assert.Equal(t, 1*time.Second, ParseRetryAfter(headers))
}

// --- RetryDelay ---

func TestRetryDelay_ExponentialBackoff(t *testing.T) {
	empty := map[string]string{}
	assert.Equal(t, 2*time.Second, RetryDelay(1, empty))
	assert.Equal(t, 4*time.Second, RetryDelay(2, empty))
	assert.Equal(t, 8*time.Second, RetryDelay(3, empty))
	assert.Equal(t, 16*time.Second, RetryDelay(4, empty))
	assert.Equal(t, 30*time.Second, RetryDelay(5, empty))
	// Higher attempts should stay capped at 30s
	assert.Equal(t, 30*time.Second, RetryDelay(10, empty))
}

func TestRetryDelay_UsesRetryAfterHeader(t *testing.T) {
	headers := map[string]string{"retry-after": "45"}
	assert.Equal(t, 45*time.Second, RetryDelay(1, headers))
}

func TestRetryDelay_NilHeaders(t *testing.T) {
	assert.Equal(t, 2*time.Second, RetryDelay(1, nil))
}

// --- ExtractMessage ---

func TestExtractMessage_OpenAIFormat(t *testing.T) {
	body := `{"error":{"message":"invalid api key","type":"invalid_request_error"}}`
	assert.Equal(t, "invalid api key", ExtractMessage(body))
}

func TestExtractMessage_SimpleFormat(t *testing.T) {
	body := `{"error":"rate limited"}`
	assert.Equal(t, "rate limited", ExtractMessage(body))
}

func TestExtractMessage_MessageField(t *testing.T) {
	body := `{"message":"something went wrong"}`
	assert.Equal(t, "something went wrong", ExtractMessage(body))
}

func TestExtractMessage_InvalidJSON(t *testing.T) {
	body := "Internal Server Error"
	assert.Equal(t, "Internal Server Error", ExtractMessage(body))
}

func TestExtractMessage_TruncatesLongBody(t *testing.T) {
	body := strings.Repeat("x", 1000)
	result := ExtractMessage(body)
	assert.Len(t, result, 500)
}

func TestExtractMessage_EmptyBody(t *testing.T) {
	assert.Equal(t, "", ExtractMessage(""))
	assert.Equal(t, "", ExtractMessage("  "))
}

func TestExtractMessage_NestedErrorPriority(t *testing.T) {
	// When both {"error":{"message":...}} and {"message":...} exist,
	// the nested format should win (checked first)
	body := `{"error":{"message":"nested error"},"message":"top level"}`
	assert.Equal(t, "nested error", ExtractMessage(body))
}

// --- ParsedError struct ---

func TestParsedError_Fields(t *testing.T) {
	pe := ParsedError{
		Type:            "context_overflow",
		Message:         "too long",
		StatusCode:      413,
		IsRetryable:     false,
		ResponseBody:    "body",
		ResponseHeaders: map[string]string{"x-id": "123"},
	}

	assert.Equal(t, "context_overflow", pe.Type)
	assert.Equal(t, "too long", pe.Message)
	assert.Equal(t, 413, pe.StatusCode)
	assert.False(t, pe.IsRetryable)
	assert.Equal(t, "body", pe.ResponseBody)
	assert.Equal(t, "123", pe.ResponseHeaders["x-id"])
}

// --- Edge cases ---

func TestClassifyError_EmptyBody(t *testing.T) {
	result := ClassifyError("openai", 500, "", nil)
	assert.Equal(t, "api_error", result.Type)
	assert.True(t, result.IsRetryable)
	assert.Equal(t, "", result.Message)
}

func TestIsContextOverflow_CaseInsensitive(t *testing.T) {
	assert.True(t, IsContextOverflow("anthropic", 400, "PROMPT IS TOO LONG"))
	assert.True(t, IsContextOverflow("openai", 400, "Exceeds The Context Window"))
}

func TestRetryDelay_ZeroAttempt(t *testing.T) {
	// Edge case: attempt 0 should use base delay (2s * 2^(-1) = 1s, but
	// our implementation doesn't enter the loop, so returns 2s)
	d := RetryDelay(0, nil)
	require.Equal(t, 2*time.Second, d)
}
