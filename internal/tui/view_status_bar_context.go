package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func currentTurnContextPercent(turn *events.TurnState) (int, bool) {
	if turn == nil {
		return 0, false
	}
	_, _, percent, _, ok := currentTurnDisplayContextUsage(turn)
	return percent, ok
}

func currentTurnContextLabel(turn *events.TurnState) (string, int, bool) {
	tokens, limit, percent, source, ok := currentTurnDisplayContextUsage(turn)
	if !ok {
		return "", 0, false
	}
	prefix := "ctx"
	if strings.TrimSpace(source) != "exact" {
		prefix = "ctx~"
	}
	return fmt.Sprintf("%s %s/%s %d%%", prefix, formatCompactTokenCount(tokens), formatCompactTokenCount(limit), percent), percent, true
}

func currentSessionContextLabel(m Model, state events.SessionState) (string, string, bool) {
	display, ok := currentSessionContextDisplay(m, state)
	if !ok {
		return "", "", false
	}
	if display.capacityOnly {
		return formatCompactTokenCount(display.limit),
			colorFor(m.theme, "subtext", "#9da8ca"),
			true
	}
	prefix := "ctx"
	if strings.TrimSpace(display.source) != "exact" {
		prefix = "ctx~"
	}
	if display.last {
		prefix += " last"
	} else if display.peak {
		prefix += " peak"
	}
	return fmt.Sprintf("%s %s/%s %d%%", prefix, formatCompactTokenCount(display.tokens), formatCompactTokenCount(display.limit), display.percent),
		contextStatusColor(m, display.percent),
		true
}

type sessionContextDisplay struct {
	tokens       int
	limit        int
	percent      int
	source       string
	capacityOnly bool
	last         bool
	peak         bool
}

func currentSessionContextDisplay(m Model, state events.SessionState) (sessionContextDisplay, bool) {
	metricsState, turnID, _ := effectiveStatusMetricsScope(m, state)
	turn := currentTurn(metricsState, turnID)
	var currentDisplay sessionContextDisplay
	currentOK := false
	if turn != nil && currentTurnUsesSelectedModel(m, state, turn) {
		currentDisplay, currentOK = contextDisplayForTurn(turn)
	}
	if currentOK {
		return currentDisplay, true
	}
	if limit, ok := currentStatusModelContextLimit(m, state); ok {
		return sessionContextDisplay{
			limit:        limit,
			capacityOnly: true,
		}, true
	}
	if turn != nil && !currentTurnUsesSelectedModel(m, state, turn) {
		return sessionContextDisplay{}, false
	}
	if turn != nil {
		if tokens, limit, percent, source, ok := currentTurnDisplayContextUsage(turn); ok {
			return sessionContextDisplay{
				tokens:  tokens,
				limit:   limit,
				percent: percent,
				source:  source,
			}, true
		}
	}
	return sessionContextDisplay{}, false
}

func currentTurnUsesSelectedModel(m Model, state events.SessionState, turn *events.TurnState) bool {
	if turn == nil {
		return false
	}
	statusRef, ok := currentStatusModelRef(m, state)
	if !ok {
		return true
	}
	statusModel := strings.TrimSpace(statusRef.String())
	if statusModel == "" {
		return true
	}
	if turn.Config != nil && strings.TrimSpace(turn.Config.Model) != "" {
		return strings.TrimSpace(turn.Config.Model) == statusModel
	}
	if attempt := events.LatestAgentProviderAttempt(turn); attempt != nil {
		for _, model := range []string{
			strings.TrimSpace(attempt.ReportedModel),
			strings.TrimSpace(attempt.Model),
			strings.TrimSpace(attempt.RequestedModel),
		} {
			if model != "" {
				return model == statusModel
			}
		}
	}
	return true
}

func contextDisplayGreater(candidate, current sessionContextDisplay) bool {
	if candidate.percent != current.percent {
		return candidate.percent > current.percent
	}
	if candidate.tokens != current.tokens {
		return candidate.tokens > current.tokens
	}
	return candidate.limit > current.limit
}

