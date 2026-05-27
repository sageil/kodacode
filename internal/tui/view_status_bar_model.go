package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func headerModelCapabilitiesLabel(m Model, state events.SessionState) string {
	model, ok := currentStatusAvailableModel(m, state)
	if !ok {
		return ""
	}
	parts := make([]string, 0, 3)
	if model.Reasoning {
		parts = append(parts, m.terminalIcon(terminalIconCheck)+"R")
	}
	if model.ToolCalls {
		parts = append(parts, m.terminalIcon(terminalIconCheck)+"T")
	}
	if model.Vision {
		parts = append(parts, m.terminalIcon(terminalIconCheck)+"V")
	}
	return strings.Join(parts, " ")
}

func headerModelLabel(m Model, state events.SessionState) string {
	base := ""
	model, provider := currentStatusModelDetails(m, state)
	if thinking := statusModelReasoningLabel(m, state); thinking != "" {
		switch {
		case model == "" || model == "unknown":
			if provider == "" || provider == "unknown" {
				base = "model unavailable"
				break
			}
			base = provider + " · " + thinking
		case provider == "" || provider == "unknown":
			base = model + " · " + thinking
		default:
			base = provider + "/" + model + " · " + thinking
		}
	} else {
		switch {
		case model == "" || model == "unknown":
			if provider == "" || provider == "unknown" {
				base = "model unavailable"
				break
			}
			base = provider
		case provider == "" || provider == "unknown":
			base = model
		default:
			base = provider + "/" + model
		}
	}

	if meta := headerModelMetadataLabel(m, state); meta != "" {
		return joinCompactTextParts(" · ", base, meta)
	}
	return base
}

func headerModelMetadataLabel(m Model, state events.SessionState) string {
	parts := make([]string, 0, 2)
	if limit, ok := currentStatusModelHeaderCapacityLimit(m, state); ok {
		parts = append(parts, formatCompactTokenCount(limit))
	}
	if caps := headerModelCapabilitiesLabel(m, state); caps != "" {
		parts = append(parts, caps)
	}
	return strings.Join(parts, " · ")
}

func headerModelZone(m Model, state events.SessionState, width int) string {
	label := strings.TrimSpace(headerModelLabel(m, state))
	if label == "" {
		return ""
	}
	width = max(width, 1)
	modelColor := lipgloss.Color(colorFor(m.theme, "primary", "#e6b450"))
	providerColor := lipgloss.Color(colorFor(m.theme, "secondary", "#39bae6"))
	subtextColor := lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))
	sepColor := lipgloss.Color(lineTone(m))
	thinkingColor := lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))
	providerStyle := lipgloss.NewStyle().Foreground(providerColor)
	modelStyle := lipgloss.NewStyle().Foreground(modelColor)
	separatorStyle := lipgloss.NewStyle().Foreground(sepColor)
	fallbackStyle := lipgloss.NewStyle().Foreground(subtextColor)

	provider, model, thinking, capacity, caps := headerModelZoneParts(m, state)
	if provider == "" && model == "" {
		return lipgloss.NewStyle().
			Foreground(subtextColor).
			Render(truncateEnd(label, width))
	}

	separator := lipgloss.NewStyle().Foreground(subtextColor).Render(" · ")
	renderedBase := headerRenderModelBase(provider, model, lipgloss.Width(provider)+lipgloss.Width(model)+1, providerStyle, modelStyle, separatorStyle, fallbackStyle)
	metadata := make([]headerModelMetadataSegment, 0, 3)
	if thinking != "" {
		metadata = append(metadata, headerModelMetadataSegment{
			plain:  thinking,
			styled: lipgloss.NewStyle().Foreground(thinkingColor).Render(thinking),
		})
	}
	if capacity != "" {
		metadata = append(metadata, headerModelMetadataSegment{
			plain:  capacity,
			styled: lipgloss.NewStyle().Foreground(subtextColor).Render(capacity),
		})
	}
	if caps != "" {
		metadata = append(metadata, headerModelMetadataSegment{
			plain:  caps,
			styled: lipgloss.NewStyle().Foreground(subtextColor).Render(caps),
		})
	}

	rendered := renderedBase
	if len(metadata) > 0 {
		rendered += separator + headerJoinModelMetadata(metadata, separator)
	}
	if lipgloss.Width(rendered) <= width {
		return rendered
	}

	const minBaseWidth = 4
	fitted := make([]headerModelMetadataSegment, 0, len(metadata))
	for idx := len(metadata) - 1; idx >= 0; idx-- {
		candidate := append([]headerModelMetadataSegment{metadata[idx]}, fitted...)
		reserved := headerModelMetadataWidth(candidate)
		if len(candidate) > 0 {
			reserved += lipgloss.Width(separator)
		}
		if width-reserved >= minBaseWidth {
			fitted = candidate
		}
	}
	if len(fitted) == 0 {
		return headerRenderModelBase(provider, model, width, providerStyle, modelStyle, separatorStyle, fallbackStyle)
	}

	reserved := headerModelMetadataWidth(fitted) + lipgloss.Width(separator)
	baseAvail := max(width-reserved, 1)
	return headerRenderModelBase(provider, model, baseAvail, providerStyle, modelStyle, separatorStyle, fallbackStyle) +
		separator +
		headerJoinModelMetadata(fitted, separator)
}

