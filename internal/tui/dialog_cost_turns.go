package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func costDialogTurnSection(th *theme.Theme, entry costDialogTurn) string {
	if entry.Turn == nil || entry.Turn.ProviderUsage == nil {
		return ""
	}
	turn := entry.Turn
	usage := turn.ProviderUsage
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorFor(th, "primary", "#7aa2f7"))).
		Render(fmt.Sprintf("Turn %d • %s • %s", entry.Ordinal, costDialogTurnStatus(turn), costDialogTurnCostLabel(entry)))

	lines := []string{
		header,
		fmt.Sprintf("Model: %s", costDialogTurnModel(turn)),
		costDialogTurnTokenLine(entry),
		fmt.Sprintf("Provider activity: %s • %s", assistantRoundtripLabel(max(usage.Steps, 0)), providerCallLabel(max(usage.Attempts, 0))),
	}
	if entry.Savings.HasInputSavings() {
		lines = append(lines, costDialogInputSavingsLine(entry.Savings, " for this turn"))
		if scope := costDialogSavingsScopeLine(max(usage.Attempts, 0), "turn"); scope != "" {
			lines = append(lines, scope)
		}
		if mix := costDialogSavingsMixLine(entry.Savings); mix != "" {
			lines = append(lines, mix)
		}
	}
	if entry.Savings.HasCacheSavings() {
		lines = append(lines, fmt.Sprintf("Estimated cache discount: %s", formatEstimatedCost(entry.Savings.EstimatedCacheSavingsCost)))
	}
	if entry.Coverage != costDialogUsageEstimated && costDialogHasCacheActivity(entry.CacheReadInputTokens, entry.CacheWriteInputTokens) {
		lines = append(lines, "Cost note: "+costDialogTurnCacheNote(turn))
	}
	if activity := costDialogTurnActivity(turn); activity != "" {
		lines = append(lines, "Activity: "+activity)
	}
	if drivers := costDialogTurnDrivers(turn, entry.PricingUnavailable); len(drivers) > 0 {
		lines = append(lines, "Likely spend drivers: "+strings.Join(drivers, " • "))
	}
	if signals := costDialogTurnSignals(turn); len(signals) > 0 {
		lines = append(lines, "Signals: "+strings.Join(signals, " • "))
	}
	if prompt := costDialogPromptPreview(turn); prompt != "" {
		lines = append(lines, "Prompt: "+prompt)
	}
	return strings.Join(lines, "\n")
}

func costDialogTurnStatus(turn *events.TurnState) string {
	if turn == nil {
		return "waiting"
	}
	status := strings.TrimSpace(string(turn.Status))
	if status != "" {
		return status
	}
	if strings.TrimSpace(turn.Error) != "" {
		return string(events.TurnStatusFailed)
	}
	return "waiting"
}

func costDialogTurnCostLabel(entry costDialogTurn) string {
	if entry.PricingUnavailable {
		return "pricing unavailable"
	}
	return "estimated " + formatEstimatedCost(entry.EstimatedCost)
}

func costDialogTurnCacheNote(turn *events.TurnState) string {
	if turn == nil || turn.ProviderReportedUsage == nil || !costDialogHasCacheActivity(turn.ProviderReportedUsage.CacheReadInputTokens, turn.ProviderReportedUsage.CacheWriteInputTokens) {
		return "provider-reported cache activity is present."
	}
	switch {
	case turn.ProviderReportedUsage.CachePricingApplied && turn.ProviderReportedUsage.CachePricingMissing:
		return "cache pricing applied where available; some cache pricing is still unavailable."
	case turn.ProviderReportedUsage.CachePricingApplied:
		return "provider-reported cache pricing applied to the estimate."
	case turn.ProviderReportedUsage.CachePricingMissing:
		return "provider-reported cache activity is present, but cache pricing is unavailable."
	default:
		return "provider-reported cache activity is present."
	}
}

