package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type costDialogTokenUsage struct {
	RequestTokens         int
	CacheReadInputTokens  int
	CacheWriteInputTokens int
	CompletionTokens      int
	ReasoningTokens       int
	TotalTokens           int
	Coverage              costDialogUsageCoverage
	ReportedAttempts      int
	TotalAttempts         int
}

type costDialogSavings struct {
	PromptCompactionTokensSaved      int
	HistoryCompactionTokensSaved     int
	CurrentTurnProjectionTokensSaved int
	ToolDescriptionTokensSaved       int
	ToolSchemaTokensSaved            int
	EstimatedInputSavingsCost        float64
	EstimatedCacheSavingsCost        float64
}

func (s costDialogSavings) ToolTokensSaved() int {
	return max(s.ToolDescriptionTokensSaved, 0) + max(s.ToolSchemaTokensSaved, 0)
}

func (s costDialogSavings) InputTokensSaved() int {
	return max(s.PromptCompactionTokensSaved, 0) +
		max(s.HistoryCompactionTokensSaved, 0) +
		max(s.CurrentTurnProjectionTokensSaved, 0) +
		s.ToolTokensSaved()
}

func (s costDialogSavings) HasInputSavings() bool {
	return s.InputTokensSaved() > 0 || s.EstimatedInputSavingsCost > 0
}

func (s costDialogSavings) HasCacheSavings() bool {
	return s.EstimatedCacheSavingsCost > 0
}

func costDialogTokenUsageForTurn(turn *events.TurnState) costDialogTokenUsage {
	if turn == nil {
		return costDialogTokenUsage{}
	}
	tokenUsage := costDialogTokenUsage{}
	if turn.ProviderUsage != nil {
		tokenUsage.RequestTokens = max(turn.ProviderUsage.RequestTokens, 0)
		tokenUsage.CompletionTokens = max(turn.ProviderUsage.CompletionTokens, 0)
		tokenUsage.TotalAttempts = max(turn.ProviderUsage.Attempts, 0)
	}
	if len(turn.ProviderAttempts) > 0 {
		tokenUsage.TotalAttempts = max(tokenUsage.TotalAttempts, len(turn.ProviderAttempts))
		for _, attempt := range turn.ProviderAttempts {
			if !costDialogAttemptHasReportedUsage(attempt) {
				continue
			}
			tokenUsage.ReportedAttempts++
			tokenUsage.CacheReadInputTokens += max(attempt.ReportedCacheReadInputTokens, 0)
			tokenUsage.CacheWriteInputTokens += max(attempt.ReportedCacheWriteInputTokens, 0)
			tokenUsage.ReasoningTokens += max(attempt.ReportedReasoningTokens, 0)
		}
	} else if turn.ProviderReportedUsage != nil {
		reported := turn.ProviderReportedUsage
		tokenUsage.CacheReadInputTokens = max(reported.CacheReadInputTokens, 0)
		tokenUsage.CacheWriteInputTokens = max(reported.CacheWriteInputTokens, 0)
		tokenUsage.ReasoningTokens = max(reported.ReasoningTokens, 0)
		tokenUsage.ReportedAttempts = max(reported.Attempts, 1)
		tokenUsage.TotalAttempts = max(tokenUsage.TotalAttempts, max(reported.Attempts, 1))
		if tokenUsage.RequestTokens == 0 && tokenUsage.CompletionTokens == 0 {
			tokenUsage.RequestTokens = max(reported.InputTokens, 0)
			tokenUsage.CompletionTokens = max(reported.OutputTokens, 0)
		}
	}
	switch {
	case tokenUsage.ReportedAttempts == 0:
		tokenUsage.Coverage = costDialogUsageEstimated
	case tokenUsage.TotalAttempts > 0 && tokenUsage.ReportedAttempts < tokenUsage.TotalAttempts:
		tokenUsage.Coverage = costDialogUsageMixed
	default:
		tokenUsage.Coverage = costDialogUsageReported
	}
	if tokenUsage.TotalAttempts == 0 {
		tokenUsage.TotalAttempts = tokenUsage.ReportedAttempts
	}
	tokenUsage.TotalTokens = tokenUsage.RequestTokens + tokenUsage.CompletionTokens
	if tokenUsage.Coverage == costDialogUsageReported && turn.ProviderReportedUsage != nil {
		if total := max(turn.ProviderReportedUsage.TotalTokens, 0); total > 0 {
			tokenUsage.TotalTokens = total
		}
	}
	return tokenUsage
}

