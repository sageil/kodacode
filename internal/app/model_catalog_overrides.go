package app

import (
	"context"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

type overrideModelCatalog struct {
	base                modelCatalog
	overridesByProvider map[string][]ModelOverrideConfig
}

func wrapModelCatalogOverrides(base modelCatalog, overrides []ModelOverrideConfig) modelCatalog {
	if len(overrides) == 0 {
		return base
	}
	byProvider := make(map[string][]ModelOverrideConfig, len(overrides))
	for _, override := range overrides {
		providerID := strings.TrimSpace(override.Ref.ProviderID)
		if providerID == "" {
			continue
		}
		byProvider[providerID] = append(byProvider[providerID], override)
	}
	if len(byProvider) == 0 {
		return base
	}
	return &overrideModelCatalog{
		base:                base,
		overridesByProvider: byProvider,
	}
}

func (c *overrideModelCatalog) ProviderName(providerID string) string {
	if c == nil || c.base == nil {
		return ""
	}
	return c.base.ProviderName(providerID)
}

func (c *overrideModelCatalog) ModelsForProvider(providerID string) []provider.CatalogModel {
	if c == nil {
		return nil
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil
	}

	var models []provider.CatalogModel
	if c.base != nil {
		models = append(models, c.base.ModelsForProvider(providerID)...)
	}
	overrides := c.overridesByProvider[providerID]
	if len(overrides) == 0 {
		return models
	}

	indexByID := make(map[string]int, len(models))
	for index, model := range models {
		indexByID[strings.TrimSpace(model.ID)] = index
	}
	for _, override := range overrides {
		modelID := strings.TrimSpace(override.Ref.ModelID)
		if modelID == "" {
			continue
		}
		index, ok := indexByID[modelID]
		if !ok {
			models = append(models, provider.CatalogModel{
				ID:   modelID,
				Name: firstNonBlank(strings.TrimSpace(override.Name), modelID),
			})
			index = len(models) - 1
			indexByID[modelID] = index
		}
		models[index] = applyModelOverride(models[index], override)
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})
	return models
}

func (c *overrideModelCatalog) EnsureFresh(ctx context.Context) error {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.EnsureFresh(ctx)
}

func (c *overrideModelCatalog) Refresh(ctx context.Context) error {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.Refresh(ctx)
}

func applyModelOverride(model provider.CatalogModel, override ModelOverrideConfig) provider.CatalogModel {
	if name := strings.TrimSpace(override.Name); name != "" {
		model.Name = name
	}
	if override.ContextSize != nil {
		model.ContextSize = *override.ContextSize
	}
	if override.MaxInputTokens != nil {
		model.MaxInputTokens = *override.MaxInputTokens
	}
	if override.MaxOutputTokens != nil {
		model.MaxOutputTokens = *override.MaxOutputTokens
	}
	if override.Reasoning != nil {
		model.Reasoning = *override.Reasoning
	}
	if override.ToolCalls != nil {
		model.ToolCalls = *override.ToolCalls
		model.ToolCallsKnown = true
	}
	if override.Vision != nil {
		model.Vision = *override.Vision
		model.VisionKnown = true
	}
	if override.CostInput != nil {
		model.CostInput = *override.CostInput
	}
	if override.CostOutput != nil {
		model.CostOutput = *override.CostOutput
	}
	model.ID = strings.TrimSpace(model.ID)
	model.Name = firstNonBlank(strings.TrimSpace(model.Name), model.ID)
	return model
}
