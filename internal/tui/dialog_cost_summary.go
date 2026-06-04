package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func costDialogSummarySection(th *theme.Theme, state events.SessionState, stats costDialogStats, pricedTurns, unpricedTurns []costDialogTurn, budgetStatus app.BudgetStatus, usageSummary app.SessionUsageSummary) string {
	lines := []string{dialogSectionStyle(th).Render("Session Summary")}
	aggregateUsage := usageSummary.ValidFor(state.SessionID) && usageSummary.HasUsage()
	if stats.UsageTurns == 0 && !aggregateUsage {
		lines = append(lines,
			"No provider usage recorded yet for this session.",
			lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(th, "subtext", "#9da8ca"))).
				Render("Estimated tokens and cost appear after an assistant roundtrip completes."),
		)
		return strings.Join(lines, "\n")
	}

	summaryCost := stats.EstimatedCost
	summaryMissingPricingTurns := stats.MissingPricingTurns
	if aggregateUsage {
		summaryCost = usageSummary.EstimatedCost
		summaryMissingPricingTurns = usageSummary.MissingPricingTurns
		stats.CompletedToolCalls = usageSummary.Local.CompletedToolCalls
		stats.FailedToolCalls = usageSummary.Local.FailedToolCalls
		stats.ContractViolationCalls = usageSummary.Local.ContractViolationCalls
	}

	if summaryMissingPricingTurns > 0 {
		switch {
		case summaryCost > 0:
			lines = append(lines, fmt.Sprintf("Estimated priced subtotal: %s", formatEstimatedCost(summaryCost)))
		default:
			lines = append(lines, "Estimated session total: pricing unavailable")
		}
		lines = append(lines,
			fmt.Sprintf("Full session total unavailable: pricing missing for %s", pluralize(summaryMissingPricingTurns, "turn")),
			fmt.Sprintf("Unpriced usage: %d input • %d output", unpricedRequestTokenTotal(stats, usageSummary, aggregateUsage), unpricedCompletionTokenTotal(stats, usageSummary, aggregateUsage)),
		)
	} else {
		lines = append(lines, fmt.Sprintf("Estimated session total: %s", formatEstimatedCost(summaryCost)))
	}
	lines = append(lines,
		fmt.Sprintf("Turns with usage: %d of %d", stats.UsageTurns, stats.TotalTurns),
		costDialogSummaryTokenLine(stats),
		fmt.Sprintf("Provider activity: %s • %s", assistantRoundtripLabel(stats.Steps), providerCallLabel(stats.Attempts)),
	)
	if line := costDialogUsageKindLine("Utility compaction", stats.UtilityCompactionUsage); line != "" {
		lines = append(lines, line)
	}
	if line := costDialogUsageKindLine("Branch summaries", stats.UtilityBranchSummaryUsage); line != "" {
		lines = append(lines, line)
	}
	if line := costDialogHistoryCompactionActivityLine(stats); line != "" {
		lines = append(lines, line)
	}
	lines = append(lines, costDialogToolOutcomeSummaryLines(
		aggregateUsage,
		usageSummary.CompletedToolCalls,
		usageSummary.FailedToolCalls,
		usageSummary.ContractViolationCalls,
		stats.CompletedToolCalls,
		stats.FailedToolCalls,
		stats.ContractViolationCalls,
	)...)
	if line := costDialogBatchEfficiencyLine(stats, false); line != "" {
		lines = append(lines, line)
	}
	lines = append(lines, costDialogScopedCurrentSessionDetailLines(stats, false)...)
	if line := costDialogPromptCacheSupportLine(state, false); line != "" {
		lines = append(lines, line)
	}
	lines = append(lines, costDialogScopedToolOutcomeLines(usageSummary, false)...)
	lines = append(lines, costDialogBudgetLines(budgetStatus)...)
	lines = append(lines, costDialogScopedCurrentSessionTurnHighlights(pricedTurns, unpricedTurns, false)...)
	if summaryMissingPricingTurns > 0 {
		lines = append(lines, fmt.Sprintf("Pricing unavailable for %s", pluralize(summaryMissingPricingTurns, "turn")))
	}
	return strings.Join(lines, "\n")
}

