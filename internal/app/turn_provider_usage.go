package app

import (
	"context"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

type turnProviderUsageEstimate struct {
	RequestTokens       int
	CompletionTokens    int
	InputCost           float64
	OutputCost          float64
	CachePricingApplied bool
	CachePricingMissing bool
}

func estimateTurnProviderUsage(models modelCatalog, ref provider.ModelRef, requestTokens, completionTokens int) turnProviderUsageEstimate {
	estimate := turnProviderUsageEstimate{
		RequestTokens:    max(requestTokens, 0),
		CompletionTokens: max(completionTokens, 0),
	}
	model, ok := catalogModelForRef(models, ref)
	if !ok {
		return estimate
	}
	estimate.InputCost = estimatedTokenCost(estimate.RequestTokens, model.CostInput)
	estimate.OutputCost = estimatedTokenCost(estimate.CompletionTokens, model.CostOutput)
	return estimate
}

func estimateReportedTurnProviderUsage(models modelCatalog, ref provider.ModelRef, report provider.UsageReport, fallback turnProviderUsageEstimate) turnProviderUsageEstimate {
	ref = providerReportedModelRef(ref, report.Model)
	estimate := turnProviderUsageEstimate{
		RequestTokens:    max(report.InputTokens, 0),
		CompletionTokens: max(report.OutputTokens, 0),
		InputCost:        fallback.InputCost,
		OutputCost:       fallback.OutputCost,
	}
	model, ok := catalogModelForRef(models, ref)
	if !ok {
		if max(report.CacheReadInputTokens, 0)+max(report.CacheWriteInputTokens, 0) > 0 {
			estimate.CachePricingMissing = true
		}
		return estimate
	}

	if model.CostOutput > 0 {
		estimate.OutputCost = estimatedTokenCost(estimate.CompletionTokens, model.CostOutput)
	}

	uncachedTokens, cacheReadTokens, cacheWriteTokens := reportedInputTokenBillingBuckets(ref, report)

	requiresInputPrice := uncachedTokens > 0
	requiresReadPrice := cacheReadTokens > 0
	requiresWritePrice := cacheWriteTokens > 0

	switch {
	case !requiresInputPrice && !requiresReadPrice && !requiresWritePrice:
		return estimate
	case (!requiresInputPrice || model.CostInput > 0) &&
		(!requiresReadPrice || model.CostCacheRead > 0) &&
		(!requiresWritePrice || model.CostCacheWrite > 0):
		estimate.InputCost = estimatedTokenCost(uncachedTokens, model.CostInput) +
			estimatedTokenCost(cacheReadTokens, model.CostCacheRead) +
			estimatedTokenCost(cacheWriteTokens, model.CostCacheWrite)
		estimate.CachePricingApplied = requiresReadPrice || requiresWritePrice
	case requiresReadPrice || requiresWritePrice:
		estimate.CachePricingMissing = true
	case model.CostInput > 0:
		estimate.InputCost = estimatedTokenCost(estimate.RequestTokens, model.CostInput)
	}
	return estimate
}

func reportedInputTokenBillingBuckets(ref provider.ModelRef, report provider.UsageReport) (uncachedTokens, cacheReadTokens, cacheWriteTokens int) {
	inputTokens := max(report.InputTokens, 0)
	cacheReadTokens = max(report.CacheReadInputTokens, 0)
	cacheWriteTokens = max(report.CacheWriteInputTokens, 0)

	switch provider.CanonicalProviderID(ref.ProviderID) {
	case "anthropic":
		// Anthropic reports input_tokens, cache_read_input_tokens, and
		// cache_creation_input_tokens as distinct billing buckets.
		return inputTokens, cacheReadTokens, cacheWriteTokens
	default:
		// OpenAI-compatible usage reports cached tokens as a subset of input
		// tokens, so clamp cache buckets inside the reported input total.
		if cacheWriteTokens > inputTokens {
			cacheWriteTokens = inputTokens
		}
		if cacheReadTokens > inputTokens-cacheWriteTokens {
			cacheReadTokens = max(inputTokens-cacheWriteTokens, 0)
		}
		return max(inputTokens-cacheReadTokens-cacheWriteTokens, 0), cacheReadTokens, cacheWriteTokens
	}
}

func providerReportedModelRef(base provider.ModelRef, raw string) provider.ModelRef {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return base
	}
	if parsed, err := provider.ParseModelRef(trimmed); err == nil {
		return parsed
	}
	ref := provider.ModelRef{
		ProviderID: strings.TrimSpace(base.ProviderID),
		ModelID:    trimmed,
	}
	if err := ref.Validate(); err != nil {
		return base
	}
	return ref
}

func providerReportedModelLabel(base provider.ModelRef, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if parsed, err := provider.ParseModelRef(trimmed); err == nil {
		return parsed.String()
	}
	ref := providerReportedModelRef(base, trimmed)
	if ref == base && ref.String() != "" && trimmed != ref.String() {
		return trimmed
	}
	return ref.String()
}

func estimatedTokenCost(tokens int, pricePerMillion float64) float64 {
	if tokens <= 0 || pricePerMillion <= 0 {
		return 0
	}
	return (float64(tokens) / 1_000_000) * pricePerMillion
}

