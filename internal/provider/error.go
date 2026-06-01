package provider

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var ErrProviderInputLimitExceeded = errors.New("provider input limit exceeded")

type ProviderError struct {
	Message    string
	StatusCode int
	Retryable  bool
	RetryAfter time.Duration
	Cause      error
	AuthDebug  *providerAuthDebugState
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("provider error (%d)", e.StatusCode)
	}
	return "provider error"
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func IsInputLimitExceeded(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrProviderInputLimitExceeded) {
		return true
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		if providerErr.StatusCode == http.StatusRequestEntityTooLarge {
			return true
		}
		if providerInputLimitExceededText(providerErr.Error()) {
			return true
		}
	}
	return providerInputLimitExceededText(err.Error())
}

func providerInputLimitExceededText(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "prompt token count") && strings.Contains(lower, "limit") &&
		(strings.Contains(lower, "exceeds") || strings.Contains(lower, "exceeded")) {
		return true
	}
	if strings.Contains(lower, "maximum context length") {
		return true
	}
	if strings.Contains(lower, "context length") &&
		(strings.Contains(lower, "exceeds") || strings.Contains(lower, "exceeded") || strings.Contains(lower, "too long")) {
		return true
	}
	if strings.Contains(lower, "too many tokens") {
		return true
	}
	if strings.Contains(lower, "input") && strings.Contains(lower, "token") &&
		(strings.Contains(lower, "exceeds") || strings.Contains(lower, "exceeded") || strings.Contains(lower, "too large") || strings.Contains(lower, "too long")) {
		return true
	}
	return false
}

func newProviderHTTPError(prefix string, statusCode int, message string, header http.Header) error {
	text := strings.TrimSpace(message)
	if text == "" {
		text = strings.TrimSpace(http.StatusText(statusCode))
	}
	if prefix = strings.TrimSpace(prefix); prefix != "" && text != "" {
		text = prefix + ": " + text
	} else if prefix != "" {
		text = prefix
	}
	retryable := httpStatusRetryable(statusCode)
	if LooksLikeAuthProviderResponse(statusCode, text) {
		retryable = false
	}
	return newProviderError(text, statusCode, retryable, parseRetryAfterHeader(header), nil)
}

func newProviderError(message string, statusCode int, retryable bool, retryAfter time.Duration, cause error) error {
	text := strings.TrimSpace(message)
	switch {
	case text != "":
	case statusCode > 0:
		text = strings.TrimSpace(http.StatusText(statusCode))
	case cause != nil:
		text = strings.TrimSpace(cause.Error())
	default:
		text = "provider error"
	}
	if cause == nil && (statusCode == http.StatusRequestEntityTooLarge || providerInputLimitExceededText(text)) {
		cause = ErrProviderInputLimitExceeded
	}
	return &ProviderError{
		Message:    text,
		StatusCode: statusCode,
		Retryable:  retryable,
		RetryAfter: retryAfter,
		Cause:      cause,
	}
}

func LooksLikeAuthProviderResponse(statusCode int, message string) bool {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return true
	}

	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case lower == "":
		return false
	case strings.Contains(lower, "permission_denied"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "forbidden"),
		strings.Contains(lower, "authentication"),
		strings.Contains(lower, "access denied"),
		strings.Contains(lower, `403 "forbidden"`),
		strings.Contains(lower, "403 forbidden"),
		strings.Contains(lower, "can't get copilot user by tracking id"),
		strings.Contains(lower, "error getting copilot user"):
		return true
	default:
		return false
	}
}
