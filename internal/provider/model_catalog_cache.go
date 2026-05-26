package provider

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (c *ModelCatalog) loadFromDisk() (map[string]catalogProvider, map[string]catalogRemoteSource, bool) {
	if strings.TrimSpace(c.cacheFile) == "" {
		return nil, nil, true
	}
	data, err := os.ReadFile(c.cacheFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, true
	}
	if err != nil {
		c.reportNonFatalError("model catalog: read cache failed", err)
		return nil, nil, true
	}

	var envelope catalogEnvelope
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Version > 0 {
		if envelope.Version != modelCatalogCacheVersion {
			return envelope.Providers, envelope.RemoteSources, true
		}
		return envelope.Providers, envelope.RemoteSources, false
	}
	return nil, nil, true
}

func (c *ModelCatalog) saveToDisk(providers map[string]catalogProvider, remoteSources map[string]catalogRemoteSource) {
	if strings.TrimSpace(c.cacheFile) == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.cacheFile), 0o755); err != nil {
		c.reportNonFatalError("model catalog: create cache dir failed", err)
		return
	}
	data, err := json.Marshal(catalogEnvelope{
		Version:       modelCatalogCacheVersion,
		Providers:     providers,
		RemoteSources: remoteSources,
	})
	if err != nil {
		c.reportNonFatalError("model catalog: marshal cache failed", err)
		return
	}
	if err := os.WriteFile(c.cacheFile, data, 0o644); err != nil {
		c.reportNonFatalError("model catalog: write cache failed", err)
	}
}

func (c *ModelCatalog) reportNonFatalError(message string, err error) {
	if c == nil || err == nil || c.reportError == nil {
		return
	}
	c.reportError(message, err)
}

func (c *ModelCatalog) isStale() bool {
	if c.expiry <= 0 || strings.TrimSpace(c.cacheFile) == "" {
		return false
	}
	info, err := os.Stat(c.cacheFile)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > c.expiry
}

func mergeCloudAndLocalProviders(cloud, current map[string]catalogProvider, locals []LocalModelCatalogProvider) map[string]catalogProvider {
	merged := cloneProviders(cloud)
	if merged == nil {
		merged = make(map[string]catalogProvider)
	}
	for _, endpoint := range locals {
		if existing, ok := current[endpoint.ID]; ok {
			merged[endpoint.ID] = existing
		}
	}
	return merged
}

func filterLocalProviders(src map[string]catalogProvider, locals []LocalModelCatalogProvider) map[string]catalogProvider {
	if src == nil {
		return nil
	}
	if len(locals) == 0 {
		return cloneProviders(src)
	}
	localIDs := make(map[string]struct{}, len(locals))
	for _, endpoint := range locals {
		if strings.TrimSpace(endpoint.ID) != "" {
			localIDs[endpoint.ID] = struct{}{}
		}
	}
	filtered := make(map[string]catalogProvider, len(src))
	for providerID, providerEntry := range src {
		if _, ok := localIDs[providerID]; ok {
			continue
		}
		filtered[providerID] = cloneProvider(providerEntry)
	}
	return filtered
}

func cloneProviders(src map[string]catalogProvider) map[string]catalogProvider {
	if src == nil {
		return nil
	}
	cloned := make(map[string]catalogProvider, len(src))
	for providerID, providerEntry := range src {
		cloned[providerID] = cloneProvider(providerEntry)
	}
	return cloned
}

func cloneCatalogRemoteSources(src map[string]catalogRemoteSource) map[string]catalogRemoteSource {
	if src == nil {
		return nil
	}
	cloned := make(map[string]catalogRemoteSource, len(src))
	for providerID, source := range src {
		cloned[providerID] = source
	}
	return cloned
}

func cloneProvider(providerEntry catalogProvider) catalogProvider {
	cloned := catalogProvider{
		ID:     providerEntry.ID,
		Name:   providerEntry.Name,
		Models: make(map[string]CatalogModel, len(providerEntry.Models)),
	}
	for modelID, model := range providerEntry.Models {
		model.InputModalities = cloneStrings(model.InputModalities)
		model.OutputModalities = cloneStrings(model.OutputModalities)
		model.SupportedEndpoints = cloneStrings(model.SupportedEndpoints)
		model.SupportedReasoningVariants = cloneStrings(model.SupportedReasoningVariants)
		cloned.Models[modelID] = model
	}
	return cloned
}

func mergeCatalogModels(primary, fallback CatalogModel) CatalogModel {
	merged := primary
	if strings.TrimSpace(fallback.Name) != "" {
		merged.Name = fallback.Name
	}
	if strings.TrimSpace(fallback.Family) != "" {
		merged.Family = fallback.Family
	}
	if fallback.ContextSize > 0 {
		merged.ContextSize = fallback.ContextSize
	}
	if fallback.MaxInputTokens > 0 {
		merged.MaxInputTokens = fallback.MaxInputTokens
	}
	if fallback.MaxOutputTokens > 0 {
		merged.MaxOutputTokens = fallback.MaxOutputTokens
	}
	if fallback.ReasoningKnown {
		merged.Reasoning = fallback.Reasoning
		merged.ReasoningKnown = true
	} else if !merged.ReasoningKnown && fallback.Reasoning {
		merged.Reasoning = true
	}
	if len(merged.SupportedReasoningVariants) == 0 {
		merged.SupportedReasoningVariants = cloneStrings(fallback.SupportedReasoningVariants)
	}
	if !merged.SupportsThinkingOutput {
		merged.SupportsThinkingOutput = fallback.SupportsThinkingOutput
	}
	if fallback.ToolCallsKnown {
		merged.ToolCalls = fallback.ToolCalls
		merged.ToolCallsKnown = true
	}
	if fallback.VisionKnown {
		merged.Vision = fallback.Vision
		merged.VisionKnown = true
	}
	if fallback.CostInputKnown {
		merged.CostInput = fallback.CostInput
		merged.CostInputKnown = true
	} else if merged.CostInput == 0 {
		merged.CostInput = fallback.CostInput
	}
	if fallback.CostOutputKnown {
		merged.CostOutput = fallback.CostOutput
		merged.CostOutputKnown = true
	} else if merged.CostOutput == 0 {
		merged.CostOutput = fallback.CostOutput
	}
	if fallback.CostCacheReadKnown {
		merged.CostCacheRead = fallback.CostCacheRead
		merged.CostCacheReadKnown = true
	} else if merged.CostCacheRead == 0 {
		merged.CostCacheRead = fallback.CostCacheRead
	}
	if fallback.CostCacheWriteKnown {
		merged.CostCacheWrite = fallback.CostCacheWrite
		merged.CostCacheWriteKnown = true
	} else if merged.CostCacheWrite == 0 {
		merged.CostCacheWrite = fallback.CostCacheWrite
	}
	if len(fallback.InputModalities) > 0 {
		merged.InputModalities = cloneStrings(fallback.InputModalities)
	}
	if len(fallback.OutputModalities) > 0 {
		merged.OutputModalities = cloneStrings(fallback.OutputModalities)
	}
	if len(fallback.SupportedEndpoints) > 0 {
		merged.SupportedEndpoints = cloneStrings(fallback.SupportedEndpoints)
	}
	return merged
}