func (u *costDialogUsageKindTotals) Add(other costDialogUsageKindTotals) {
	if u == nil {
		return
	}
	u.RequestTokens += max(other.RequestTokens, 0)
	u.CompletionTokens += max(other.CompletionTokens, 0)
	u.Attempts += max(other.Attempts, 0)
	u.EstimatedCost += max(other.EstimatedCost, 0)
	u.MissingPricingAttempts += max(other.MissingPricingAttempts, 0)
}

func costDialogTurnUsageForKind(turn *events.TurnState, kind events.TurnProviderUsageKind) costDialogUsageKindTotals {
	if turn == nil || len(turn.ProviderAttempts) == 0 {
		return costDialogUsageKindTotals{}
	}
	out := costDialogUsageKindTotals{}
	for _, attempt := range turn.ProviderAttempts {
		if events.TurnProviderUsageKind(strings.TrimSpace(attempt.Kind)) != kind {
			continue
		}
		requestTokens := max(attempt.RequestTokens, 0)
		completionTokens := max(attempt.CompletionTokens, 0)
		if costDialogAttemptHasReportedUsage(attempt) {
			requestTokens = max(attempt.ReportedInputTokens, 0)
			completionTokens = max(attempt.ReportedOutputTokens, 0)
		}
		estimatedCost := max(attempt.EstimatedInputCost, 0) + max(attempt.EstimatedOutputCost, 0)
		out.RequestTokens += requestTokens
		out.CompletionTokens += completionTokens
		out.Attempts++
		out.EstimatedCost += estimatedCost
		if estimatedCost <= 0 && requestTokens+completionTokens > 0 {
			out.MissingPricingAttempts++
		}
	}
	return out
}

func costDialogTurnSavings(turn *events.TurnState) costDialogSavings {
	if turn == nil {
		return costDialogSavings{}
	}
	savings := costDialogSavings{}
	if len(turn.ProviderAttempts) > 0 {
		for _, attempt := range turn.ProviderAttempts {
			savings.PromptCompactionTokensSaved += max(attempt.PromptCompactionTokensSaved, 0)
			savings.HistoryCompactionTokensSaved += max(attempt.HistoryCompactionTokensSaved, 0)
			savings.CurrentTurnProjectionTokensSaved += max(attempt.CurrentTurnProjectionTokensSaved, 0)
			savings.ToolDescriptionTokensSaved += max(attempt.ToolDescriptionTokensSaved, 0)
			savings.ToolSchemaTokensSaved += max(attempt.ToolSchemaTokensSaved, 0)
			savings.EstimatedInputSavingsCost += attempt.EstimatedInputSavingsCost
			savings.EstimatedCacheSavingsCost += attempt.EstimatedCacheSavingsCost
		}
		return savings
	}
	if reported := turn.ProviderReportedUsage; reported != nil {
		savings.EstimatedCacheSavingsCost = reported.EstimatedCacheSavingsCost
	}
	return savings
}

func costDialogAttemptHasReportedUsage(attempt events.TurnProviderAttemptState) bool {
	return attempt.ReportedRequestID != "" ||
		attempt.ReportedModel != "" ||
		attempt.ReportedInputTokens > 0 ||
		costDialogHasCacheActivity(attempt.ReportedCacheReadInputTokens, attempt.ReportedCacheWriteInputTokens) ||
		attempt.ReportedOutputTokens > 0 ||
		attempt.ReportedReasoningTokens > 0 ||
		attempt.ReportedTotalTokens > 0 ||
		attempt.CachePricingApplied ||
		attempt.CachePricingMissing
}

func costDialogTokenLabel(coverage costDialogUsageCoverage) string {
	switch coverage {
	case costDialogUsageReported:
		return "Provider tokens"
	case costDialogUsageMixed:
		return "Mixed tokens"
	default:
		return "Estimated tokens"
	}
}

