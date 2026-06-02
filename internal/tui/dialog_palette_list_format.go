package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func renderListBadge(th *theme.Theme, kind commandPaletteKind) string {
	var color, label string
	switch kind {
	case commandPaletteModel, commandPaletteUtilityModel, commandPaletteReviewerModel:
		color, label = colorFor(th, "secondary", "#7dcfff"), "model"
	case commandPaletteAgent:
		color, label = colorFor(th, "primary", "#7aa2f7"), "agent"
	case commandPaletteWorkflow:
		color, label = colorFor(th, "warning", "#e0af68"), "flow"
	case commandPaletteActions:
		color, label = colorFor(th, "success", "#9ece6a"), "action"
	default:
		return strings.Repeat(" ", 9)
	}
	bracket := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(th, "soft", "#565f89"))).Render
	return bracket("[") + muted(centerPaletteBadgeLabel(label, 7)) + bracket("]")
}

func centerPaletteBadgeLabel(label string, width int) string {
	label = strings.TrimSpace(label)
	if width <= 0 {
		return ""
	}
	if ansiWidth(label) >= width {
		return truncateEnd(label, width)
	}
	pad := width - ansiWidth(label)
	left := pad / 2
	right := pad - left
	return strings.Repeat(" ", left) + label + strings.Repeat(" ", right)
}

func formatPaletteContextSize(ctx int) string {
	if ctx <= 0 {
		return ""
	}
	if ctx >= 1000000 {
		return fmt.Sprintf("%.0fM", float64(ctx)/1000000)
	}
	return fmt.Sprintf("%dk", ctx/1000)
}

func paletteModelWindowText(item modelItem) string {
	if !item.Capacity.HasDistinctWindow() {
		return ""
	}
	return formatPaletteContextSize(item.Capacity.WindowTokens)
}

func formatPalettePrice(input, output float64) string {
	if input == 0 && output == 0 {
		return "plan"
	}
	return fmt.Sprintf("$%s/$%s", formatPaletteCost(input), formatPaletteCost(output))
}

func formatPaletteCost(value float64) string {
	switch {
	case value == 0:
		return "0"
	case value >= 10:
		return fmt.Sprintf("%.0f", value)
	case value >= 1:
		return fmt.Sprintf("%.1f", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

const (
	paletteModelColProvider = 12
	paletteModelColModel    = 24
	paletteModelColCtx      = 5
	paletteModelColWindow   = 6
	paletteModelColPrice    = 10
)

func paletteModelDisplayNames(item modelItem) (string, string) {
	providerName := item.ProviderName
	modelName := item.ModelName
	if item.Exact {
		providerName = item.Ref.ProviderID
		modelName = item.Ref.ModelID + " (exact)"
	}
	return truncateEnd(providerName, paletteModelColProvider), truncateEnd(modelName, paletteModelColModel)
}

func paletteModelHeaderPlainLine() string {
	return fmt.Sprintf("%-*s %-*s %*s %*s %*s %s",
		paletteModelColProvider, "Provider",
		paletteModelColModel, "Model",
		paletteModelColCtx, "Input",
		paletteModelColWindow, "Window",
		paletteModelColPrice, "$/M",
		"Caps",
	)
}