type headerModelMetadataSegment struct {
	plain  string
	styled string
}

func headerJoinModelMetadata(segments []headerModelMetadataSegment, separator string) string {
	parts := make([]string, 0, len(segments)*2-1)
	for idx, segment := range segments {
		if idx > 0 {
			parts = append(parts, separator)
		}
		parts = append(parts, segment.styled)
	}
	return strings.Join(parts, "")
}

func headerModelMetadataWidth(segments []headerModelMetadataSegment) int {
	if len(segments) == 0 {
		return 0
	}
	width := 0
	for idx, segment := range segments {
		if idx > 0 {
			width += lipgloss.Width(" · ")
		}
		width += lipgloss.Width(segment.plain)
	}
	return width
}

func headerRenderModelBase(provider, model string, width int, providerStyle, modelStyle, separatorStyle, fallbackStyle lipgloss.Style) string {
	width = max(width, 1)
	switch {
	case provider == "" && model == "":
		return ""
	case provider == "":
		return modelStyle.Render(truncateMiddle(model, width))
	case model == "":
		return providerStyle.Render(truncateMiddle(provider, width))
	}

	fullWidth := lipgloss.Width(provider) + 1 + lipgloss.Width(model)
	if fullWidth <= width {
		return providerStyle.Render(provider) +
			separatorStyle.Render("/") +
			modelStyle.Render(model)
	}

	if providerWidth := lipgloss.Width(provider); providerWidth+1 < width {
		modelAvail := max(width-providerWidth-1, 1)
		return providerStyle.Render(provider) +
			separatorStyle.Render("/") +
			modelStyle.Render(truncateMiddle(model, modelAvail))
	}

	return fallbackStyle.Render(truncateMiddle(provider+"/"+model, width))
}

func headerModelZoneParts(m Model, state events.SessionState) (provider, model, thinking, capacity, caps string) {
	model, provider = currentStatusModelDetails(m, state)
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "unknown" {
		provider = ""
	}
	if model == "unknown" {
		model = ""
	}
	thinking = strings.TrimSpace(statusModelReasoningLabel(m, state))
	if limit, ok := currentStatusModelHeaderCapacityLimit(m, state); ok {
		capacity = formatCompactTokenCount(limit)
	}
	caps = strings.TrimSpace(headerModelCapabilitiesLabel(m, state))
	return provider, model, thinking, capacity, caps
}

func currentStatusModelDetails(m Model, state events.SessionState) (string, string) {
	ref, ok := currentStatusModelRef(m, state)
	if !ok {
		return "unknown", "unknown"
	}
	return inspectorModelDetails(ref)
}

