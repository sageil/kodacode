package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func (m *Model) refreshDialogState() error {
	if m == nil {
		return nil
	}
	if m.backend == nil || m.ctx == nil {
		m.providerCatalog.models = nil
		return nil
	}
	state, err := m.backend.DialogState(m.ctx)
	if err != nil {
		return err
	}
	m.cacheDialogState(state)
	return nil
}

func (m *Model) cacheAvailableModels(models []app.AvailableModel) {
	if m == nil {
		return
	}
	if len(models) == 0 {
		m.providerCatalog.models = nil
		return
	}
	cached := make(map[string]app.AvailableModel, len(models))
	for _, model := range models {
		key := strings.TrimSpace(model.Ref.String())
		if key == "" {
			continue
		}
		cached[key] = model
	}
	if len(cached) == 0 {
		m.providerCatalog.models = nil
		return
	}
	m.providerCatalog.models = cached
}

func (m *Model) materializeCurrentAvailableModels() {
	if m == nil || m.projector == nil {
		return
	}
	state := m.projector.CurrentState()
	m.materializeAvailableModelRef(refFromEffectiveSelectedAgentModel(*m, state))
	m.materializeAvailableModelRef(refFromRunningStatusTurnModel(*m, state))
}

func refFromEffectiveSelectedAgentModel(m Model, state events.SessionState) provider.ModelRef {
	ref, ok := effectiveSelectedAgentModelRef(m, state)
	if !ok {
		return provider.ModelRef{}
	}
	return ref
}

func refFromRunningStatusTurnModel(m Model, state events.SessionState) provider.ModelRef {
	ref, _, ok := runningStatusTurnModelRef(m, state)
	if !ok {
		return provider.ModelRef{}
	}
	return ref
}

func (m *Model) materializeAvailableModelRef(ref provider.ModelRef) {
	if m == nil {
		return
	}
	key := strings.TrimSpace(ref.String())
	if key == "" {
		return
	}
	if m.providerCatalog.models != nil {
		if _, ok := m.providerCatalog.models[key]; ok {
			return
		}
	} else {
		m.providerCatalog.models = make(map[string]app.AvailableModel)
	}
	m.providerCatalog.models[key] = app.AvailableModel{
		Ref:          ref,
		ProviderName: providerDisplayName(ref.ProviderID),
		ModelName:    strings.TrimSpace(ref.ModelID),
	}
}