func costDialogCacheTokenParts(readTokens, writeTokens int, suffix string) []string {
	parts := make([]string, 0, 2)
	if read := max(readTokens, 0); read > 0 {
		label := fmt.Sprintf("%d cache-read", read)
		if suffix != "" {
			label += " " + suffix
		}
		parts = append(parts, label)
	}
	if write := max(writeTokens, 0); write > 0 {
		label := fmt.Sprintf("%d cache-write", write)
		if suffix != "" {
			label += " " + suffix
		}
		parts = append(parts, label)
	}
	return parts
}

func costDialogTurnModel(turn *events.TurnState) string {
	if turn == nil {
		return "unknown"
	}
	if turn.ProviderReportedUsage != nil {
		if model := strings.TrimSpace(turn.ProviderReportedUsage.Model); model != "" {
			return model
		}
	}
	if turn.ProviderUsage != nil {
		if model := strings.TrimSpace(turn.ProviderUsage.Model); model != "" {
			return model
		}
	}
	if turn.Config != nil {
		if model := strings.TrimSpace(turn.Config.Model); model != "" {
			return model
		}
	}
	return "unknown"
}

func costDialogTurnSignals(turn *events.TurnState) []string {
	if turn == nil {
		return nil
	}
	signals := make([]string, 0, 4)
	if percent, ok := currentTurnContextPercent(turn); ok {
		signals = append(signals, fmt.Sprintf("ctx %d%%", percent))
	}
	if turn.Continuation != nil {
		if text := strings.TrimSpace(turn.Continuation.ActivityText); text != "" {
			signals = append(signals, text)
		}
	}
	if label := currentTurnPruningLabel(turn); label != "" {
		signals = append(signals, label)
	}
	return signals
}

func costDialogTurnActivity(turn *events.TurnState) string {
	if turn == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if toolCount := len(orderedToolCallIDs(turn)); toolCount > 0 {
		parts = append(parts, pluralize(toolCount, "tool call"))
	}
	if handoffCount := len(orderedHandoffIDs(turn)); handoffCount > 0 {
		parts = append(parts, pluralize(handoffCount, "handoff"))
	}
	return strings.Join(parts, " • ")
}

func costDialogTurnDrivers(turn *events.TurnState, pricingUnavailable bool) []string {
	if turn == nil {
		return nil
	}
	drivers := make([]string, 0, 6)
	if pricingUnavailable {
		drivers = append(drivers, "pricing missing")
	}
	if usage := turn.ProviderUsage; usage != nil {
		if max(usage.Steps, 0) > 1 {
			drivers = append(drivers, "multiple assistant roundtrips")
		}
		if max(usage.Attempts, 0) > 1 {
			drivers = append(drivers, "multiple provider calls")
		}
		if max(usage.RequestTokens, 0) >= 4000 {
			drivers = append(drivers, "large prompt")
		}
		if max(usage.CompletionTokens, 0) >= 1500 {
			drivers = append(drivers, "large completion")
		}
	}
	if mix, ok := traceDialogTurnRequestMix(turn); ok {
		switch traceDialogDominantRequestDriver(mix) {
		case "tool surface":
			drivers = append(drivers, "tool catalog")
		case "conversation replay":
			drivers = append(drivers, "conversation replay")
		case "instructions":
			drivers = append(drivers, "instruction load")
		}
	}
	if len(orderedToolCallIDs(turn)) >= 3 {
		drivers = append(drivers, "tool-heavy")
	}
	if len(orderedHandoffIDs(turn)) > 0 {
		drivers = append(drivers, "delegation")
	}
	if percent, ok := currentTurnContextPercent(turn); ok && percent >= 80 {
		drivers = append(drivers, "high context pressure")
	}
	if turn.Continuation != nil || turn.Pruning != nil {
		drivers = append(drivers, "history pressure")
	}
	return drivers
}

func costDialogPromptPreview(turn *events.TurnState) string {
	if turn == nil {
		return ""
	}
	prompt := strings.Join(strings.Fields(turn.UserText), " ")
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	return truncateEnd(prompt, 240)
}

func assistantRoundtripLabel(n int) string {
	if n == 1 {
		return "1 assistant roundtrip"
	}
	return fmt.Sprintf("%d assistant roundtrips", n)
}

func providerCallLabel(n int) string {
	if n == 1 {
		return "1 provider call"
	}
	return fmt.Sprintf("%d provider calls", n)
}
