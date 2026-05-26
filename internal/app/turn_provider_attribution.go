package app

import (
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

type turnProviderRequestAttribution struct {
	PromptCompactionTokensSaved       int
	HistoryCompactionTokensSaved      int
	CurrentTurnProjectionTokensSaved  int
	ToolDescriptionTokensSaved        int
	ToolSchemaTokensSaved             int
	DeterministicContextTokens        int
	DeterministicContextOmittedTokens int
}

func (a turnProviderRequestAttribution) ToolTokensSaved() int {
	return max(a.ToolDescriptionTokensSaved, 0) + max(a.ToolSchemaTokensSaved, 0)
}

func (a turnProviderRequestAttribution) InputTokensSaved() int {
	return max(a.PromptCompactionTokensSaved, 0) +
		max(a.HistoryCompactionTokensSaved, 0) +
		max(a.CurrentTurnProjectionTokensSaved, 0) +
		a.ToolTokensSaved()
}

func buildTurnProviderRequestAttribution(
	promptCompactionTokensSaved int,
	toolSurface providerToolSurface,
	historyCompaction *events.SessionHistoryContinuationUpdatedPayload,
	currentTurnProjectionTokensSaved int,
) turnProviderRequestAttribution {
	return turnProviderRequestAttribution{
		PromptCompactionTokensSaved:      max(promptCompactionTokensSaved, 0),
		HistoryCompactionTokensSaved:     historyCompactionTokensSaved(historyCompaction),
		CurrentTurnProjectionTokensSaved: max(currentTurnProjectionTokensSaved, 0),
		ToolDescriptionTokensSaved:       toolSurface.DescriptionTokensSaved(),
		ToolSchemaTokensSaved:            toolSurface.SchemaTokensSaved(),
	}
}

func historyCompactionTokensSaved(payload *events.SessionHistoryContinuationUpdatedPayload) int {
	if payload == nil {
		return 0
	}
	return max(continuationEstimatedRequestTokens(payload)-continuationCompactedRequestTokens(payload), 0)
}

func estimateInputSavingsCost(models modelCatalog, ref provider.ModelRef, savedTokens int) float64 {
	if savedTokens <= 0 {
		return 0
	}
	model, ok := catalogModelForRef(models, ref)
	if !ok || model.CostInput <= 0 {
		return 0
	}
	return estimatedTokenCost(savedTokens, model.CostInput)
}

func estimateReportedCacheSavingsCost(models modelCatalog, ref provider.ModelRef, report provider.UsageReport) float64 {
	inputTokens := max(report.InputTokens, 0)
	if inputTokens == 0 {
		return 0
	}
	cacheTokens := max(report.CacheReadInputTokens, 0) + max(report.CacheWriteInputTokens, 0)
	if cacheTokens == 0 {
		return 0
	}
	model, ok := catalogModelForRef(models, ref)
	if !ok || model.CostInput <= 0 {
		return 0
	}
	baselineInputCost := estimatedTokenCost(inputTokens, model.CostInput)
	reported := estimateReportedTurnProviderUsage(models, ref, report, turnProviderUsageEstimate{
		RequestTokens:    inputTokens,
		CompletionTokens: max(report.OutputTokens, 0),
		InputCost:        baselineInputCost,
		OutputCost:       estimatedTokenCost(max(report.OutputTokens, 0), model.CostOutput),
	})
	if reported.CachePricingMissing {
		return 0
	}
	return max(baselineInputCost-reported.InputCost, 0)
}