func costDialogAggregateTokenLine(requestTokens, completionTokens, cacheReadTokens, cacheWriteTokens, reasoningTokens int) string {
	parts := []string{
		fmt.Sprintf("%d input", requestTokens),
		fmt.Sprintf("%d output", completionTokens),
	}
	parts = append(parts, costDialogCacheTokenParts(cacheReadTokens, cacheWriteTokens, "")...)
	if reasoningTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking", reasoningTokens))
	}
	return "Estimated tokens: " + strings.Join(parts, " • ")
}

func costDialogHistoryCompactionActivityLine(stats costDialogStats) string {
	summaryUpdates := max(stats.HistoryCompactionSummaryUpdates, 0)
	pruningPasses := max(stats.HistoryCompactionPruningPasses, 0)
	if summaryUpdates == 0 && pruningPasses == 0 {
		return ""
	}
	parts := make([]string, 0, 2)
	if summaryUpdates > 0 {
		parts = append(parts, pluralize(summaryUpdates, "summary update"))
	}
	if pruningPasses > 0 {
		parts = append(parts, pluralize(pruningPasses, "pruning/reuse pass"))
	}
	return "History compaction activity: " + strings.Join(parts, " • ")
}

func costDialogScopedUsageLine(label string, requestTokens, completionTokens int, estimatedCost float64, missingPricingTurns int) string {
	parts := make([]string, 0, 3)
	switch {
	case estimatedCost > 0:
		parts = append(parts, formatEstimatedCost(estimatedCost))
	case missingPricingTurns > 0:
		parts = append(parts, fmt.Sprintf("pricing missing for %s", pluralize(missingPricingTurns, "turn")))
	}
	if requestTokens > 0 || completionTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d input • %d output", requestTokens, completionTokens))
	}
	if len(parts) == 0 {
		parts = append(parts, "no usage recorded")
	}
	return label + ": " + strings.Join(parts, " • ")
}

func unpricedRequestTokenTotal(stats costDialogStats, usageSummary app.SessionUsageSummary, aggregateUsage bool) int {
	if aggregateUsage {
		return usageSummary.UnpricedRequestTokens
	}
	return stats.UnpricedRequestTokens
}

func unpricedCompletionTokenTotal(stats costDialogStats, usageSummary app.SessionUsageSummary, aggregateUsage bool) int {
	if aggregateUsage {
		return usageSummary.UnpricedCompletionTokens
	}
	return stats.UnpricedCompletionTokens
}

func costDialogUsageKindLine(label string, usage costDialogUsageKindTotals) string {
	if usage.Attempts <= 0 {
		return ""
	}
	costLabel := formatEstimatedCost(usage.EstimatedCost)
	if usage.MissingPricingAttempts > 0 {
		if usage.EstimatedCost > 0 {
			costLabel = formatEstimatedCost(usage.EstimatedCost) + " priced subtotal"
		} else {
			costLabel = "pricing unavailable"
		}
	}
	return fmt.Sprintf("%s: %s • %d input • %d output • %s", label, costLabel, max(usage.RequestTokens, 0), max(usage.CompletionTokens, 0), providerCallLabel(max(usage.Attempts, 0)))
}

func costDialogToolOutcomeSummaryLines(aggregateUsage bool, aggregateCompleted, aggregateFailed, aggregateContractViolations, localCompleted, localFailed, localContractViolations int) []string {
	completed := localCompleted
	failed := localFailed
	contractViolations := localContractViolations
	if aggregateUsage {
		completed = aggregateCompleted
		failed = aggregateFailed
		contractViolations = aggregateContractViolations
	}
	return costDialogToolOutcomeLines("Tool outcomes", "Contract violations", completed, failed, contractViolations)
}

