package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/provider"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

type turnIssueKind string

const (
	turnIssueUnknown              turnIssueKind = "unknown"
	turnIssueBusy                 turnIssueKind = "busy"
	turnIssueRateLimited          turnIssueKind = "rate_limited"
	turnIssueConnection           turnIssueKind = "connection"
	turnIssueProviderStream       turnIssueKind = "provider_stream"
	turnIssueProviderInternal     turnIssueKind = "provider_internal"
	turnIssueAuth                 turnIssueKind = "auth"
	turnIssueModelUnavailable     turnIssueKind = "model_unavailable"
	turnIssueProviderMissing      turnIssueKind = "provider_missing"
	turnIssueProviderRequest      turnIssueKind = "provider_request"
	turnIssueContinuation         turnIssueKind = "continuation"
	turnIssueNoProgress           turnIssueKind = "no_progress"
	turnIssueProviderRequestLimit turnIssueKind = "provider_request_limit"
	turnIssueAgentMissing         turnIssueKind = "agent_missing"
	turnIssueWorkflowMissing      turnIssueKind = "workflow_missing"
	turnIssueModelSelection       turnIssueKind = "model_selection"
	turnIssuePlannerContract      turnIssueKind = "planner_contract"
)

func userFacingTurnErrorMessage(err error) string {
	return userFacingTurnMessage(err, false, time.Time{})
}

func userFacingTurnRetryMessage(err error, retryAt time.Time) string {
	return userFacingTurnMessage(err, true, retryAt)
}

func userFacingTurnMessage(err error, retrying bool, retryAt time.Time) string {
	if errors.Is(err, errUtilityModelTimedOut) {
		if retrying {
			return errUtilityModelTimedOut.Error() + ". " + retrySchedulePhrase(retryAt)
		}
		return errUtilityModelTimedOut.Error()
	}
	kind := classifyTurnIssue(err)
	message := ""
	switch kind {
	case turnIssueNoProgress:
		message = "The model repeated the same tool call and result without making progress."
	case turnIssueProviderRequestLimit:
		message = "The turn reached the assistant roundtrip limit."
	case turnIssueBusy:
		if retrying {
			return "The model is busy right now. " + retrySchedulePhrase(retryAt)
		}
		message = "The model is busy right now. Please try again in a moment."
	case turnIssueRateLimited:
		if retrying {
			return "The provider is handling a lot of requests right now. " + retrySchedulePhrase(retryAt)
		}
		message = "The provider is handling a lot of requests right now. Please try again in a moment."
	case turnIssueConnection:
		if retrying {
			return "The connection to the model dropped. " + retrySchedulePhrase(retryAt)
		}
		message = "The connection to the model failed. Check your network, VPN, or firewall and try again."
	case turnIssueProviderStream:
		if retrying {
			return "The provider stopped streaming the response. " + retrySchedulePhrase(retryAt)
		}
		message = "The provider stopped streaming the response before it finished. Please try again."
	case turnIssueProviderInternal:
		if retrying {
			return "The provider hit a temporary internal error. " + retrySchedulePhrase(retryAt)
		}
		message = "The provider hit a temporary internal error before the response finished. Please try again."
	case turnIssueAuth:
		message = "The provider connection was rejected. Check your account or access settings."
	case turnIssueModelUnavailable:
		message = "This model is not available on the current provider."
	case turnIssueProviderMissing:
		message = "No provider connection is configured for this model."
	case turnIssueAgentMissing:
		message = "The selected agent could not be found."
	case turnIssueWorkflowMissing:
		message = "The selected workflow could not be found."
	case turnIssueModelSelection:
		message = ErrModelSelectionRequired.Error()
	case turnIssuePlannerContract:
		message = "The planner attempted to finish the save-plan workflow before showing the plan and receiving user approval."
	case turnIssueProviderRequest:
		if retrying {
			return "The provider hit a temporary issue. " + retrySchedulePhrase(retryAt)
		}
		message = "The provider could not complete this request."
	case turnIssueContinuation:
		message = "This provider route cannot continue the current open tool loop yet."
	default:
		var budgetErr BudgetExceededError
		if errors.As(err, &budgetErr) {
			return budgetErr.Error()
		}
		if retrying {
			return "Something temporary went wrong. " + retrySchedulePhrase(retryAt)
		}
		message = "This turn could not be completed."
	}
	if !retrying {
		if detail := userFacingTurnFailureDetail(err, kind); detail != "" {
			return message + " Details: " + detail
		}
	}
	return message
}

