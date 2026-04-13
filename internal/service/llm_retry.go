package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

// maxSilentRetries is the number of times to silently retry transient
// capacity/rate-limit errors before surfacing them to the user.
const maxSilentRetries = 10

// retryChat attempts to open a chat stream, retrying on retryable errors
// with exponential backoff. Transient errors (capacity exhausted, short
// rate limits) are retried silently. The user only sees an error if all
// silent retries fail or the error is terminal (quota exhausted).
func retryChat(
	ctx context.Context,
	req *pipeline.TurnRequest,
	maxRetries int,
	prov provider.Provider,
	modelID string,
	tools []provider.Tool,
	publish func(sessionID string, ev SSEEvent),
	onStream func(<-chan provider.StreamChunk),
) error {
	opts := resolveChatReasoning(req)
	opts.SystemParts = req.SystemParts
	opts.Tools = tools
	opts.MaxTokens = req.Agent.MaxTokens

	budgetStr := "nil"
	if opts.ReasoningBudget != nil {
		budgetStr = fmt.Sprintf("%d", *opts.ReasoningBudget)
	}
	log.Printf("[variant] provider=%s model=%s variant=%q budget=%s effort=%q step=%d",
		prov.ID(), modelID, req.Variant, budgetStr, opts.ReasoningEffort, req.Step)

	req.Messages = sanitizeToolPairs(req.Messages)

	totalAttempts := maxRetries + maxSilentRetries

	for attempt := 1; attempt <= totalAttempts; attempt++ {
		stream, err := prov.Chat(ctx, modelID, req.Messages, opts)
		if err == nil {
			onStream(stream)
			return nil
		}
		errMsg := err.Error()
		log.Printf("llm: chat error at step %d (attempt %d/%d): %v", req.Step, attempt, totalAttempts, err)

		if isInvalidToolCallArgsError(errMsg) {
			if sanitized, n := stripUnknownToolCalls(req.Messages, opts.Tools); n > 0 {
				req.Messages = sanitized
				log.Printf("llm: stripped %d unknown tool calls from history before retry", n)
				continue
			}
		}

		// If the provider doesn't support tool use, retry without tools.
		if isNoToolSupportError(errMsg) && len(opts.Tools) > 0 {
			log.Printf("llm: provider does not support tools, retrying without tools")
			opts.Tools = nil
			stream, err = prov.Chat(ctx, modelID, req.Messages, opts)
			if err == nil {
				onStream(stream)
				return nil
			}
			errMsg = err.Error()
		}

		if !isRetryableError(errMsg) {
			if isRateLimitError(errMsg) {
				return fmt.Errorf("%s", formatRateLimitMessage(errMsg))
			}
			return fmt.Errorf("%s", cleanErrorMessage(errMsg))
		}

		if attempt == totalAttempts {
			return fmt.Errorf("%s", formatRateLimitMessage(errMsg))
		}

		delay := provider.RetryDelay(attempt, nil, errMsg)
		if delay > 5*time.Minute {
			return fmt.Errorf("%s", formatRateLimitMessage(errMsg))
		}

		log.Printf("llm: silently retrying in %v...", delay)

		// Only surface retry messages after exhausting silent retries.
		if attempt > maxSilentRetries {
			visible := attempt - maxSilentRetries
			publish(req.SessionID, SSEEvent{
				Type: "retry",
				Data: SSEErrorData{Message: fmt.Sprintf("%s — retrying in %v (attempt %d/%d)",
					cleanErrorMessage(errMsg), delay, visible+1, maxRetries)},
			})
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("all retry attempts failed, please try again")
}

func isInvalidToolCallArgsError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "400") && strings.Contains(lower, "invalid tool call")
}

// isStreamOptionsError detects 400 errors likely caused by the stream_options
// parameter being rejected by an OpenAI-compatible provider.
func isStreamOptionsError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	if isInvalidToolCallArgsError(errMsg) {
		return false
	}
	return strings.Contains(lower, "400") &&
		(strings.Contains(lower, "invalid") || strings.Contains(lower, "parameter") || strings.Contains(lower, "unknown"))
}