func costDialogTurnTokenLine(entry costDialogTurn) string {
	parts := []string{
		fmt.Sprintf("%d input", entry.RequestTokens),
		fmt.Sprintf("%d output", entry.CompletionTokens),
		fmt.Sprintf("%d total", entry.TotalTokens),
	}
	parts = append(parts, costDialogCacheTokenParts(entry.CacheReadInputTokens, entry.CacheWriteInputTokens, "")...)
	if entry.ReasoningTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking", entry.ReasoningTokens))
	}
	if entry.Coverage == costDialogUsageMixed && entry.TotalAttempts > 0 {
		parts = append(parts, fmt.Sprintf("reported %d of %d provider calls", entry.ReportedAttempts, entry.TotalAttempts))
	}
	return costDialogTokenLabel(entry.Coverage) + ": " + strings.Join(parts, " • ")
}

func costDialogUsage(state events.SessionState) (costDialogStats, []costDialogTurn, []costDialogTurn) {
	stats := costDialogStats{TotalTurns: len(orderedSessionTurnIDs(state))}
	pricedTurns := make([]costDialogTurn, 0, stats.TotalTurns)
	unpricedTurns := make([]costDialogTurn, 0, stats.TotalTurns)
	for _, turnID := range orderedSessionTurnIDs(state) {
		turn := state.Turns[turnID]
		completedToolCalls, failedToolCalls, contractViolationCalls := costDialogToolOutcomeCounts(turn)
		batchStats := costDialogToolBatchStats(turn)
		stats.CompletedToolCalls += completedToolCalls
		stats.FailedToolCalls += failedToolCalls
		stats.ContractViolationCalls += contractViolationCalls
		stats.BatchedToolCallBatches += batchStats.Batches
		stats.BatchedToolCalls += batchStats.ToolCalls
		stats.EstimatedBatchProviderCallsAvoided += batchStats.EstimatedProviderCallsAvoided
		if turn == nil || turn.ProviderUsage == nil {
			continue
		}
		usage := turn.ProviderUsage
		estimatedCost := usage.EstimatedInputCost + usage.EstimatedOutputCost
		pricingUnavailable := costDialogPricingUnavailable(turn)
		tokenUsage := costDialogTokenUsageForTurn(turn)
		savings := costDialogTurnSavings(turn)
		requestTokens := tokenUsage.RequestTokens
		completionTokens := tokenUsage.CompletionTokens
		totalTokens := tokenUsage.TotalTokens

		stats.UsageTurns++
		stats.RequestTokens += requestTokens
		stats.CompletionTokens += completionTokens
		stats.CacheReadInputTokens += tokenUsage.CacheReadInputTokens
		stats.CacheWriteInputTokens += tokenUsage.CacheWriteInputTokens
		stats.ReasoningTokens += tokenUsage.ReasoningTokens
		stats.Steps += max(usage.Steps, 0)
		stats.Attempts += max(usage.Attempts, 0)
		stats.EstimatedCost += estimatedCost
		stats.PromptCompactionTokensSaved += savings.PromptCompactionTokensSaved
		stats.HistoryCompactionTokensSaved += savings.HistoryCompactionTokensSaved
		stats.CurrentTurnProjectionTokensSaved += savings.CurrentTurnProjectionTokensSaved
		stats.ToolDescriptionTokensSaved += savings.ToolDescriptionTokensSaved
		stats.ToolSchemaTokensSaved += savings.ToolSchemaTokensSaved
		stats.DeterministicContextTokens += costDialogTurnDeterministicContextTokens(turn)
		stats.DeterministicContextOmittedTokens += costDialogTurnDeterministicContextOmittedTokens(turn)
		stats.EstimatedInputSavingsCost += savings.EstimatedInputSavingsCost
		stats.EstimatedCacheSavingsCost += savings.EstimatedCacheSavingsCost
		stats.UtilityCompactionUsage.Add(costDialogTurnUsageForKind(turn, events.TurnProviderUsageKindUtilityCompaction))
		if turn.Continuation != nil {
			stats.HistoryCompactionSummaryUpdates++
		}
		if turn.Pruning != nil && costDialogPruningPassHasCompactionActivity(turn.Pruning) {
			stats.HistoryCompactionPruningPasses++
		}
		switch tokenUsage.Coverage {
		case costDialogUsageReported:
			stats.FullyReportedTurns++
		case costDialogUsageMixed:
			stats.PartiallyReportedTurns++
		}
		if turn.ProviderReportedUsage != nil {
			if turn.ProviderReportedUsage.CachePricingApplied {
				stats.CachePricingAppliedTurns++
			}
			if turn.ProviderReportedUsage.CachePricingMissing {
				stats.CachePricingMissingTurns++
			}
		}
		if pricingUnavailable {
			stats.MissingPricingTurns++
			stats.UnpricedRequestTokens += requestTokens
			stats.UnpricedCompletionTokens += completionTokens
		}

		entry := costDialogTurn{
			TurnID:                turnID,
			Ordinal:               sessionToolTurnOrdinal(state, turnID),
			Turn:                  turn,
			EstimatedCost:         estimatedCost,
			RequestTokens:         requestTokens,
			CompletionTokens:      completionTokens,
			CacheReadInputTokens:  tokenUsage.CacheReadInputTokens,
			CacheWriteInputTokens: tokenUsage.CacheWriteInputTokens,
			ReasoningTokens:       tokenUsage.ReasoningTokens,
			TotalTokens:           totalTokens,
			PricingUnavailable:    pricingUnavailable,
			Coverage:              tokenUsage.Coverage,
			ReportedAttempts:      tokenUsage.ReportedAttempts,
			TotalAttempts:         tokenUsage.TotalAttempts,
			Savings:               savings,
		}
		if pricingUnavailable {
			unpricedTurns = append(unpricedTurns, entry)
		} else {
			pricedTurns = append(pricedTurns, entry)
		}
	}

	sort.SliceStable(pricedTurns, func(i, j int) bool {
		if pricedTurns[i].EstimatedCost == pricedTurns[j].EstimatedCost {
			if pricedTurns[i].TotalTokens == pricedTurns[j].TotalTokens {
				return pricedTurns[i].Ordinal > pricedTurns[j].Ordinal
			}
			return pricedTurns[i].TotalTokens > pricedTurns[j].TotalTokens
		}
		return pricedTurns[i].EstimatedCost > pricedTurns[j].EstimatedCost
	})
	sort.SliceStable(unpricedTurns, func(i, j int) bool {
		if unpricedTurns[i].TotalTokens == unpricedTurns[j].TotalTokens {
			if unpricedTurns[i].Turn != nil && unpricedTurns[j].Turn != nil && unpricedTurns[i].Turn.ProviderUsage != nil && unpricedTurns[j].Turn.ProviderUsage != nil {
				if unpricedTurns[i].Turn.ProviderUsage.Steps == unpricedTurns[j].Turn.ProviderUsage.Steps {
					return unpricedTurns[i].Ordinal > unpricedTurns[j].Ordinal
				}
				return unpricedTurns[i].Turn.ProviderUsage.Steps > unpricedTurns[j].Turn.ProviderUsage.Steps
			}
			return unpricedTurns[i].Ordinal > unpricedTurns[j].Ordinal
		}
		return unpricedTurns[i].TotalTokens > unpricedTurns[j].TotalTokens
	})
	return stats, pricedTurns, unpricedTurns
}

