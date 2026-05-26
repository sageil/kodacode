package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (c *ModelCatalog) shouldRefreshCloud(force bool) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if force {
		return true
	}
	if !c.lastCloudAttempt.IsZero() && time.Since(c.lastCloudAttempt) < modelCatalogRetryInterval {
		return false
	}
	return c.needsCloudRefresh || c.isStale()
}

func (c *ModelCatalog) refreshCloudLocked(ctx context.Context, force bool) error {
	if !force && !c.shouldRefreshCloud(false) {
		return nil
	}

	c.mu.Lock()
	c.lastCloudAttempt = time.Now()
	currentAll := cloneProviders(c.providers)
	currentRemoteSources := cloneCatalogRemoteSources(c.remoteSources)
	c.mu.Unlock()

	refreshed, refreshedSources, err := c.refreshCloudProviders(ctx, filterLocalProviders(currentAll, c.localProviders), currentRemoteSources)

	c.mu.Lock()
	c.providers = mergeCloudAndLocalProviders(refreshed, currentAll, c.localProviders)
	c.remoteSources = refreshedSources
	c.needsCloudRefresh = err != nil
	c.mu.Unlock()

	c.saveToDisk(filterLocalProviders(refreshed, c.localProviders), refreshedSources)
	return err
}

func (c *ModelCatalog) refreshLocal(ctx context.Context) {
	if err := c.refreshLocalLocked(ctx); err != nil {
		c.reportNonFatalError("model catalog: local refresh failed", err)
	}
}

func (c *ModelCatalog) refreshLocalLocked(ctx context.Context) error {
	if len(c.localProviders) == 0 {
		return nil
	}
	var errs []error
	for _, endpoint := range c.localProviders {
		models, err := c.fetchLocal(ctx, endpoint)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		c.mergeLocalProvider(endpoint, models)
	}
	return errors.Join(errs...)
}

func (c *ModelCatalog) mergeLocalProvider(endpoint LocalModelCatalogProvider, models []CatalogModel) {
	c.mu.Lock()
	defer c.mu.Unlock()

	existing := c.providers[endpoint.ID]
	if existing.Models == nil {
		existing.Models = make(map[string]CatalogModel)
	}
	existing.ID = endpoint.ID
	existing.Name = firstNonBlank(existing.Name, endpoint.Name, endpoint.ID)

	currentIDs := make(map[string]struct{}, len(models))
	for _, model := range models {
		currentIDs[model.ID] = struct{}{}
		if cached, ok := existing.Models[model.ID]; ok {
			existing.Models[model.ID] = mergeCatalogModels(cached, model)
			continue
		}
		existing.Models[model.ID] = model
	}
	for modelID := range existing.Models {
		if _, ok := currentIDs[modelID]; !ok {
			delete(existing.Models, modelID)
		}
	}
	c.providers[endpoint.ID] = existing
}

func remoteProviderCacheMismatch(
	cached map[string]catalogProvider,
	cachedSources map[string]catalogRemoteSource,
	providers []RemoteModelCatalogProvider,
) bool {
	if len(providers) == 0 {
		return false
	}
	for _, providerEntry := range providers {
		providerID := strings.TrimSpace(providerEntry.ID)
		if providerID == "" {
			continue
		}
		if _, ok := cached[providerID]; !ok {
			return true
		}
		if !remoteProviderSourceMatches(cachedSources, providerEntry) {
			return true
		}
	}
	return false
}

func filterConfiguredRemoteCache(
	cached map[string]catalogProvider,
	cachedSources map[string]catalogRemoteSource,
	providers []RemoteModelCatalogProvider,
) (map[string]catalogProvider, map[string]catalogRemoteSource) {
	if len(cached) == 0 || len(providers) == 0 {
		return cached, cachedSources
	}
	filteredProviders := make(map[string]catalogProvider, len(providers))
	filteredSources := make(map[string]catalogRemoteSource, len(providers))
	for _, providerEntry := range providers {
		providerID := strings.TrimSpace(providerEntry.ID)
		if providerID == "" || !remoteProviderSourceMatches(cachedSources, providerEntry) {
			continue
		}
		existing, ok := cached[providerID]
		if !ok {
			continue
		}
		filteredProviders[providerID] = cloneProvider(existing)
		if source, ok := cachedSources[providerID]; ok {
			filteredSources[providerID] = source
		}
	}
	return filteredProviders, filteredSources
}

func remoteProviderSourceMatches(cached map[string]catalogRemoteSource, providerEntry RemoteModelCatalogProvider) bool {
	source, ok := cached[strings.TrimSpace(providerEntry.ID)]
	if !ok {
		return false
	}
	return source == catalogRemoteSourceFromProvider(providerEntry)
}

func catalogRemoteSourceFromProvider(providerEntry RemoteModelCatalogProvider) catalogRemoteSource {
	baseURL := modelCatalogRoot(strings.TrimSpace(providerEntry.BaseURL))
	if providerEntry.Kind == RemoteModelCatalogProviderModelsDev {
		baseURL = ""
	}
	return catalogRemoteSource{
		Kind:    providerEntry.Kind,
		BaseURL: baseURL,
	}
}

func providerEnrichment(enrichment map[string]catalogProvider, providerID string) catalogProvider {
	for _, key := range catalogEnrichmentProviderKeys(providerID) {
		if providerEntry, ok := enrichment[key]; ok {
			return cloneProvider(providerEntry)
		}
	}
	return catalogProvider{}
}

func catalogEnrichmentProviderKeys(providerID string) []string {
	keys := catalogProviderKeys(providerID)
	canonical := CanonicalProviderID(providerID)
	if strings.TrimSpace(canonical) != "" && !strings.EqualFold(strings.TrimSpace(canonical), strings.TrimSpace(providerID)) {
		keys = append(keys, catalogProviderKeys(canonical)...)
	}
	var filtered []string
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, key)
	}
	return filtered
}

func mergeCatalogProviderWithEnrichment(primary, enrichment catalogProvider) catalogProvider {
	merged := cloneProvider(primary)
	merged.ID = firstNonBlank(merged.ID, enrichment.ID)
	merged.Name = firstNonBlank(merged.Name, enrichment.Name, merged.ID)
	if merged.Models == nil {
		merged.Models = make(map[string]CatalogModel)
	}
	for modelID, model := range merged.Models {
		if enriched, ok := enrichedCatalogModelForID(merged.ID, modelID, enrichment); ok {
			merged.Models[modelID] = mergeCatalogModels(model, enriched)
		}
	}
	return merged
}

func enrichedCatalogModelForID(providerID, modelID string, enrichment catalogProvider) (CatalogModel, bool) {
	for _, key := range catalogModelKeys(providerID, modelID) {
		if enriched, ok := enrichment.Models[key]; ok {
			return enriched, true
		}
	}
	return CatalogModel{}, false
}

func modelsDevCatalogProvider(source RemoteModelCatalogProvider, enrichment map[string]catalogProvider) (*catalogProvider, error) {
	providerEntry := providerEnrichment(enrichment, source.ID)
	if strings.TrimSpace(providerEntry.ID) == "" {
		return nil, fmt.Errorf("%s: models.dev provider not found", source.ID)
	}
	providerEntry.ID = strings.TrimSpace(source.ID)
	providerEntry.Name = firstNonBlank(strings.TrimSpace(source.Name), strings.TrimSpace(providerEntry.Name), strings.TrimSpace(source.ID))
	return &providerEntry, nil
}