func recordedTurnProviderUsageCountsFullUsage(payload events.TurnProviderUsageRecordedPayload) bool {
	if strings.TrimSpace(payload.Error) == "" {
		return true
	}
	if payload.DurableProgress {
		return true
	}
	if max(payload.EstimatedCompletionTokens, 0) > 0 {
		return true
	}
	if max(payload.ExecutedTools, 0) > 0 || max(payload.ReusedTools, 0) > 0 {
		return true
	}
	return false
}

func normalizeRecordedTurnProviderUsagePayload(payload events.TurnProviderUsageRecordedPayload) events.TurnProviderUsageRecordedPayload {
	payload.Kind = strings.TrimSpace(payload.Kind)
	if payload.Kind == "" {
		payload.Kind = string(events.TurnProviderUsageKindAgent)
	}
	if !payload.RequestStarted && strings.TrimSpace(payload.Error) != "" {
		return clearUnstartedRecordedProviderUsageEstimate(payload)
	}
	if recordedTurnProviderUsageCountsFullUsage(payload) {
		return payload
	}
	if payload.RequestStarted {
		payload.EstimatedCompletionTokens = 0
		payload.EstimatedOutputCost = 0
		return payload
	}
	payload = clearUnstartedRecordedProviderUsageEstimate(payload)
	return payload
}

func clearUnstartedRecordedProviderUsageEstimate(payload events.TurnProviderUsageRecordedPayload) events.TurnProviderUsageRecordedPayload {
	payload.EstimatedRequestTokens = 0
	payload.EstimatedPromptTokens = 0
	payload.EstimatedConversationTokens = 0
	payload.EstimatedToolNameTokens = 0
	payload.EstimatedToolDescriptionTokens = 0
	payload.EstimatedToolSchemaTokens = 0
	payload.EstimatedPromptCompactionTokensSaved = 0
	payload.EstimatedHistoryCompactionTokensSaved = 0
	payload.EstimatedCurrentTurnProjectionTokensSaved = 0
	payload.EstimatedToolDescriptionTokensSaved = 0
	payload.EstimatedToolSchemaTokensSaved = 0
	payload.ToolCount = 0
	payload.EstimatedCompletionTokens = 0
	payload.EstimatedInputCost = 0
	payload.EstimatedOutputCost = 0
	payload.EstimatedInputSavingsCost = 0
	return payload
}

func (r *TurnRunner) appendTurnProviderUsageRecorded(ctx context.Context, sessionID, turnID string, payload events.TurnProviderUsageRecordedPayload) error {
	payload = normalizeRecordedTurnProviderUsagePayload(payload)
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload:   payload,
	})
	return err
}

func (r *TurnRunner) appendTurnProviderUsageReported(ctx context.Context, sessionID, turnID string, payload events.TurnProviderUsageReportedPayload) error {
	payload.Kind = strings.TrimSpace(payload.Kind)
	if payload.Kind == "" {
		payload.Kind = string(events.TurnProviderUsageKindAgent)
	}
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnProviderUsageReported,
		Payload:   payload,
	})
	return err
}

func providerUsageModelForTrace(requested provider.ModelRef, trace provider.RouteTrace) provider.ModelRef {
	if model, ok := trace.SelectedModel(); ok {
		return model
	}
	if len(trace.Attempts) > 0 {
		last := trace.Attempts[len(trace.Attempts)-1].Model
		if strings.TrimSpace(last.ProviderID) != "" && strings.TrimSpace(last.ModelID) != "" {
			return last
		}
	}
	return requested
}

func providerRouteAttemptPayloads(trace provider.RouteTrace) []events.TurnProviderRouteAttemptPayload {
	if len(trace.Attempts) == 0 {
		return nil
	}
	out := make([]events.TurnProviderRouteAttemptPayload, 0, len(trace.Attempts))
	for _, attempt := range trace.Attempts {
		out = append(out, events.TurnProviderRouteAttemptPayload{
			Model:    attempt.Model.String(),
			Selected: attempt.Selected,
			Error:    errorString(attempt.Error),
		})
	}
	return out
}

func providerAttemptRouteTrace(request provider.Request, stream provider.Stream, err error) provider.RouteTrace {
	if trace, ok := provider.StreamRouteTrace(stream); ok {
		return trace
	}
	if trace, ok := provider.ErrorRouteTrace(err); ok {
		return trace
	}
	trace := provider.RouteTrace{
		RequestedModel: request.Model,
		Attempts: []provider.RouteAttempt{{
			SessionID: request.SessionID,
			TurnID:    request.TurnID,
			AgentID:   request.AgentID,
			Attempt:   1,
			Model:     request.Model,
			Selected:  err == nil,
			Error:     err,
		}},
	}
	return trace
}

func providerAttemptRequestStarted(request provider.Request, stream provider.Stream, err error) bool {
	if stream != nil {
		return true
	}
	if err == nil {
		return false
	}
	if request.Validate() != nil {
		return false
	}
	if trace, ok := provider.ErrorRouteTrace(err); ok {
		for _, attempt := range trace.Attempts {
			if attempt.Selected {
				return true
			}
			if attempt.Error != nil && !errors.Is(attempt.Error, provider.ErrProviderNotConfigured) {
				return true
			}
		}
		return false
	}
	return !errors.Is(err, provider.ErrProviderNotConfigured)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
