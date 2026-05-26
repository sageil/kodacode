package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func refreshSessionCompactionPayloadMetadata(
	payload *events.SessionHistoryContinuationUpdatedPayload,
	summaryFallback *events.SessionHistoryContinuationUpdatedPayload,
	route provider.ModelRoute,
	budget sessionHistoryBudget,
	estimatedTokens int,
	compactedTokens int,
	measurementSource provider.TokenCountSource,
	summarySource string,
) *events.SessionHistoryContinuationUpdatedPayload {
	if payload == nil {
		return nil
	}
	copyPayload := cloneCompactionPayload(payload)
	copyPayload.Artifact = normalizeHistoryCompactionArtifact(copyPayload.Artifact)
	if route.Primary != (provider.ModelRef{}) {
		if model := strings.TrimSpace(route.Primary.String()); model != "" {
			copyPayload.Attribution.Model = model
		}
	}
	if source := strings.TrimSpace(budget.InputLimitSource); source != "" {
		copyPayload.Attribution.InputLimitSource = source
	}
	copyPayload.Attribution.MeasurementSource = strings.TrimSpace(copyPayload.Attribution.MeasurementSource)
	if measurementSource != "" {
		copyPayload.Attribution.MeasurementSource = string(measurementSource)
	}
	copyPayload.Attribution.SummarySource = strings.TrimSpace(copyPayload.Attribution.SummarySource)
	if strings.TrimSpace(summarySource) != "" {
		copyPayload.Attribution.SummarySource = strings.TrimSpace(summarySource)
	}
	if copyPayload.InputBudget == nil {
		copyPayload.InputBudget = &events.HistoryInputBudgetPayload{}
	}
	if budget.InputLimitTokens > 0 {
		copyPayload.InputBudget.InputLimitTokens = budget.InputLimitTokens
	}
	if budget.TriggerTokens > 0 {
		copyPayload.InputBudget.TriggerTokens = budget.TriggerTokens
	}
	if budget.TargetTokens > 0 {
		copyPayload.InputBudget.TargetTokens = budget.TargetTokens
	}
	if estimatedTokens > 0 {
		copyPayload.InputBudget.EstimatedRequestTokens = estimatedTokens
	}
	if compactedTokens > 0 {
		copyPayload.InputBudget.ConsolidatedRequestTokens = compactedTokens
	}
	if budget.SummaryBudgetBytes > 0 {
		copyPayload.RenderedSummary = renderSessionCompactionArtifactSummary(copyPayload.Artifact, budget.SummaryBudgetBytes)
	}
	return applySessionCompactionActivityText(copyPayload)
}

func overlaySessionCompactionPayloadMetadata(
	payload *events.SessionHistoryContinuationUpdatedPayload,
	metadataSource *events.SessionHistoryContinuationUpdatedPayload,
) *events.SessionHistoryContinuationUpdatedPayload {
	if payload == nil {
		return nil
	}
	if metadataSource == nil {
		return cloneCompactionPayload(payload)
	}
	copyPayload := cloneCompactionPayload(payload)
	if model := strings.TrimSpace(metadataSource.Attribution.Model); model != "" {
		copyPayload.Attribution.Model = model
	}
	if source := strings.TrimSpace(metadataSource.Attribution.InputLimitSource); source != "" {
		copyPayload.Attribution.InputLimitSource = source
	}
	if source := strings.TrimSpace(metadataSource.Attribution.MeasurementSource); source != "" {
		copyPayload.Attribution.MeasurementSource = source
	}
	if source := strings.TrimSpace(metadataSource.Attribution.SummarySource); source != "" {
		copyPayload.Attribution.SummarySource = source
	}
	if metadataSource.InputBudget != nil {
		if copyPayload.InputBudget == nil {
			copyPayload.InputBudget = &events.HistoryInputBudgetPayload{}
		}
		if metadataSource.InputBudget.InputLimitTokens > 0 {
			copyPayload.InputBudget.InputLimitTokens = metadataSource.InputBudget.InputLimitTokens
		}
		if metadataSource.InputBudget.TriggerTokens > 0 {
			copyPayload.InputBudget.TriggerTokens = metadataSource.InputBudget.TriggerTokens
		}
		if metadataSource.InputBudget.TargetTokens > 0 {
			copyPayload.InputBudget.TargetTokens = metadataSource.InputBudget.TargetTokens
		}
	}
	return applySessionCompactionActivityText(copyPayload)
}

func sessionHistoryRequestUsable(request provider.Request, currentTurnInputs []provider.Input) bool {
	return provider.PromptText(request) != "" || len(currentTurnInputs) > 0
}

func estimateSessionRequestTokens(request provider.Request, historyInputs, currentTurnInputs []provider.Input) int {
	shaped := request
	shaped.Inputs = append(append([]provider.Input(nil), historyInputs...), currentTurnInputs...)
	shaped = provider.PreparePromptRequest(shaped)
	return max(provider.EstimateRequestTokenBreakdown(shaped).TotalTokens, 0)
}

func preserveSessionCompactionMeasurement(history *sessionHistoryState, effectiveCompaction *events.SessionHistoryContinuationUpdatedPayload, rawTokens int, rawSource provider.TokenCountSource) {
	if history == nil {
		return
	}
	if rawSource != "" {
		history.RequestTokenSource = rawSource
	}
	switch {
	case effectiveCompaction != nil && sameSessionCompactionScope(history.ExistingContinuation, effectiveCompaction):
		history.EstimatedTokens = rawTokens
		if history.ExistingContinuation != nil {
			if compactedTokens := continuationCompactedRequestTokens(history.ExistingContinuation); compactedTokens > 0 {
				history.CompactedTokens = compactedTokens
				return
			}
			if estimatedTokens := continuationEstimatedRequestTokens(history.ExistingContinuation); estimatedTokens > 0 {
				history.CompactedTokens = estimatedTokens
				return
			}
		}
		history.CompactedTokens = rawTokens
	default:
		history.EstimatedTokens = rawTokens
		history.CompactedTokens = rawTokens
	}
}

func continuationEstimatedRequestTokens(payload *events.SessionHistoryContinuationUpdatedPayload) int {
	if payload == nil || payload.InputBudget == nil {
		return 0
	}
	return max(payload.InputBudget.EstimatedRequestTokens, 0)
}

func continuationCompactedRequestTokens(payload *events.SessionHistoryContinuationUpdatedPayload) int {
	if payload == nil || payload.InputBudget == nil {
		return 0
	}
	return max(payload.InputBudget.ConsolidatedRequestTokens, 0)
}

func continuationInputLimitTokens(payload *events.SessionHistoryContinuationUpdatedPayload) int {
	if payload == nil || payload.InputBudget == nil {
		return 0
	}
	return max(payload.InputBudget.InputLimitTokens, 0)
}

func continuationTriggerTokens(payload *events.SessionHistoryContinuationUpdatedPayload) int {
	if payload == nil || payload.InputBudget == nil {
		return 0
	}
	return max(payload.InputBudget.TriggerTokens, 0)
}

func continuationTargetTokens(payload *events.SessionHistoryContinuationUpdatedPayload) int {
	if payload == nil || payload.InputBudget == nil {
		return 0
	}
	return max(payload.InputBudget.TargetTokens, 0)
}
