package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

type composerAvailability int

const (
	composerAvailabilityReady composerAvailability = iota
	composerAvailabilityNeedsProvider
	composerAvailabilityNeedsModel
)

func (m *Model) cacheDialogState(state app.DialogState) {
	if m == nil {
		return
	}
	m.providerCatalog.defaultModelRoute = state.ModelRoute
	m.providerCatalog.utilityModel = state.UtilityModel
	m.cacheConnectedProviders(state.ConnectedProviders)
	m.cacheAvailableModels(state.AvailableModels)
	m.materializeCurrentAvailableModels()
}

func (m *Model) cacheConnectedProviders(providers []app.ConnectedProvider) {
	if m == nil {
		return
	}
	m.providerCatalog.providerConfigKnown = true
	if len(providers) == 0 {
		m.providerCatalog.connectedProviders = nil
		return
	}
	cached := make(map[string]app.ConnectedProvider, len(providers))
	for _, configured := range providers {
		key := connectedProviderKey(configured.ProviderID)
		if key == "" {
			continue
		}
		cached[key] = configured
	}
	if len(cached) == 0 {
		m.providerCatalog.connectedProviders = nil
		return
	}
	m.providerCatalog.connectedProviders = cached
}

func connectedProviderKey(providerID string) string {
	trimmed := strings.TrimSpace(providerID)
	if trimmed == "" {
		return ""
	}
	canonical := strings.TrimSpace(provider.CanonicalProviderID(trimmed))
	if canonical != "" {
		return canonical
	}
	return trimmed
}

func (m Model) composerAvailabilityForState(state events.SessionState) composerAvailability {
	if m.providerCatalog.providerConfigKnown && len(m.providerCatalog.connectedProviders) == 0 {
		return composerAvailabilityNeedsProvider
	}
	ref, ok := effectiveSelectedAgentModelRef(m, state)
	if !ok || strings.TrimSpace(ref.ProviderID) == "" || strings.TrimSpace(ref.ModelID) == "" {
		return composerAvailabilityNeedsModel
	}
	return composerAvailabilityReady
}

func (m Model) composerInputEnabledForState(state events.SessionState) bool {
	return m.composerAvailabilityForState(state) == composerAvailabilityReady
}

func (m Model) composerInputEnabled() bool {
	return m.composerInputEnabledForState(m.projector.CurrentState())
}

func (m Model) composerDisabledMessage(state events.SessionState) string {
	switch m.composerAvailabilityForState(state) {
	case composerAvailabilityNeedsProvider:
		return "Connect a provider to enable the composer. Press ctrl+p and choose Connect provider."
	case composerAvailabilityNeedsModel:
		return "Select a model to enable the composer. Press ctrl+p and choose Model."
	default:
		return ""
	}
}

func (m Model) openComposerSetupDialog() tea.Cmd {
	switch m.composerAvailabilityForState(m.projector.CurrentState()) {
	case composerAvailabilityNeedsProvider:
		return m.openConnectDialog()
	case composerAvailabilityNeedsModel:
		return m.openModelDialog()
	default:
		return nil
	}
}

func (m Model) shouldAutoOpenProviderDialog() bool {
	return m.dialog == nil && m.composerAvailabilityForState(m.projector.CurrentState()) == composerAvailabilityNeedsProvider
}