func costDialogScopedToolOutcomeLines(usageSummary app.SessionUsageSummary, scoped bool) []string {
	if !scoped {
		return nil
	}
	lines := make([]string, 0, 2)
	lines = append(lines, costDialogToolOutcomeDetailLine(
		"Current session tool outcomes",
		usageSummary.Local.CompletedToolCalls,
		usageSummary.Local.FailedToolCalls,
		usageSummary.Local.ContractViolationCalls,
	))
	return lines
}

func costDialogToolOutcomeLines(outcomesLabel, contractLabel string, completed, failed, contractViolations int) []string {
	if completed <= 0 {
		return nil
	}
	lines := []string{
		fmt.Sprintf("%s: %d completed • %d failed", outcomesLabel, completed, max(failed, 0)),
	}
	parts := []string{fmt.Sprintf("%d", max(contractViolations, 0))}
	if completed > 0 {
		parts = append(parts, fmt.Sprintf("%d%% of completed", percentOf(max(contractViolations, 0), completed)))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d%% of failed", percentOf(max(contractViolations, 0), failed)))
	}
	lines = append(lines, contractLabel+": "+strings.Join(parts, " • "))
	return lines
}

func costDialogToolOutcomeDetailLine(label string, completed, failed, contractViolations int) string {
	if completed <= 0 {
		return label + ": no completed tool calls"
	}
	line := fmt.Sprintf("%s: %d completed • %d failed", label, completed, max(failed, 0))
	if contractViolations > 0 {
		line += fmt.Sprintf(" • %d contract violation", contractViolations)
		if contractViolations != 1 {
			line += "s"
		}
	}
	return line
}

func costDialogBatchEfficiencyLine(stats costDialogStats, scoped bool) string {
	batches := max(stats.BatchedToolCallBatches, 0)
	calls := max(stats.BatchedToolCalls, 0)
	avoided := max(stats.EstimatedBatchProviderCallsAvoided, 0)
	if batches == 0 || calls == 0 {
		return ""
	}
	label := "Batch efficiency"
	if scoped {
		label = "Current session batch efficiency"
	}
	parts := []string{
		fmt.Sprintf("%d batched tool calls in %s", calls, pluralize(batches, "batch")),
	}
	if avoided > 0 {
		parts = append(parts, fmt.Sprintf("estimated %s avoided", providerCallLabel(avoided)))
	}
	return label + ": " + strings.Join(parts, " • ")
}

