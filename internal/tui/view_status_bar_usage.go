package tui

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func sessionTokensArrowLabel(m Model, state events.SessionState) string {
	inputTokens, outputTokens, _, ok := effectiveSessionTokenTotals(m, state)
	if !ok {
		return ""
	}
	scopePrefix := ""
	if summary, ok := effectiveSessionUsageSummary(m, state); ok && summary.HasDelegatedSessions() {
		scopePrefix = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render("Σ")
	}
	upArrow := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "secondary", "#39bae6"))).
		Render("↑")
	downArrow := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#e6b450"))).
		Render("↓")
	sub := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	result := scopePrefix + upArrow + sub.Render(formatCompactTokenCount(inputTokens))
	if outputTokens > 0 {
		result += " " + downArrow + sub.Render(formatCompactTokenCount(outputTokens))
	}
	return result
}

func sessionTokenTotals(state events.SessionState) (inputTokens, outputTokens int, exact bool, ok bool) {
	exact = true
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		turnInputTokens, turnOutputTokens, turnExact, turnOK := sessionTurnTokenTotals(turn)
		if !turnOK {
			continue
		}
		inputTokens += turnInputTokens
		outputTokens += turnOutputTokens
		exact = exact && turnExact
		ok = true
	}
	return inputTokens, outputTokens, exact, ok
}

func sessionTurnTokenTotals(turn *events.TurnState) (inputTokens, outputTokens int, exact bool, ok bool) {
	if turn == nil {
		return 0, 0, true, false
	}
	if turn.ProviderUsage != nil {
		inputTokens = max(turn.ProviderUsage.RequestTokens, 0)
		outputTokens = max(turn.ProviderUsage.CompletionTokens, 0)
	}
	if len(turn.ProviderAttempts) > 0 {
		reportedAttempts := 0
		for _, attempt := range turn.ProviderAttempts {
			if !sessionTurnAttemptHasReportedUsage(attempt) {
				continue
			}
			reportedAttempts++
		}
		if reportedAttempts > 0 {
			return inputTokens, outputTokens, reportedAttempts == len(turn.ProviderAttempts), inputTokens > 0 || outputTokens > 0
		}
		if turn.ProviderReportedUsage != nil && turn.ProviderReportedUsage.Attempts > 0 {
			if inputTokens == 0 && outputTokens == 0 {
				inputTokens = max(turn.ProviderReportedUsage.InputTokens, 0)
				outputTokens = max(turn.ProviderReportedUsage.OutputTokens, 0)
			}
			return inputTokens, outputTokens, max(turn.ProviderReportedUsage.Attempts, 0) == len(turn.ProviderAttempts), inputTokens > 0 || outputTokens > 0
		}
		if inputTokens > 0 || outputTokens > 0 {
			return inputTokens, outputTokens, false, true
		}
		return 0, 0, true, false
	}
	if turn.ProviderReportedUsage != nil && turn.ProviderReportedUsage.Attempts > 0 {
		if inputTokens == 0 && outputTokens == 0 {
			inputTokens = max(turn.ProviderReportedUsage.InputTokens, 0)
			outputTokens = max(turn.ProviderReportedUsage.OutputTokens, 0)
		}
		return inputTokens, outputTokens, true, inputTokens > 0 || outputTokens > 0
	}
	if inputTokens > 0 || outputTokens > 0 {
		return inputTokens, outputTokens, false, true
	}
	return 0, 0, true, false
}

func sessionTurnAttemptHasReportedUsage(attempt events.TurnProviderAttemptState) bool {
	return attempt.ReportedRequestID != "" ||
		attempt.ReportedModel != "" ||
		attempt.ReportedInputTokens > 0 ||
		attempt.ReportedCacheReadInputTokens > 0 ||
		attempt.ReportedCacheWriteInputTokens > 0 ||
		attempt.ReportedOutputTokens > 0 ||
		attempt.ReportedReasoningTokens > 0 ||
		attempt.ReportedTotalTokens > 0 ||
		attempt.CachePricingApplied ||
		attempt.CachePricingMissing
}

func effectiveSessionTokenTotals(m Model, state events.SessionState) (inputTokens, outputTokens int, exact bool, ok bool) {
	if summary, ok := effectiveSessionUsageSummary(m, state); ok {
		return summary.RequestTokens, summary.CompletionTokens, summary.Exact, summary.RequestTokens > 0 || summary.CompletionTokens > 0
	}
	return sessionTokenTotals(state)
}

func renderActivityElapsed(elapsed time.Duration) string {
	if elapsed <= 0 {
		return "0s"
	}
	elapsed = elapsed.Round(time.Second)
	if elapsed < time.Minute {
		return fmt.Sprintf("%ds", int(elapsed/time.Second))
	}
	if elapsed < time.Hour {
		minutes := int(elapsed / time.Minute)
		seconds := int((elapsed % time.Minute) / time.Second)
		return fmt.Sprintf("%dm %02ds", minutes, seconds)
	}
	hours := int(elapsed / time.Hour)
	minutes := int((elapsed % time.Hour) / time.Minute)
	return fmt.Sprintf("%dh %02dm", hours, minutes)
}