// isToolChoiceError detects errors caused by the provider rejecting the
// tool_choice parameter (e.g., local models, some OpenRouter endpoints).
func isToolChoiceError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return (strings.Contains(lower, "400") || strings.Contains(lower, "404")) &&
		(strings.Contains(lower, "tool_choice") || strings.Contains(lower, "toolchoice") ||
			strings.Contains(lower, "no endpoints found that support") && strings.Contains(lower, "tool_choice"))
}

// isReasoningSummaryError detects errors caused by the API rejecting the
// reasoning summary value (e.g., "concise" not supported, only "detailed").
func isReasoningSummaryError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "unsupported value") && strings.Contains(lower, "supported values")
}

// isNoToolSupportError checks if the error indicates the provider/model doesn't support tool use.
func isNoToolSupportError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "no endpoints found that support tool use") ||
		strings.Contains(lower, "does not support tools") ||
		strings.Contains(lower, "tool use is not supported") ||
		strings.Contains(lower, "does not support function calling")
}

// isRetryableError checks if an error message indicates a retryable condition.
func isRetryableError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	retryPatterns := []string{
		"429", "rate limit", "too many requests",
		"500", "502", "503", "529",
		"internal server error", "bad gateway", "service unavailable", "overloaded",
		"unexpected eof", "connection reset", "broken pipe",
		"stream interrupted",
	}
	for _, p := range retryPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	// Don't retry context overflow or auth errors.
	if provider.IsContextOverflow("", 0, errMsg) {
		return false
	}
	return false
}

// isRateLimitError checks if an error indicates the user has hit their usage limit.
func isRateLimitError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "429") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "overloaded")
}

// formatRateLimitMessage creates a user-friendly rate limit message with reset time.
func formatRateLimitMessage(errMsg string) string {
	// Try to find a reset time in the error or headers.
	// Anthropic typically resets on the hour. Calculate next reset.
	now := time.Now()
	// Round up to the next hour.
	nextHour := now.Truncate(time.Hour).Add(time.Hour)
	zone, _ := now.Zone()
	resetStr := nextHour.Format("3:04pm") + " " + zone

	cleaned := cleanErrorMessage(errMsg)
	lower := strings.ToLower(cleaned)

	if strings.Contains(lower, "overloaded") {
		return fmt.Sprintf("API is overloaded — try again in a few minutes (resets %s)", resetStr)
	}
	return fmt.Sprintf("You've hit your usage limit — resets %s", resetStr)
}

// cleanErrorMessage extracts a human-readable error from the raw error string.
func cleanErrorMessage(errMsg string) string {
	// Strip common prefixes.
	for _, prefix := range []string{
		"llm stream: ",
		"llm chat: ",
		"openai: stream: ",
		"openai: stream interrupted: ",
		"anthropic: stream: ",
		"google: stream: ",
		"google: api error ",
	} {
		errMsg = strings.TrimPrefix(errMsg, prefix)
	}

	// Try to extract message from JSON in the error string.
	if idx := strings.Index(errMsg, "{"); idx >= 0 {
		jsonPart := errMsg[idx:]
		if extracted := provider.ExtractMessage(jsonPart); extracted != "" {
			// Prefix with status info if available.
			prefix := strings.TrimSpace(errMsg[:idx])
			if prefix != "" {
				// Extract just the status code part.
				if strings.Contains(prefix, "400") {
					return extracted
				}
				if strings.Contains(prefix, "401") || strings.Contains(prefix, "403") {
					return "Authentication error: " + extracted
				}
				return extracted
			}
			return extracted
		}
	}

	// Clean up "POST url: status" pattern.
	if strings.HasPrefix(errMsg, "POST ") || strings.HasPrefix(errMsg, "GET ") {
		if idx := strings.LastIndex(errMsg, ": "); idx > 0 {
			return strings.TrimSpace(errMsg[idx+2:])
		}
	}

	return errMsg
}