func costDialogTurnDeterministicContextTokens(turn *events.TurnState) int {
	if turn == nil {
		return 0
	}
	total := 0
	for _, attempt := range turn.ProviderAttempts {
		total += max(attempt.DeterministicContextTokens, 0)
	}
	return total
}

func costDialogTurnDeterministicContextOmittedTokens(turn *events.TurnState) int {
	if turn == nil {
		return 0
	}
	total := 0
	for _, attempt := range turn.ProviderAttempts {
		total += max(attempt.DeterministicContextOmittedTokens, 0)
	}
	return total
}

type costDialogToolBatchTotals struct {
	Batches                       int
	ToolCalls                     int
	EstimatedProviderCallsAvoided int
}

func costDialogToolBatchStats(turn *events.TurnState) costDialogToolBatchTotals {
	if turn == nil {
		return costDialogToolBatchTotals{}
	}
	stats := costDialogToolBatchTotals{}
	for _, batch := range turn.ToolCallBatches {
		callCount := len(batch.CallIDs)
		if callCount <= 1 {
			continue
		}
		stats.Batches++
		stats.ToolCalls += callCount
		stats.EstimatedProviderCallsAvoided += callCount - 1
	}
	return stats
}

func costDialogPruningPassHasCompactionActivity(pruning *events.PruningState) bool {
	if pruning == nil {
		return false
	}
	return max(pruning.CompactedPriorTurns, 0) > 0 || max(pruning.OmittedPriorTurns, 0) > 0
}