func costDialogScopedCurrentSessionDetailLines(stats costDialogStats, scoped bool) []string {
	lines := make([]string, 0, 8)
	reportedTurns := stats.FullyReportedTurns + stats.PartiallyReportedTurns
	if reportedTurns > 0 {
		parts := make([]string, 0, 2)
		if stats.FullyReportedTurns > 0 {
			parts = append(parts, pluralize(stats.FullyReportedTurns, "turn")+" fully reported")
		}
		if stats.PartiallyReportedTurns > 0 {
			parts = append(parts, pluralize(stats.PartiallyReportedTurns, "turn")+" partially reported")
		}
		if len(parts) == 0 {
			parts = append(parts, pluralize(reportedTurns, "turn"))
		}
		label := "Provider-reported usage: "
		if scoped {
			label = "Current session provider-reported usage: "
		}
		lines = append(lines, label+strings.Join(parts, " • "))
	}
	if costDialogHasCacheActivity(stats.CacheReadInputTokens, stats.CacheWriteInputTokens) {
		line := "Reported cache activity: " + strings.Join(costDialogCacheTokenParts(stats.CacheReadInputTokens, stats.CacheWriteInputTokens, "input tokens"), " • ")
		if scoped {
			line = "Current session reported cache activity: " + strings.Join(costDialogCacheTokenParts(stats.CacheReadInputTokens, stats.CacheWriteInputTokens, "input tokens"), " • ")
		}
		switch {
		case stats.CachePricingAppliedTurns > 0 && stats.CachePricingMissingTurns > 0:
			line += fmt.Sprintf(" • cache pricing applied on %s • pricing unavailable on %s", pluralize(stats.CachePricingAppliedTurns, "turn"), pluralize(stats.CachePricingMissingTurns, "turn"))
		case stats.CachePricingAppliedTurns > 0:
			line += fmt.Sprintf(" • cache pricing applied on %s", pluralize(stats.CachePricingAppliedTurns, "turn"))
		case stats.CachePricingMissingTurns > 0:
			line += fmt.Sprintf(" • cache pricing unavailable on %s", pluralize(stats.CachePricingMissingTurns, "turn"))
		default:
			line += " • cost remains estimated"
		}
		lines = append(lines, line)
	}
	if line := costDialogDeterministicContextLine(stats, scoped); line != "" {
		lines = append(lines, line)
	}
	if savings := costDialogSavingsFromStats(stats); savings.HasInputSavings() {
		if scoped {
			lines = append(lines, costDialogCurrentSessionInputSavingsLine(savings))
			if scope := costDialogCurrentSessionSavingsScopeLine(stats.Attempts); scope != "" {
				lines = append(lines, scope)
			}
			if mix := costDialogCurrentSessionSavingsMixLine(savings); mix != "" {
				lines = append(lines, mix)
			}
		} else {
			lines = append(lines, costDialogInputSavingsLine(savings, " across this session"))
			if scope := costDialogSavingsScopeLine(stats.Attempts, "session"); scope != "" {
				lines = append(lines, scope)
			}
			if mix := costDialogSavingsMixLine(savings); mix != "" {
				lines = append(lines, mix)
			}
		}
	}
	if savings := costDialogSavingsFromStats(stats); savings.HasCacheSavings() {
		if scoped {
			lines = append(lines, fmt.Sprintf("Current session estimated cache discounts: %s where cache pricing was known", formatEstimatedCost(savings.EstimatedCacheSavingsCost)))
		} else {
			lines = append(lines, fmt.Sprintf("Estimated cache discounts: %s where cache pricing was known", formatEstimatedCost(savings.EstimatedCacheSavingsCost)))
		}
	}
	return lines
}

func costDialogDeterministicContextLine(stats costDialogStats, scoped bool) string {
	included := max(stats.DeterministicContextTokens, 0)
	omitted := max(stats.DeterministicContextOmittedTokens, 0)
	if included == 0 && omitted == 0 {
		return ""
	}
	label := "Deterministic context"
	if scoped {
		label = "Current session deterministic context"
	}
	parts := make([]string, 0, 2)
	if included > 0 {
		parts = append(parts, fmt.Sprintf("%d input tokens included", included))
	}
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("%d omitted under input pressure", omitted))
	}
	return label + ": " + strings.Join(parts, " • ")
}

func costDialogScopedCurrentSessionTurnHighlights(pricedTurns, unpricedTurns []costDialogTurn, scoped bool) []string {
	lines := make([]string, 0, 2)
	if len(pricedTurns) > 0 {
		label := "Highest priced turn"
		if scoped {
			label = "Current session highest priced turn"
		}
		lines = append(lines, fmt.Sprintf("%s: turn %d • %s", label, pricedTurns[0].Ordinal, formatEstimatedCost(pricedTurns[0].EstimatedCost)))
	}
	if len(unpricedTurns) > 0 {
		label := "Highest unpriced turn"
		if scoped {
			label = "Current session highest unpriced turn"
		}
		lines = append(lines, fmt.Sprintf("%s: turn %d • %d tokens", label, unpricedTurns[0].Ordinal, unpricedTurns[0].TotalTokens))
	}
	return lines
}