func contextDisplayForTurn(turn *events.TurnState) (sessionContextDisplay, bool) {
	tokens, limit, percent, source, ok := currentTurnDisplayContextUsage(turn)
	if !ok {
		return sessionContextDisplay{}, false
	}
	return sessionContextDisplay{
		tokens:  tokens,
		limit:   limit,
		percent: percent,
		source:  source,
	}, true
}

func currentTurnDisplayContextUsage(turn *events.TurnState) (tokens, limit, percent int, source string, ok bool) {
	usage, ok := events.DisplayTurnContextUsage(turn)
	if !ok {
		return 0, 0, 0, "", false
	}
	tokens = max(usage.Tokens, 0)
	limit = max(usage.Limit, 0)
	if tokens <= 0 || limit <= 0 {
		return 0, 0, 0, "", false
	}
	percent = int((float64(tokens) / float64(limit)) * 100)
	if percent < 0 {
		percent = 0
	}
	source = strings.TrimSpace(usage.Source)
	if source == "" {
		source = "estimated"
	}
	return tokens, limit, percent, source, true
}

func contextStatusColor(m Model, percent int) string {
	switch {
	case percent >= 95:
		return colorFor(m.theme, "error", "#ff9aa6")
	case percent >= 80:
		return colorFor(m.theme, "warning", "#ffd28f")
	default:
		return colorFor(m.theme, "subtext", "#9da8ca")
	}
}

func formatCompactTokenCount(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	case tokens >= 10_000:
		return fmt.Sprintf("%.1fk", float64(tokens)/1_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fk", float64(tokens)/1_000)
	default:
		return fmt.Sprintf("%d", max(tokens, 0))
	}
}

func contextGaugeBarColor(m Model, percent int) string {
	switch {
	case percent >= 95:
		return colorFor(m.theme, "error", "#ff9aa6")
	case percent >= 80:
		return colorFor(m.theme, "warning", "#ffd28f")
	default:
		return colorFor(m.theme, "primary", "#e6b450")
	}
}

func contextGaugeBar(m Model, percent int) string {
	percent = min(max(percent, 0), 100)
	filled := percent / 10
	partial := 0
	if (percent%10) >= 5 && filled < 10 {
		partial = 1
	}
	empty := 10 - filled - partial
	barColor := contextGaugeBarColor(m, percent)
	emptyColor := toneValue(m.theme, toneLineStrong)
	var parts []string
	if filled+partial > 0 {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(lipgloss.Color(barColor)).
			Render(strings.Repeat("▓", filled)+strings.Repeat("▒", partial)))
	}
	if empty > 0 {
		parts = append(parts, lipgloss.NewStyle().
			Foreground(lipgloss.Color(emptyColor)).
			Render(strings.Repeat("░", empty)))
	}
	return strings.Join(parts, "")
}

func headerContextMetricsZone(m Model, state events.SessionState) string {
	metricsState, _, _ := effectiveStatusMetricsScope(m, state)
	primary := ""
	if display, ok := currentSessionContextDisplay(m, state); ok {
		if !display.capacityOnly {
			barColor := contextGaugeBarColor(m, display.percent)
			label, _, _ := currentSessionContextLabel(m, state)
			ctxLabel := lipgloss.NewStyle().
				Foreground(lipgloss.Color(barColor)).
				Render(label)
			primary = contextGaugeBar(m, display.percent) + " " + ctxLabel
		}
	}

	groups := make([]string, 0, 3)
	if primary != "" {
		groups = append(groups, primary)
	}
	if tokenStr := sessionTokensArrowLabel(m, metricsState); tokenStr != "" {
		groups = append(groups, tokenStr)
	}
	if costLabel := headerEstimatedCostLabel(m, metricsState); costLabel != "" {
		groups = append(groups, costLabel)
	}
	if len(groups) == 0 {
		return ""
	}

	groupSeparator := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "soft", softTextColor))).
		Render(" │ ")
	return strings.Join(groups, groupSeparator)
}