func costDialogToolOutcomeCounts(turn *events.TurnState) (completed, failed, contractViolations int) {
	if turn == nil {
		return 0, 0, 0
	}
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil || !call.Completed {
			continue
		}
		completed++
		if call.Succeeded {
			continue
		}
		failed++
		if strings.TrimSpace(call.FailureClass) == "contract_violation" {
			contractViolations++
		}
	}
	return completed, failed, contractViolations
}

func percentOf(part, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(max(part, 0))*100/float64(total) + 0.5)
}

func costDialogPricingUnavailable(turn *events.TurnState) bool {
	if turn == nil || turn.ProviderUsage == nil {
		return false
	}
	usage := turn.ProviderUsage
	estimatedCost := usage.EstimatedInputCost + usage.EstimatedOutputCost
	return estimatedCost <= 0 && (usage.RequestTokens > 0 || usage.CompletionTokens > 0)
}

func costDialogHasCacheActivity(readTokens, writeTokens int) bool {
	return max(readTokens, 0) > 0 || max(writeTokens, 0) > 0
}

func costDialogSavingsFromStats(stats costDialogStats) costDialogSavings {
	return costDialogSavings{
		PromptCompactionTokensSaved:      stats.PromptCompactionTokensSaved,
		HistoryCompactionTokensSaved:     stats.HistoryCompactionTokensSaved,
		CurrentTurnProjectionTokensSaved: stats.CurrentTurnProjectionTokensSaved,
		ToolDescriptionTokensSaved:       stats.ToolDescriptionTokensSaved,
		ToolSchemaTokensSaved:            stats.ToolSchemaTokensSaved,
		EstimatedInputSavingsCost:        stats.EstimatedInputSavingsCost,
		EstimatedCacheSavingsCost:        stats.EstimatedCacheSavingsCost,
	}
}

func costDialogSavingsMixLabel(savings costDialogSavings) string {
	parts := make([]string, 0, 4)
	if saved := max(savings.PromptCompactionTokensSaved, 0); saved > 0 {
		parts = append(parts, fmt.Sprintf("%d prompt compaction", saved))
	}
	if saved := max(savings.HistoryCompactionTokensSaved, 0); saved > 0 {
		parts = append(parts, fmt.Sprintf("%d history compaction", saved))
	}
	if saved := max(savings.CurrentTurnProjectionTokensSaved, 0); saved > 0 {
		parts = append(parts, fmt.Sprintf("%d current-turn projection", saved))
	}
	if saved := savings.ToolTokensSaved(); saved > 0 {
		label := fmt.Sprintf("%d tool catalog compression", saved)
		details := make([]string, 0, 2)
		if schema := max(savings.ToolSchemaTokensSaved, 0); schema > 0 {
			details = append(details, fmt.Sprintf("%d schema", schema))
		}
		if descriptions := max(savings.ToolDescriptionTokensSaved, 0); descriptions > 0 {
			details = append(details, fmt.Sprintf("%d descriptions", descriptions))
		}
		if len(details) > 0 {
			label += " (" + strings.Join(details, " • ") + ")"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " • ")
}

func costDialogInputSavingsLine(savings costDialogSavings, qualifier string) string {
	if savings.EstimatedInputSavingsCost > 0 {
		return fmt.Sprintf(
			"Estimated cumulative input savings%s: %s from %d avoided input tokens",
			qualifier,
			formatEstimatedCost(savings.EstimatedInputSavingsCost),
			savings.InputTokensSaved(),
		)
	}
	return fmt.Sprintf(
		"Estimated cumulative input savings%s: %d avoided input tokens",
		qualifier,
		savings.InputTokensSaved(),
	)
}

func costDialogSavingsScopeLine(providerCalls int, scope string) string {
	scope = strings.TrimSpace(scope)
	if providerCalls <= 0 || scope == "" {
		return ""
	}
	return fmt.Sprintf("Savings scope: aggregated across %s in this %s", providerCallLabel(providerCalls), scope)
}

func costDialogSavingsMixLine(savings costDialogSavings) string {
	mix := costDialogSavingsMixLabel(savings)
	if mix == "" {
		return ""
	}
	return fmt.Sprintf("Savings mix: %s = %d avoided input tokens", mix, savings.InputTokensSaved())
}
