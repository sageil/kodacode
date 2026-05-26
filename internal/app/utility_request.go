package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

const (
	defaultUtilityRetryAttempts      = 1
	defaultUtilityRetryAfterMaxDelay = 5 * time.Second
)

var (
	errEmptyUtilityTextResponse = errors.New("empty utility text response")
	errUtilityModelTimedOut     = errors.New("connection to utility model timed out")
)

type utilityRetryPolicy struct {
	Attempts           int
	RetryAfterMaxDelay time.Duration
}

type utilityTextResult struct {
	Text     string
	Attempts []utilityTextAttempt
}

type utilityTextAttempt struct {
	Attempt        int
	Text           string
	Duration       time.Duration
	RequestStarted bool
	RouteTrace     provider.RouteTrace
	UsageReport    *provider.UsageReport
	Error          error
}

func defaultUtilityRetryPolicy() utilityRetryPolicy {
	return utilityRetryPolicy{
		Attempts:           defaultUtilityRetryAttempts,
		RetryAfterMaxDelay: defaultUtilityRetryAfterMaxDelay,
	}
}

func utilityRetryPolicyFromConfig(config Config) utilityRetryPolicy {
	return utilityRetryPolicy{
		Attempts:           config.UtilityModelRetryAttempts,
		RetryAfterMaxDelay: time.Duration(config.UtilityModelRetryAfterMaxSeconds) * time.Second,
	}
}

func requestUtilityText(ctx context.Context, client provider.Client, request provider.Request, timeout time.Duration, policy utilityRetryPolicy) (string, error) {
	result, err := requestUtilityTextWithAttempts(ctx, client, request, timeout, policy)
	return result.Text, err
}

func requestUtilityTextWithAttempts(ctx context.Context, client provider.Client, request provider.Request, timeout time.Duration, policy utilityRetryPolicy) (utilityTextResult, error) {
	if client == nil {
		return utilityTextResult{}, provider.ErrProviderNotConfigured
	}

	request = prepareUtilityTextRequest(request)
	result := utilityTextResult{}
	for attempt := 1; ; attempt++ {
		attemptResult := requestUtilityTextOnceAttempt(ctx, client, request, timeout, attempt)
		result.Attempts = append(result.Attempts, attemptResult)
		if attemptResult.Error == nil {
			result.Text = attemptResult.Text
			return result, nil
		}
		delay, retryable := utilityRetryDecision(ctx, attemptResult.Error, attempt, timeout, policy)
		if !retryable {
			return result, attemptResult.Error
		}
		if delay > 0 {
			if err := waitWithContext(ctx, delay); err != nil {
				return result, err
			}
		}
	}
}

func prepareUtilityTextRequest(request provider.Request) provider.Request {
	request.ThinkingEnabled = false
	request.ThinkingSupported = utilityRequestSupportsReasoningControls(request.Model)
	request.ThinkingMode = utilityReasoningMode(request.Model)
	return request
}

func utilityRequestSupportsReasoningControls(model provider.ModelRef) bool {
	return provider.SupportsThinkingOutputForTurn(model, nil) ||
		len(provider.SupportedReasoningVariantsForTurn(model, nil)) > 0
}

func utilityReasoningMode(model provider.ModelRef) string {
	preferred := []string{
		provider.ReasoningVariantNone,
		provider.ReasoningVariantMinimal,
		provider.ReasoningVariantLow,
	}
	supported := provider.SupportedReasoningVariantsForTurn(model, nil)
	for _, preferredVariant := range preferred {
		for _, variant := range supported {
			if variant == preferredVariant {
				return preferredVariant
			}
		}
	}
	return ""
}

func requestUtilityTextOnceAttempt(ctx context.Context, client provider.Client, request provider.Request, timeout time.Duration, attempt int) utilityTextAttempt {
	startedAt := time.Now()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	stream, err := client.Stream(ctx, request)
	trace := providerAttemptRouteTrace(request, stream, err)
	requestStarted := providerAttemptRequestStarted(request, stream, err)
	finish := func(text string, err error) utilityTextAttempt {
		var usageReport *provider.UsageReport
		if stream != nil {
			_ = stream.Close()
			if report, ok := provider.StreamUsageReport(stream); ok {
				copyReport := report
				usageReport = &copyReport
			}
		}
		return utilityTextAttempt{
			Attempt:        max(attempt, 1),
			Text:           text,
			Duration:       time.Since(startedAt),
			RequestStarted: requestStarted,
			RouteTrace:     trace,
			UsageReport:    usageReport,
			Error:          err,
		}
	}
	if err != nil {
		if utilityAttemptTimedOut(err, timeout) {
			return finish("", errUtilityModelTimedOut)
		}
		return finish("", err)
	}

	var text strings.Builder
	for {
		event, err := stream.Recv()
		switch {
		case errors.Is(err, io.EOF):
			if strings.TrimSpace(text.String()) == "" {
				return finish(text.String(), errEmptyUtilityTextResponse)
			}
			return finish(text.String(), nil)
		case err != nil:
			if utilityAttemptTimedOut(err, timeout) {
				return finish(text.String(), errUtilityModelTimedOut)
			}
			return finish(text.String(), err)
		}
		if event.Kind == provider.EventKindAssistantDelta {
			text.WriteString(event.AssistantDelta)
		}
	}
}

func utilityRetryDecision(ctx context.Context, err error, attempt int, timeout time.Duration, policy utilityRetryPolicy) (time.Duration, bool) {
	if attempt > policy.Attempts {
		return 0, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return 0, false
	}
	if timeout > 0 && errors.Is(err, errUtilityModelTimedOut) {
		return 0, true
	}
	if errors.Is(err, errEmptyUtilityTextResponse) {
		return 0, true
	}
	hint := provider.RetryHintForError(err)
	if !hint.Retryable {
		return 0, false
	}
	delay := hint.RetryAfter
	if delay <= 0 {
		delay = defaultProviderRetryDelay(attempt)
	}
	if policy.RetryAfterMaxDelay > 0 && delay > policy.RetryAfterMaxDelay {
		return 0, false
	}
	return delay, true
}

func utilityAttemptTimedOut(err error, timeout time.Duration) bool {
	return timeout > 0 && errors.Is(err, context.DeadlineExceeded)
}

func utilityTimeoutDuration(timeoutSeconds int) time.Duration {
	if timeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(timeoutSeconds) * time.Second
}

func effectiveUtilityTimeout(configured, fallback time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return fallback
}
