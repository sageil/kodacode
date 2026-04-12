package provider

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParsedError represents a classified LLM error.
type ParsedError struct {
	Type            string
	Message         string
	StatusCode      int
	IsRetryable     bool
	ResponseBody    string
	ResponseHeaders map[string]string
}

// ClassifyError classifies an LLM API error based on provider, status code,
// and response body.
func ClassifyError(providerID string, statusCode int, responseBody string, responseHeaders map[string]string) ParsedError {
	msg := ExtractMessage(responseBody)

	if IsContextOverflow(providerID, statusCode, responseBody) {
		return ParsedError{
			Type:            "context_overflow",
			Message:         msg,
			StatusCode:      statusCode,
			IsRetryable:     false,
			ResponseBody:    responseBody,
			ResponseHeaders: responseHeaders,
		}
	}

	switch statusCode {
	case 413:
		return ParsedError{
			Type:            "context_overflow",
			Message:         msg,
			StatusCode:      statusCode,
			IsRetryable:     false,
			ResponseBody:    responseBody,
			ResponseHeaders: responseHeaders,
		}
	case 429:
		return ParsedError{
			Type:            "api_error",
			Message:         msg,
			StatusCode:      statusCode,
			IsRetryable:     true,
			ResponseBody:    responseBody,
			ResponseHeaders: responseHeaders,
		}
	case 500, 502, 503, 529:
		return ParsedError{
			Type:            "api_error",
			Message:         msg,
			StatusCode:      statusCode,
			IsRetryable:     true,
			ResponseBody:    responseBody,
			ResponseHeaders: responseHeaders,
		}
	case 404:
		if providerID == "openai" {
			return ParsedError{
				Type:            "api_error",
				Message:         msg,
				StatusCode:      statusCode,
				IsRetryable:     true,
				ResponseBody:    responseBody,
				ResponseHeaders: responseHeaders,
			}
		}
	}

	return ParsedError{
		Type:            "api_error",
		Message:         msg,
		StatusCode:      statusCode,
		IsRetryable:     false,
		ResponseBody:    responseBody,
		ResponseHeaders: responseHeaders,
	}
}

// Context overflow regex patterns per provider.
var contextOverflowPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)prompt is too long`),                     // Anthropic
	regexp.MustCompile(`(?i)input is too long for requested model`),  // Bedrock
	regexp.MustCompile(`(?i)exceeds the context window`),             // OpenAI
	regexp.MustCompile(`(?i)input token count.*exceeds the maximum`), // Google
	regexp.MustCompile(`(?i)maximum prompt length is \d+`),           // xAI
	regexp.MustCompile(`(?i)reduce the length of the messages`),      // Groq
	regexp.MustCompile(`(?i)maximum context length is \d+ tokens`),   // OpenRouter/DeepSeek
	regexp.MustCompile(`(?i)exceeds the limit of \d+`),               // GitHub Copilot
	regexp.MustCompile(`(?i)exceeds the available context size`),     // llama.cpp
	regexp.MustCompile(`(?i)greater than the context length`),        // LM Studio
	regexp.MustCompile(`(?i)context window exceeds limit`),           // MiniMax
	regexp.MustCompile(`(?i)exceeded model token limit`),             // Kimi/Moonshot
	regexp.MustCompile(`(?i)context[_ ]length[_ ]exceeded`),          // Generic
	regexp.MustCompile(`^4(00|13)\s*(status code)?\s*\(no body\)`),   // Cerebras/Mistral
}

// IsContextOverflow checks if an error indicates context window overflow.
func IsContextOverflow(providerID string, statusCode int, message string) bool {
	if statusCode == 413 {
		return true
	}
	for _, re := range contextOverflowPatterns {
		if re.MatchString(message) {
			return true
		}
	}
	return false
}

// ParseRetryAfter parses the Retry-After header from response headers.
// Returns 0 if no header is found or parsing fails.
func ParseRetryAfter(headers map[string]string) time.Duration {
	// Check retry-after-ms first (some providers use this)
	if ms, ok := headers["retry-after-ms"]; ok {
		if v, err := strconv.ParseInt(ms, 10, 64); err == nil && v > 0 {
			return time.Duration(v) * time.Millisecond
		}
	}

	val, ok := headers["retry-after"]
	if !ok {
		return 0
	}

	// Try numeric (seconds)
	if secs, err := strconv.ParseInt(val, 10, 64); err == nil {
		return time.Duration(secs) * time.Second
	}

	// Try date string
	for _, layout := range []string{
		time.RFC1123,
		time.RFC1123Z,
		time.RFC850,
		time.ANSIC,
	} {
		if t, err := time.Parse(layout, val); err == nil {
			d := time.Until(t)
			if d > 0 {
				return d
			}
			return 0
		}
	}

	return 0
}

// RetryDelay calculates the delay before retrying an API call.
// It checks (in order): Retry-After headers, retry delay embedded in error
// message (Google API), then falls back to exponential backoff.
func RetryDelay(attempt int, headers map[string]string, errMsg ...string) time.Duration {
	if d := ParseRetryAfter(headers); d > 0 {
		return d
	}
	// Check for retry delay in error message (e.g., Google's retryDelay field).
	if len(errMsg) > 0 {
		if d := parseRetryDelayFromError(errMsg[0]); d > 0 {
			// Add a small buffer to avoid hitting the limit immediately on retry.
			return d + 200*time.Millisecond
		}
	}
	// Exponential backoff: 2s * 2^(attempt-1), capped at 30s
	d := 2 * time.Second
	for i := 1; i < attempt; i++ {
		d *= 2
	}
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// retryDelayPattern matches retryDelay values in Google API error messages.
// Examples: "retryDelay:0.952635527s", "retryDelay:179.305058ms"
var retryDelayPattern = regexp.MustCompile(`retryDelay:([0-9.]+(?:s|ms))`)

// parseRetryDelayFromError extracts a retry delay from an error message string.
// Returns 0 if no delay is found.
func parseRetryDelayFromError(errMsg string) time.Duration {
	m := retryDelayPattern.FindStringSubmatch(errMsg)
	if len(m) < 2 {
		return 0
	}
	d, err := time.ParseDuration(m[1])
	if err != nil {
		return 0
	}
	return d
}

// ExtractMessage tries to extract a human-readable error message from a
// JSON response body. Falls back to the raw body (truncated to 500 chars).
func ExtractMessage(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	// Try {"error": {"message": "..."}} (OpenAI format)
	var nested struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &nested) == nil && nested.Error.Message != "" {
		return nested.Error.Message
	}

	// Try {"error": "..."} (simple format)
	var simple struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(body), &simple) == nil && simple.Error != "" {
		return simple.Error
	}

	// Try {"message": "..."} (another format)
	var msg struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &msg) == nil && msg.Message != "" {
		return msg.Message
	}

	// Fall back to raw body, truncated
	if len(body) > 500 {
		return body[:500]
	}
	return body
}