func currentStatusModelRef(m Model, state events.SessionState) (provider.ModelRef, bool) {
	if ref, _, ok := runningStatusTurnModelRef(m, state); ok {
		return ref, true
	}
	return effectiveSelectedAgentModelRef(m, state)
}

func runningStatusTurnModelRef(m Model, state events.SessionState) (provider.ModelRef, *events.TurnConfigState, bool) {
	turnID := strings.TrimSpace(effectiveFooterTurnID(m, state))
	if turnID == "" {
		return provider.ModelRef{}, nil, false
	}
	turn := currentTurn(state, turnID)
	if turn == nil || isTurnFinished(turn) || turn.Config == nil {
		return provider.ModelRef{}, nil, false
	}
	ref, err := provider.ParseModelRef(strings.TrimSpace(turn.Config.Model))
	if err != nil {
		return provider.ModelRef{}, nil, false
	}
	return ref, turn.Config, true
}

func currentStatusAvailableModel(m Model, state events.SessionState) (app.AvailableModel, bool) {
	ref, ok := currentStatusModelRef(m, state)
	if !ok {
		return app.AvailableModel{}, false
	}
	model, ok := m.providerCatalog.models[strings.TrimSpace(ref.String())]
	if !ok {
		return app.AvailableModel{}, false
	}
	return model, true
}

func currentStatusModelContextLimit(m Model, state events.SessionState) (int, bool) {
	if model, ok := currentStatusAvailableModel(m, state); ok {
		limit := model.Capacity.InputTokens
		if limit > 0 {
			return limit, true
		}
	}
	return 0, false
}

func currentStatusModelHeaderCapacityLimit(m Model, state events.SessionState) (int, bool) {
	if model, ok := currentStatusAvailableModel(m, state); ok {
		limit := model.Capacity.InputTokens
		if model.Capacity.HasDistinctWindow() && model.Capacity.WindowTokens > 0 {
			limit = model.Capacity.WindowTokens
		}
		if limit > 0 {
			return limit, true
		}
	}
	return 0, false
}

func statusModelReasoningLabel(m Model, state events.SessionState) string {
	if _, config, ok := runningStatusTurnModelRef(m, state); ok {
		return turnReasoningLabel(config)
	}
	return currentSessionReasoningLabel(m, state)
}

func headerEstimatedCostLabel(m Model, state events.SessionState) string {
	costLabel := effectiveSessionEstimatedCostLabel(m, state)
	if costLabel == "" {
		return ""
	}
	fields := strings.Fields(costLabel)
	if len(fields) < 2 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render(costLabel)
	}
	cost, _ := effectiveSessionEstimatedCost(m, state)
	label := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render(fields[0])
	value := lipgloss.NewStyle().
		Foreground(lipgloss.Color(estimatedCostColor(m, cost))).
		Render(strings.Join(fields[1:], " "))
	return label + " " + value
}

func effectiveSessionEstimatedCost(m Model, state events.SessionState) (float64, bool) {
	if summary, ok := effectiveSessionUsageSummary(m, state); ok {
		if summary.EstimatedCost <= 0 {
			return 0, false
		}
		return summary.EstimatedCost, true
	}
	cost := sessionEstimatedCost(state)
	if cost <= 0 {
		return 0, false
	}
	return cost, true
}

func estimatedCostColor(m Model, cost float64) string {
	switch {
	case cost >= 4.0:
		return colorFor(m.theme, "error", "#ff9aa6")
	case cost >= 1.0:
		return colorFor(m.theme, "warning", "#ffd28f")
	default:
		return colorFor(m.theme, "success", "#90e5b4")
	}
}

func headerCenterZone(modelZone, separator, metricsZone string) string {
	switch {
	case modelZone == "":
		return metricsZone
	case metricsZone == "":
		return modelZone
	default:
		return modelZone + separator + metricsZone
	}
}
