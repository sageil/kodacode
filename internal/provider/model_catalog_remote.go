package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (c *ModelCatalog) refreshCloudProviders(
	ctx context.Context,
	current map[string]catalogProvider,
	currentSources map[string]catalogRemoteSource,
) (map[string]catalogProvider, map[string]catalogRemoteSource, error) {
	enrichment := make(map[string]catalogProvider)
	if c.fetchEnrichment != nil {
		if fetched, err := c.fetchEnrichment(ctx); err != nil {
			c.reportNonFatalError("model catalog: enrichment refresh failed", err)
		} else if fetched != nil {
			enrichment = fetched
		}
	}

	refreshed := make(map[string]catalogProvider, len(c.remoteProviders))
	refreshedSources := make(map[string]catalogRemoteSource, len(c.remoteProviders))
	var errs []error
	for _, providerEntry := range c.remoteProviders {
		refreshedProvider, err := c.refreshRemoteCatalogProvider(ctx, providerEntry, current, currentSources, enrichment)
		if err != nil {
			errs = append(errs, err)
		}
		if refreshedProvider != nil {
			refreshed[providerEntry.ID] = *refreshedProvider
			refreshedSources[providerEntry.ID] = catalogRemoteSourceFromProvider(providerEntry)
		}
	}
	return refreshed, refreshedSources, errors.Join(errs...)
}

func (c *ModelCatalog) refreshRemoteCatalogProvider(
	ctx context.Context,
	source RemoteModelCatalogProvider,
	current map[string]catalogProvider,
	currentSources map[string]catalogRemoteSource,
	enrichment map[string]catalogProvider,
) (*catalogProvider, error) {
	if source.Kind == RemoteModelCatalogProviderModelsDev {
		refreshed, err := modelsDevCatalogProvider(source, enrichment)
		if refreshed == nil && err != nil {
			if existing, ok := current[strings.TrimSpace(source.ID)]; ok && remoteProviderSourceMatches(currentSources, source) {
				cloned := cloneProvider(existing)
				return &cloned, err
			}
			return nil, err
		}
		return refreshed, err
	}
	if c.fetchRemote == nil {
		return nil, nil
	}
	refreshed, err := c.fetchRemote(ctx, source)
	if refreshed == nil {
		if err != nil {
			if existing, ok := current[strings.TrimSpace(source.ID)]; ok && remoteProviderSourceMatches(currentSources, source) {
				cloned := cloneProvider(existing)
				return &cloned, err
			}
			return nil, err
		}
		return nil, nil
	}
	merged := mergeCatalogProviderWithEnrichment(*refreshed, providerEnrichment(enrichment, source.ID))
	return &merged, err
}

func (c *ModelCatalog) fetchRemoteProvider(ctx context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
	switch source.Kind {
	case RemoteModelCatalogProviderOpenAI:
		return fetchOpenAIModelCatalogProvider(ctx, source)
	case RemoteModelCatalogProviderAnthropic:
		return fetchAnthropicModelCatalogProvider(ctx, source)
	case RemoteModelCatalogProviderGoogle:
		return fetchGoogleModelCatalogProvider(ctx, source)
	case RemoteModelCatalogProviderOpenAICompatible:
		return fetchOpenAICompatibleModelCatalogProvider(ctx, source)
	case RemoteModelCatalogProviderGitHubCopilot:
		return fetchGitHubCopilotModelCatalogProvider(ctx, source)
	case RemoteModelCatalogProviderModelsDev:
		return nil, fmt.Errorf("%s: models.dev-backed providers are resolved from enrichment", source.ID)
	default:
		return nil, fmt.Errorf("%s: unsupported remote provider kind %q", source.ID, source.Kind)
	}
}

func (c *ModelCatalog) fetchModelsDevData(ctx context.Context) (map[string]catalogProvider, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultModelCatalogTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsDevURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev: HTTP %d", resp.StatusCode)
	}

	var payload map[string]modelsDevProvider
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	providers := make(map[string]catalogProvider, len(payload))
	for providerID, providerEntry := range payload {
		catalogProvider := catalogProvider{
			ID:     firstNonBlank(providerEntry.ID, providerID),
			Name:   firstNonBlank(providerEntry.Name, providerID),
			Models: make(map[string]CatalogModel, len(providerEntry.Models)),
		}
		for modelID, model := range providerEntry.Models {
			catalogModel := CatalogModel{
				ID:                 firstNonBlank(model.ID, modelID),
				Name:               firstNonBlank(model.Name, modelID),
				Family:             model.Family,
				ContextSize:        model.Limit.Context,
				MaxInputTokens:     model.Limit.Input,
				MaxOutputTokens:    model.Limit.Output,
				Reasoning:          model.Reasoning,
				ReasoningKnown:     true,
				ToolCalls:          model.ToolCall,
				ToolCallsKnown:     true,
				Vision:             modelHasVision(model),
				VisionKnown:        true,
				SupportedEndpoints: cloneStrings(model.SupportedEndpoints),
			}
			if model.Cost.Input != nil {
				catalogModel.CostInput = *model.Cost.Input
				catalogModel.CostInputKnown = true
			}
			if model.Cost.Output != nil {
				catalogModel.CostOutput = *model.Cost.Output
				catalogModel.CostOutputKnown = true
			}
			if model.Cost.CacheRead != nil {
				catalogModel.CostCacheRead = *model.Cost.CacheRead
				catalogModel.CostCacheReadKnown = true
			}
			if model.Cost.CacheWrite != nil {
				catalogModel.CostCacheWrite = *model.Cost.CacheWrite
				catalogModel.CostCacheWriteKnown = true
			}
			if model.Modalities != nil {
				catalogModel.InputModalities = cloneStrings(model.Modalities.Input)
				catalogModel.OutputModalities = cloneStrings(model.Modalities.Output)
			}
			catalogProvider.Models[catalogModel.ID] = catalogModel
		}
		providers[catalogProvider.ID] = catalogProvider
	}
	return providers, nil
}