func costDialogCurrentSessionInputSavingsLine(savings costDialogSavings) string {
	if savings.EstimatedInputSavingsCost > 0 {
		return fmt.Sprintf(
			"Current session estimated cumulative input savings: %s from %d avoided input tokens",
			formatEstimatedCost(savings.EstimatedInputSavingsCost),
			savings.InputTokensSaved(),
		)
	}
	return fmt.Sprintf(
		"Current session estimated cumulative input savings: %d avoided input tokens",
		savings.InputTokensSaved(),
	)
}

func costDialogCurrentSessionSavingsScopeLine(providerCalls int) string {
	if providerCalls <= 0 {
		return ""
	}
	return fmt.Sprintf("Current session savings scope: aggregated across %s", providerCallLabel(providerCalls))
}

func costDialogCurrentSessionSavingsMixLine(savings costDialogSavings) string {
	mix := costDialogSavingsMixLabel(savings)
	if mix == "" {
		return ""
	}
	return fmt.Sprintf("Current session savings mix: %s = %d avoided input tokens", mix, savings.InputTokensSaved())
}

func costDialogBudgetLines(status app.BudgetStatus) []string {
	lines := []string{}
	if status.HasWorkflowBudget() {
		label := "Workflow budget"
		if strings.TrimSpace(status.WorkflowID) != "" {
			label += " (" + strings.TrimSpace(status.WorkflowID) + ")"
		}
		line := fmt.Sprintf(
			"%s: %s of %s used",
			label,
			formatEstimatedCost(status.WorkflowCost),
			formatEstimatedCost(status.WorkflowBudget),
		)
		if status.WorkflowWarnThreshold > 0 {
			line += fmt.Sprintf(" • warn at %.0f%%", status.WorkflowWarnThreshold*100)
		}
		if status.WorkflowMissingPricingTurns > 0 {
			line += fmt.Sprintf(" • pricing missing for %s", pluralize(status.WorkflowMissingPricingTurns, "turn"))
		}
		lines = append(lines, line)
	}
	if status.HasSessionBudget() {
		line := fmt.Sprintf(
			"Session budget: %s of %s used",
			formatEstimatedCost(status.SessionCost),
			formatEstimatedCost(status.SessionBudget),
		)
		if status.SessionWarnThreshold > 0 {
			line += fmt.Sprintf(" • warn at %.0f%%", status.SessionWarnThreshold*100)
		}
		if status.SessionMissingPricingTurns > 0 {
			line += fmt.Sprintf(" • pricing missing for %s", pluralize(status.SessionMissingPricingTurns, "turn"))
		}
		lines = append(lines, line)
	}
	if status.HasTotalBudget() {
		line := fmt.Sprintf(
			"Cross-session budget: %s of %s used",
			formatEstimatedCost(status.TotalCost),
			formatEstimatedCost(status.TotalBudget),
		)
		if status.TotalWarnThreshold > 0 {
			line += fmt.Sprintf(" • warn at %.0f%%", status.TotalWarnThreshold*100)
		}
		if status.TotalMissingPricingTurns > 0 {
			line += fmt.Sprintf(" • pricing missing for %s", pluralize(status.TotalMissingPricingTurns, "turn"))
		}
		lines = append(lines, line)
	}
	return lines
}

func costDialogSummaryTokenLine(stats costDialogStats) string {
	label := "Estimated tokens"
	if stats.PartiallyReportedTurns > 0 || (stats.FullyReportedTurns > 0 && stats.FullyReportedTurns < stats.UsageTurns) {
		label = "Mixed tokens"
	} else if stats.FullyReportedTurns > 0 {
		label = "Provider tokens"
	}
	parts := []string{
		fmt.Sprintf("%d input", stats.RequestTokens),
		fmt.Sprintf("%d output", stats.CompletionTokens),
	}
	parts = append(parts, costDialogCacheTokenParts(stats.CacheReadInputTokens, stats.CacheWriteInputTokens, "")...)
	if stats.ReasoningTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking", stats.ReasoningTokens))
	}
	return label + ": " + strings.Join(parts, " • ")
}
