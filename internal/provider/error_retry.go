package provider

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type RetryHint struct {
	Retryable  bool
	RetryAfter time.Duration
}

func RetryHintForError(err error) RetryHint {
	return walkRetryHint(err)
}

func walkRetryHint(err error) RetryHint {
	if err == nil {
		return RetryHint{}
	}

	var best RetryHint
	var visit func(error)
	visit = func(current error) {
		if current == nil {
			return
		}

		var providerErr *ProviderError
		if errors.As(current, &providerErr) && providerErr != nil && providerErr.Retryable {
			hint := RetryHint{
				Retryable:  true,
				RetryAfter: providerErr.RetryAfter,
			}
			if shouldPreferRetryHint(hint, best) {
				best = hint
			}
		}

		if hint, ok := classifyTransportRetry(current); ok && shouldPreferRetryHint(hint, best) {
			best = hint
		}

		type unwrapMany interface{ Unwrap() []error }
		if many, ok := current.(unwrapMany); ok {
			for _, next := range many.Unwrap() {
				visit(next)
			}
			return
		}
		if next := errors.Unwrap(current); next != nil {
			visit(next)
		}
	}

	visit(err)
	return best
}

func shouldPreferRetryHint(candidate, current RetryHint) bool {
	if !candidate.Retryable {
		return false
	}
	if !current.Retryable {
		return true
	}
	switch {
	case candidate.RetryAfter > 0 && current.RetryAfter <= 0:
		return true
	case candidate.RetryAfter <= 0 && current.RetryAfter > 0:
		return false
	case candidate.RetryAfter > 0 && current.RetryAfter > 0:
		return candidate.RetryAfter < current.RetryAfter
	default:
		return false
	}
}

func httpStatusRetryable(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return statusCode >= 500 && statusCode <= 599
	}
}

func parseRetryAfterHeader(header http.Header) time.Duration {
	if header == nil {
		return 0
	}
	if value := strings.TrimSpace(header.Get("Retry-After-Ms")); value != "" {
		if millis, err := strconv.ParseInt(value, 10, 64); err == nil && millis > 0 {
			return time.Duration(millis) * time.Millisecond
		}
	}
	if value := strings.TrimSpace(header.Get("Retry-After")); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 {
			return time.Duration(seconds * float64(time.Second))
		}
		if when, err := http.ParseTime(value); err == nil {
			if delay := time.Until(when); delay > 0 {
				return delay
			}
		}
	}
	return 0
}

func retryableProviderSignals(values ...string) bool {
	for _, value := range values {
		lower := strings.ToLower(strings.TrimSpace(value))
		switch {
		case lower == "":
		case strings.Contains(lower, "rate_limit"),
			strings.Contains(lower, "rate limit"),
			strings.Contains(lower, "too_many_requests"),
			strings.Contains(lower, "too many requests"),
			strings.Contains(lower, "overload"),
			strings.Contains(lower, "high demand"),
			strings.Contains(lower, "servererror"),
			strings.Contains(lower, "internalservererror"),
			strings.Contains(lower, "unexpected tokens remaining in message header"),
			strings.Contains(lower, "message header"),
			strings.Contains(lower, "unavailable"),
			strings.Contains(lower, "temporar"),
			strings.Contains(lower, "timeout"),
			strings.Contains(lower, "timed out"),
			strings.Contains(lower, "try again"),
			strings.Contains(lower, "server_error"),
			strings.Contains(lower, "internal_error"),
			strings.Contains(lower, "bad gateway"),
			strings.Contains(lower, "gateway timeout"),
			strings.Contains(lower, "service unavailable"):
			return true
		}
	}
	return false
}

func classifyTransportRetry(err error) (RetryHint, bool) {
	if err == nil {
		return RetryHint{}, false
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return RetryHint{}, false
	case errors.Is(err, io.ErrUnexpectedEOF):
		return RetryHint{Retryable: true}, true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr != nil {
		if netErr.Timeout() || isTemporaryNetError(netErr) {
			return RetryHint{Retryable: true}, true
		}
	}

	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(lower, "unexpected eof"),
		strings.Contains(lower, "connection reset by peer"),
		strings.Contains(lower, "broken pipe"),
		strings.Contains(lower, "temporary failure"),
		strings.Contains(lower, "stream error:") && strings.Contains(lower, "internal_error"),
		strings.Contains(lower, "unexpected tokens remaining in message header"),
		strings.Contains(lower, "timeout"):
		return RetryHint{Retryable: true}, true
	default:
		return RetryHint{}, false
	}
}

type temporaryNetError interface {
	Temporary() bool
}

func isTemporaryNetError(err net.Error) bool {
	temporary, ok := err.(temporaryNetError)
	return ok && temporary.Temporary()
}