func retrySchedulePhrase(retryAt time.Time) string {
	wait := time.Until(retryAt)
	if retryAt.IsZero() || wait <= 0 {
		return "Trying again now."
	}
	return "Trying again in " + formatRetryWait(wait) + "."
}

func formatRetryWait(wait time.Duration) string {
	if wait <= 0 {
		return "0s"
	}
	if wait < time.Second {
		return wait.Round(100 * time.Millisecond).String()
	}
	return wait.Round(time.Second).String()
}

func classifyTurnIssue(err error) turnIssueKind {
	switch {
	case err == nil:
		return turnIssueUnknown
	case errors.Is(err, ErrNativeToolContinuationContractUnsupported):
		return turnIssueContinuation
	case errors.Is(err, agent.ErrAgentNotFound):
		return turnIssueAgentMissing
	case errors.Is(err, workflowpkg.ErrWorkflowNotFound):
		return turnIssueWorkflowMissing
	case errors.Is(err, ErrModelSelectionRequired):
		return turnIssueModelSelection
	case errors.Is(err, ErrTurnStalledNoProgress):
		return turnIssueNoProgress
	case errors.Is(err, ErrTurnExceededProviderRequestLimit):
		return turnIssueProviderRequestLimit
	case errors.Is(err, ErrPlannerSavePlanQuestionRequiresVisiblePlan), errors.Is(err, ErrPlannerSavePlanQuestionInvalid), errors.Is(err, ErrPlannerPlanApprovalDisabled):
		return turnIssuePlannerContract
	}

	var budgetErr BudgetExceededError
	if errors.As(err, &budgetErr) {
		return turnIssueUnknown
	}

	var providerErr *provider.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		if provider.LooksLikeAuthProviderResponse(providerErr.StatusCode, providerErr.Error()) {
			return turnIssueAuth
		}
		switch providerErr.StatusCode {
		case http.StatusRequestTimeout:
			return turnIssueConnection
		case http.StatusUnauthorized, http.StatusForbidden:
			return turnIssueAuth
		case http.StatusNotFound:
			return turnIssueModelUnavailable
		case http.StatusTooManyRequests:
			return turnIssueRateLimited
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusGatewayTimeout:
			return turnIssueProviderInternal
		case http.StatusBadRequest, http.StatusConflict, http.StatusUnprocessableEntity:
			return turnIssueProviderRequest
		}
	}

	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case containsAny(lower, "no provider stream configured"):
		return turnIssueProviderMissing
	case containsAny(lower, "stream error", "streaming error"):
		if containsAny(lower, "internal_error", "server_error", "received from peer") {
			return turnIssueProviderInternal
		}
		return turnIssueProviderStream
	case containsAny(lower, "rate limit", "rate_limit", "too many requests", "too_many_requests"):
		return turnIssueRateLimited
	case containsAny(lower, "high demand", "busy", "overload", "overloaded", "service unavailable", "unavailable", "try again later", "temporarily unavailable"):
		return turnIssueBusy
	case containsAny(lower,
		"unexpected eof",
		"context deadline exceeded",
		"deadline exceeded",
		"timeout",
		"timed out",
		"connection reset",
		"connection refused",
		"broken pipe",
		"temporary failure",
		"connection dropped",
		"no route to host",
		"network is unreachable",
		"host is unreachable",
		"no such host",
		"i/o timeout",
		"tls handshake timeout",
		"dial tcp",
		"read tcp",
		"lookup ",
	):
		return turnIssueConnection
	case containsAny(lower, "internal_error", "server_error", "bad gateway", "gateway timeout", "unexpected tokens remaining in message header", "message header"):
		return turnIssueProviderInternal
	case containsAny(lower, "unauthorized", "forbidden", "api key", "authentication", "access denied"):
		return turnIssueAuth
	case containsAny(lower, "model not found", "unknown model", "not available on this provider", "does not exist", "not found"):
		return turnIssueModelUnavailable
	case provider.RetryHintForError(err).Retryable:
		return turnIssueProviderRequest
	case providerErr != nil:
		return turnIssueProviderRequest
	default:
		return turnIssueUnknown
	}
}

