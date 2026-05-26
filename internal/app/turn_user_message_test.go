package app

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

func TestUserFacingTurnRetryMessageBusyProvider(t *testing.T) {
	err := &provider.ProviderError{
		Message:    "google: This model is currently experiencing high demand.",
		StatusCode: http.StatusServiceUnavailable,
		Retryable:  true,
	}

	retryAt := time.Now().Add(12 * time.Second)
	if got := userFacingTurnRetryMessage(err, retryAt); got != "The model is busy right now. Trying again in 12s." {
		t.Fatalf("retry message = %q", got)
	}
	if got := userFacingTurnErrorMessage(err); got != "The model is busy right now. Please try again in a moment." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageClassifiesConnectionFailures(t *testing.T) {
	err := errors.New("unexpected EOF")

	retryAt := time.Now().Add(5 * time.Second)
	if got := userFacingTurnRetryMessage(err, retryAt); got != "The connection to the model dropped. Trying again in 5s." {
		t.Fatalf("retry message = %q", got)
	}
	if got := userFacingTurnErrorMessage(err); got != "The connection to the model failed. Check your network, VPN, or firewall and try again. Details: unexpected EOF." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageClassifiesNoRouteToHostFailures(t *testing.T) {
	err := errors.New("read tcp 10.6.1.177:55754->140.82.114.22:443: read: no route to host")

	if got := userFacingTurnErrorMessage(err); got != "The connection to the model failed. Check your network, VPN, or firewall and try again. Details: no route to host." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageClassifiesProviderStreamFailures(t *testing.T) {
	err := errors.New("stream error: stream ID 43; CANCEL")

	retryAt := time.Now().Add(7 * time.Second)
	if got := userFacingTurnRetryMessage(err, retryAt); got != "The provider stopped streaming the response. Trying again in 7s." {
		t.Fatalf("retry message = %q", got)
	}
	if got := userFacingTurnErrorMessage(err); got != "The provider stopped streaming the response before it finished. Please try again. Details: stream ID 43; CANCEL." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageClassifiesProviderInternalStreamFailures(t *testing.T) {
	err := errors.New("stream error: stream ID 43; INTERNAL_ERROR; received from peer")

	retryAt := time.Now().Add(9 * time.Second)
	if got := userFacingTurnRetryMessage(err, retryAt); got != "The provider hit a temporary internal error. Trying again in 9s." {
		t.Fatalf("retry message = %q", got)
	}
	if got := userFacingTurnErrorMessage(err); got != "The provider hit a temporary internal error before the response finished. Please try again. Details: stream error: stream ID 43; INTERNAL_ERROR; received from peer." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageClassifiesHarmonyParserHeaderFailuresAsProviderInternal(t *testing.T) {
	err := errors.New(`unexpected tokens remaining in message header: Some("...<|end|><|start|>assistant<|channel|>commentary")`)

	retryAt := time.Now().Add(6 * time.Second)
	if got := userFacingTurnRetryMessage(err, retryAt); got != "The provider hit a temporary internal error. Trying again in 6s." {
		t.Fatalf("retry message = %q", got)
	}
	if got := userFacingTurnErrorMessage(err); got != "The provider hit a temporary internal error before the response finished. Please try again. Details: unexpected tokens remaining in message header: Some(\"...<|end|><|start|>assistant<|channel|>commentary\")." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnRetryMessageUsesImmediateCopyWhenRetryIsDue(t *testing.T) {
	err := &provider.ProviderError{
		Message:    "provider temporary issue",
		StatusCode: http.StatusTooManyRequests,
		Retryable:  true,
	}

	if got := userFacingTurnRetryMessage(err, time.Now().Add(-1*time.Second)); got != "The provider is handling a lot of requests right now. Trying again now." {
		t.Fatalf("retry message = %q", got)
	}
}

func TestUserFacingTurnMessageClassifiesAuthFailures(t *testing.T) {
	err := &provider.ProviderError{
		Message:    "openai: unauthorized",
		StatusCode: http.StatusUnauthorized,
	}

	if got := userFacingTurnErrorMessage(err); got != "The provider connection was rejected. Check your account or access settings." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageIncludesDetailForUnknownProviderFailure(t *testing.T) {
	err := &provider.ProviderError{Message: "review transport failed"}

	if got := userFacingTurnErrorMessage(err); got != "The provider could not complete this request. Details: review transport failed." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageIncludesTimeoutDetail(t *testing.T) {
	err := &provider.ProviderError{
		Message:    "openai/gpt-5: context deadline exceeded",
		StatusCode: http.StatusRequestTimeout,
	}

	if got := userFacingTurnErrorMessage(err); got != "The connection to the model failed. Check your network, VPN, or firewall and try again. Details: request timed out." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageUsesUtilityTimeoutCopy(t *testing.T) {
	if got := userFacingTurnErrorMessage(errUtilityModelTimedOut); got != "connection to utility model timed out" {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageUsesHTTPStatusForGenericProviderBody(t *testing.T) {
	err := &provider.ProviderError{
		Message:    "openai compatible chat completions api: Provider returned error",
		StatusCode: http.StatusBadGateway,
		Retryable:  true,
	}

	if got := userFacingTurnErrorMessage(err); got != "The provider hit a temporary internal error before the response finished. Please try again. Details: provider returned 502 Bad Gateway." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageUsesHTTPStatusForGenericProviderRequestLabel(t *testing.T) {
	err := &provider.ProviderError{
		Message:    "github copilot chat completions api: Bad Request",
		StatusCode: http.StatusBadRequest,
	}

	if got := userFacingTurnErrorMessage(err); got != "The provider could not complete this request. Details: provider returned 400 Bad Request." {
		t.Fatalf("error message = %q", got)
	}
}

func TestUserFacingTurnMessageUsesProductCopyForTurnFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "no progress",
			err:  ErrTurnStalledNoProgress,
			want: "The model repeated the same tool call and result without making progress.",
		},
		{
			name: "provider request limit",
			err:  ErrTurnExceededProviderRequestLimit,
			want: "The turn reached the assistant roundtrip limit.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := userFacingTurnErrorMessage(tc.err); got != tc.want {
				t.Fatalf("error message = %q, want %q", got, tc.want)
			}
		})
	}
}
