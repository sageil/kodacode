package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func sessionModelRoute(state events.SessionState) (provider.ModelRoute, bool) {
	ref, err := provider.ParseModelRef(strings.TrimSpace(state.Model))
	if err != nil {
		return provider.ModelRoute{}, false
	}
	return provider.ModelRoute{Primary: ref}, true
}

func capabilitySelectionModelRoute(m Model, state events.SessionState) (provider.ModelRoute, bool) {
	if available, ok := m.providerCatalog.agents[strings.TrimSpace(m.agentID)]; ok && modelRouteConfigured(available.ModelRoute) {
		return available.ModelRoute, true
	}
	if route, ok := sessionModelRoute(state); ok {
		return route, true
	}
	if modelRouteConfigured(m.providerCatalog.defaultModelRoute) {
		return m.providerCatalog.defaultModelRoute, true
	}
	return provider.ModelRoute{}, false
}

func availableModelForRef(m Model, ref provider.ModelRef) (app.AvailableModel, bool) {
	model, ok := m.providerCatalog.models[strings.TrimSpace(ref.String())]
	if !ok {
		return app.AvailableModel{}, false
	}
	return model, true
}

func selectedAvailableModelForState(m Model, state events.SessionState) (app.AvailableModel, bool) {
	ref, ok := effectiveSelectedAgentModelRef(m, state)
	if !ok {
		return app.AvailableModel{}, false
	}
	return availableModelForRef(m, ref)
}

func normalizedAvailableModelReasoningVariant(model app.AvailableModel, variant string) string {
	normalized := strings.TrimSpace(strings.ToLower(variant))
	if normalized == "" {
		return ""
	}
	for _, candidate := range model.SupportedReasoningVariants {
		if strings.EqualFold(strings.TrimSpace(candidate), normalized) {
			return normalized
		}
	}
	return ""
}

func sessionSupportsReasoningVariants(m Model, state events.SessionState) bool {
	model, ok := selectedAvailableModelForState(m, state)
	return ok && len(model.SupportedReasoningVariants) > 0
}

func currentSessionSupportsReasoningVariants(m Model) bool {
	return sessionSupportsReasoningVariants(m, m.projector.CurrentState())
}

func sessionSupportsThinkingOutput(m Model, state events.SessionState) bool {
	model, ok := selectedAvailableModelForState(m, state)
	return ok && model.SupportsThinkingOutput
}

func currentSessionSupportsThinkingOutput(m Model) bool {
	return sessionSupportsThinkingOutput(m, m.projector.CurrentState())
}

func sessionSupportsReasoningControl(m Model, state events.SessionState) bool {
	return sessionSupportsReasoningVariants(m, state) || sessionSupportsThinkingOutput(m, state)
}

func currentSessionReasoningVariantLabel(m Model, state events.SessionState) string {
	model, ok := selectedAvailableModelForState(m, state)
	if !ok {
		return ""
	}
	return normalizedAvailableModelReasoningVariant(model, m.reasoningVariant)
}

func currentSessionReasoningLabel(m Model, state events.SessionState) string {
	if _, ok := effectiveSelectedAgentModelRef(m, state); !ok {
		return ""
	}
	if !sessionSupportsReasoningControl(m, state) {
		return ""
	}
	parts := make([]string, 0, 2)
	if sessionSupportsThinkingOutput(m, state) && m.thinkingEnabled {
		parts = append(parts, "thinking")
	}
	if variant := currentSessionReasoningVariantLabel(m, state); variant != "" {
		parts = append(parts, variant)
	}
	return strings.Join(parts, "/")
}

func (m *Model) openReasoningVariantDialog(model provider.ModelRef, variants []string, applyModel bool, view sessionView) tea.Cmd {
	dialog := newReasoningVariantDialog(model, variants, m.reasoningVariant, applyModel, view, m.theme, m.terminalIcons)
	width, height := dialogRenderSize(*m, m.projector.CurrentState())
	dialog.SetFrame(width, height)
	return func() tea.Msg {
		return dialogOpenedMsg{dialog: dialog}
	}
}

func turnReasoningLabel(config *events.TurnConfigState) string {
	if config == nil || (!config.SupportsReasoningVariants && !config.SupportsThinkingOutput) {
		return ""
	}
	parts := make([]string, 0, 2)
	if config.SupportsThinkingOutput && config.ThinkingEnabled {
		parts = append(parts, "thinking")
	}
	if normalized := strings.TrimSpace(config.ThinkingMode); normalized != "" && config.SupportsReasoningVariants {
		parts = append(parts, normalized)
	}
	return strings.Join(parts, "/")
}

func effectiveSelectedAgentModelRoute(m Model, state events.SessionState) (provider.ModelRoute, bool) {
	return capabilitySelectionModelRoute(m, state)
}

func effectiveSelectedAgentModelRef(m Model, state events.SessionState) (provider.ModelRef, bool) {
	route, ok := effectiveSelectedAgentModelRoute(m, state)
	if !ok {
		return provider.ModelRef{}, false
	}
	return route.Primary, true
}

func effectiveSelectedAgentModelDetails(m Model, state events.SessionState) (string, string) {
	ref, ok := effectiveSelectedAgentModelRef(m, state)
	if !ok {
		return "unknown", "unknown"
	}
	modelID := strings.TrimSpace(ref.ModelID)
	if modelID == "" {
		modelID = "unknown"
	}
	providerID := strings.TrimSpace(ref.ProviderID)
	if providerID == "" {
		providerID = "unknown"
	}
	return modelID, providerID
}

func modelRouteConfigured(route provider.ModelRoute) bool {
	return strings.TrimSpace(route.Primary.ProviderID) != "" && strings.TrimSpace(route.Primary.ModelID) != ""
}