func userFacingTurnFailureDetail(err error, kind turnIssueKind) string {
	switch kind {
	case turnIssueConnection, turnIssueProviderStream, turnIssueProviderInternal, turnIssueProviderRequest:
	default:
		if kind == turnIssueContinuation && err != nil {
			return ensureSentence(err.Error())
		}
		return ""
	}
	if err == nil {
		return ""
	}

	var providerErr *provider.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		if detail := providerIssueDetail(providerErr.Message, providerErr.StatusCode, kind); detail != "" {
			return detail
		}
		if detail := providerIssueDetail("", providerErr.StatusCode, kind); detail != "" {
			return detail
		}
		if providerErr.Cause != nil {
			if detail := providerIssueDetail(providerErr.Cause.Error(), 0, kind); detail != "" {
				return detail
			}
		}
	}
	return providerIssueDetail(err.Error(), 0, kind)
}

func providerIssueDetail(message string, statusCode int, kind turnIssueKind) string {
	if detail := normalizedProviderMessageDetail(message, kind); detail != "" {
		return detail
	}
	if statusCode > 0 {
		if statusText := strings.TrimSpace(http.StatusText(statusCode)); statusText != "" {
			return fmt.Sprintf("provider returned %d %s.", statusCode, statusText)
		}
		return fmt.Sprintf("provider returned status %d.", statusCode)
	}
	return ""
}

func normalizedProviderMessageDetail(message string, kind turnIssueKind) string {
	text := trimProviderErrorPrefix(message)
	if text == "" {
		return ""
	}
	lower := strings.ToLower(text)
	switch kind {
	case turnIssueConnection:
		return normalizedTransportDetail(lower)
	case turnIssueProviderStream:
		text = trimKnownPrefix(text, "stream error:")
	case turnIssueProviderInternal, turnIssueProviderRequest:
		if isGenericProviderMessage(lower) {
			return ""
		}
	}
	return ensureSentence(text)
}

func normalizedTransportDetail(lower string) string {
	switch {
	case containsAny(lower, "context deadline exceeded", "deadline exceeded", "timeout", "timed out", "i/o timeout", "tls handshake timeout"):
		return "request timed out."
	case containsAny(lower, "unexpected eof"):
		return "unexpected EOF."
	case containsAny(lower, "connection reset"):
		return "connection reset."
	case containsAny(lower, "connection refused"):
		return "connection refused."
	case containsAny(lower, "broken pipe"):
		return "broken pipe."
	case containsAny(lower, "no route to host"):
		return "no route to host."
	case containsAny(lower, "network is unreachable"):
		return "network is unreachable."
	case containsAny(lower, "host is unreachable"):
		return "host is unreachable."
	case containsAny(lower, "no such host", "lookup "):
		return "DNS lookup failed."
	case containsAny(lower, "connection dropped"):
		return "connection dropped."
	default:
		return ""
	}
}

func isGenericProviderMessage(lower string) bool {
	switch strings.TrimSpace(lower) {
	case "",
		"provider error",
		"provider returned error",
		"provider returned an error",
		"unexpected error response",
		"bad request",
		"conflict",
		"unprocessable entity",
		"unauthorized",
		"forbidden",
		"not found",
		"request timeout",
		"too many requests",
		"internal server error",
		"bad gateway",
		"service unavailable",
		"gateway timeout":
		return true
	default:
		return false
	}
}

func trimProviderErrorPrefix(message string) string {
	text := strings.TrimSpace(message)
	for text != "" {
		colon := strings.Index(text, ":")
		if colon <= 0 {
			return text
		}
		prefix := strings.ToLower(strings.TrimSpace(text[:colon]))
		switch {
		case strings.Contains(prefix, "/"),
			prefix == "openai",
			prefix == "anthropic",
			prefix == "google",
			prefix == "github-copilot",
			prefix == "nvidia",
			prefix == "deepseek",
			prefix == "mistral",
			prefix == "togetherai",
			prefix == "openrouter",
			prefix == "local",
			prefix == "proxy",
			strings.HasSuffix(prefix, " api"),
			strings.HasSuffix(prefix, " endpoint"):
			text = strings.TrimSpace(text[colon+1:])
			continue
		default:
			return text
		}
	}
	return ""
}

func trimKnownPrefix(text string, prefix string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(prefix)) {
		return strings.TrimSpace(trimmed[len(prefix):])
	}
	return trimmed
}

func ensureSentence(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, "!") || strings.HasSuffix(trimmed, "?") {
		return trimmed
	}
	return trimmed + "."
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
