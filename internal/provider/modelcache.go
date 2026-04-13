package provider

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
)

type ModelCache struct {
	mu              sync.RWMutex
	refreshMu       sync.Mutex
	providers       map[string]modelsDevProvider
	locals          []LocalProviderEndpoint
	refreshInterval time.Duration
	cacheFile       string
	oauthProviders  map[string]bool // providers using OAuth (for model filtering)
	fetchCloud      func(context.Context) map[string]modelsDevProvider
	fetchCopilot    func(context.Context, string) ([]CopilotModel, error)
	fetchLocal      func(context.Context, LocalProviderEndpoint) []modelsDevModel
	networkCheck    func() bool
	copilotToken    func() string
}

// NewModelCache creates a ModelCache with the given refresh interval in days.
func NewModelCache(refreshIntervalDays int) *ModelCache {
	mc := &ModelCache{
		refreshInterval: time.Duration(refreshIntervalDays) * 24 * time.Hour,
		cacheFile:       filepath.Join(dataDir(), "models-cache.json"),
	}
	mc.fetchCloud = mc.fetchModelsDevData
	mc.fetchCopilot = FetchCopilotModels
	mc.fetchLocal = mc.fetchLocalModels
	mc.networkCheck = hasNetwork
	return mc
}

// SetOAuthProvider marks a provider as using OAuth authentication.
// This enables model filtering for providers that only support a subset
// of models via OAuth (e.g. OpenAI codex endpoint).
func (mc *ModelCache) SetOAuthProvider(providerID string) {
	if mc.oauthProviders == nil {
		mc.oauthProviders = make(map[string]bool)
	}
	mc.oauthProviders[providerID] = true
}

func (mc *ModelCache) SetCopilotTokenProvider(fn func() string) {
	mc.copilotToken = fn
}

// RegisterLocal adds a local provider endpoint for model discovery.
func (mc *ModelCache) RegisterLocal(ep LocalProviderEndpoint) {
	mc.mu.Lock()
	for i := range mc.locals {
		if mc.locals[i].ID == ep.ID {
			mc.locals[i] = ep
			mc.mu.Unlock()
			return
		}
	}
	mc.locals = append(mc.locals, ep)
	mc.mu.Unlock()
}

// dataDir returns the platform-appropriate data directory for kodacode.
func dataDir() string {
	return config.DataDir()
}

// Init loads the cache from disk synchronously. If the refresh interval has
// lapsed and a network connection is available, a background refresh is
// kicked off. Otherwise no network calls are made, and the next startup will
// check again.
func (mc *ModelCache) Init(ctx context.Context) {
	mc.mu.Lock()

	cached, needsRefresh := mc.loadFromDisk()
	if cached == nil {
		cached = make(map[string]modelsDevProvider)
	}
	locals := append([]LocalProviderEndpoint(nil), mc.locals...)
	cached = filterLocalProviders(cached, locals)
	markCloudModelMetadataKnown(cached)
	mc.providers = cached

	stale := mc.isStale() || needsRefresh
	mc.mu.Unlock()

	if len(locals) > 0 {
		mc.refreshLocal(ctx, locals)
	}

	if stale && mc.networkAvailable() {
		if len(cached) == 0 {
			mc.refreshCloud(ctx)
		} else {
			go mc.refreshCloud(ctx)
		}
	}
}

// isStale reports whether the cache file is older than the refresh interval
// or missing entirely. Caller must hold mc.mu (read or write).
func (mc *ModelCache) isStale() bool {
	if mc.refreshInterval <= 0 {
		return false
	}
	info, err := os.Stat(mc.cacheFile)
	if err != nil {
		return true // missing or unreadable
	}
	return time.Since(info.ModTime()) > mc.refreshInterval
}

// hasNetwork does a quick TCP dial to check for internet connectivity.
func hasNetwork() bool {
	conn, err := net.DialTimeout("tcp", "1.1.1.1:443", 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Refresh forces a re-fetch from all sources, updating the cache.
func (mc *ModelCache) Refresh(ctx context.Context) {
	mc.refreshMu.Lock()
	defer mc.refreshMu.Unlock()

	mc.mu.RLock()
	current := cloneProviders(mc.providers)
	locals := append([]LocalProviderEndpoint(nil), mc.locals...)
	mc.mu.RUnlock()

	refreshed := mc.refreshFromSources(ctx, current, locals)

	mc.mu.Lock()
	mc.providers = refreshed
	mc.mu.Unlock()

	mc.saveToDisk(filterLocalProviders(refreshed, locals))
}

func (mc *ModelCache) refreshCloud(ctx context.Context) {
	mc.refreshMu.Lock()
	defer mc.refreshMu.Unlock()

	mc.mu.RLock()
	currentAll := cloneProviders(mc.providers)
	locals := append([]LocalProviderEndpoint(nil), mc.locals...)
	mc.mu.RUnlock()

	current := filterLocalProviders(currentAll, locals)
	refreshed := mc.refreshCloudProviders(ctx, current)

	mc.mu.Lock()
	if mc.providers == nil {
		mc.providers = make(map[string]modelsDevProvider)
	}
	for id := range mc.providers {
		delete(mc.providers, id)
	}
	for providerID, providerEntry := range refreshed {
		mc.providers[providerID] = providerEntry
	}
	for _, ep := range locals {
		if existing, ok := currentAll[ep.ID]; ok {
			mc.providers[ep.ID] = existing
		}
	}
	mc.mu.Unlock()

	mc.saveToDisk(filterLocalProviders(refreshed, locals))
}

func (mc *ModelCache) refreshLocal(ctx context.Context, locals []LocalProviderEndpoint) {
	for _, ep := range locals {
		localModels := mc.fetchLocal(ctx, ep)
		if localModels == nil {
			continue
		}
		mc.mu.Lock()
		existing := mc.providers[ep.ID]
		if existing.Models == nil {
			existing.Models = make(map[string]modelsDevModel)
		}
		existing.ID = ep.ID
		if existing.Name == "" {
			existing.Name = ep.Name
		}
		for _, m := range localModels {
			if cached, ok := existing.Models[m.ID]; ok {
				existing.Models[m.ID] = mergeLocalDiscoveredModel(cached, m)
			} else {
				existing.Models[m.ID] = m
			}
		}
		localIDs := make(map[string]bool, len(localModels))
		for _, m := range localModels {
			localIDs[m.ID] = true
		}
		for id := range existing.Models {
			if !localIDs[id] {
				delete(existing.Models, id)
			}
		}
		mc.providers[ep.ID] = existing
		mc.mu.Unlock()
	}
}

func (mc *ModelCache) refreshFromSources(ctx context.Context, current map[string]modelsDevProvider, locals []LocalProviderEndpoint) map[string]modelsDevProvider {
	cloud := mc.refreshCloudProviders(ctx, filterLocalProviders(current, locals))

	for _, ep := range locals {
		localModels := mc.fetchLocal(ctx, ep)
		if localModels == nil {
			// Keep existing cache entry for this local provider if fetch fails.
			if existing, ok := current[ep.ID]; ok {
				cloud[ep.ID] = existing
			}
			continue
		}
		// Start with models.dev metadata for this provider (if any),
		// then add/update with locally discovered models.
		existing := cloud[ep.ID]
		if existing.Models == nil {
			existing.Models = make(map[string]modelsDevModel)
		}
		existing.ID = ep.ID
		if existing.Name == "" {
			existing.Name = ep.Name
		}
		// Add locally discovered models, enriching with existing metadata.
		for _, m := range localModels {
			if cached, ok := existing.Models[m.ID]; ok {
				existing.Models[m.ID] = mergeLocalDiscoveredModel(cached, m)
			} else {
				existing.Models[m.ID] = m
			}
		}
		// Remove models from cache that are no longer loaded locally.
		localIDs := make(map[string]bool, len(localModels))
		for _, m := range localModels {
			localIDs[m.ID] = true
		}
		for id := range existing.Models {
			if !localIDs[id] {
				delete(existing.Models, id)
			}
		}
		cloud[ep.ID] = existing
	}
	return cloud
}

func (mc *ModelCache) refreshCloudProviders(ctx context.Context, current map[string]modelsDevProvider) map[string]modelsDevProvider {
	cloud := mc.fetchCloud(ctx)
	markCloudModelMetadataKnown(cloud)
	if cloud == nil && current != nil {
		cloud = cloneProviders(current)
	}
	if cloud == nil {
		cloud = make(map[string]modelsDevProvider)
	}
	if copilot := mc.refreshCopilotProvider(ctx, cloud["github-copilot"]); copilot != nil {
		cloud["github-copilot"] = *copilot
	}
	return cloud
}

// ModelsForProvider returns the models for a given provider ID from the cache.
// Alias models (e.g. "claude-opus-4-5" pointing to "claude-opus-4-5-20251101")
// are deduplicated: when a dated version exists, the undated alias is excluded.
func (mc *ModelCache) ModelsForProvider(providerID string) []Model {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	p, ok := mc.providers[providerID]
	if !ok {
		return nil
	}

	allIDs := make(map[string]bool, len(p.Models))
	for id := range p.Models {
		allIDs[id] = true
	}

	models := make([]Model, 0, len(p.Models))
	for _, m := range p.Models {
		// Skip "*-latest" aliases (e.g. "claude-3-7-sonnet-latest").
		if strings.HasSuffix(m.ID, "-latest") {
			continue
		}
		// Skip undated alias when a dated version exists.
		// e.g. skip "claude-opus-4-5" if "claude-opus-4-5-YYYYMMDD" is present.
		if isUndatedAlias(m.ID, allIDs) {
			continue
		}
		// Google: filter to Gemini chat models only. models.dev includes
		// embedding, TTS, image, live, dated previews, and other non-chat models.
		if providerID == "google" && !isGeminiChatModel(m.ID) {
			continue
		}
		// OpenAI OAuth: filter to codex-compatible models only.
		if providerID == "openai" && mc.oauthProviders["openai"] && !isOpenAICodexModel(m.ID) {
			continue
		}

		name := m.Name
		if name == "" {
			name = m.ID
		}
		// Strip "(latest)" suffix from display names since we keep the dated version.
		name = strings.TrimSuffix(name, " (latest)")
		name = strings.TrimSpace(name)

		vision := cachedModelVision(m)
		var inputModalities, outputModalities []string
		if m.Modalities != nil {
			inputModalities = cloneStrings(m.Modalities.Input)
			outputModalities = cloneStrings(m.Modalities.Output)
		}
		models = append(models, Model{
			ID:                 m.ID,
			Name:               name,
			ContextSize:        m.Limit.Context,
			MaxInputTokens:     m.Limit.Input,
			Reasoning:          m.Reasoning,
			ToolCall:           m.ToolCall,
			ToolCallKnown:      m.ToolCallKnown,
			Attachment:         m.Attachment,
			AttachmentKnown:    m.AttachmentKnown,
			Vision:             vision,
			VisionKnown:        m.VisionKnown,
			CostInput:          m.Cost.Input,
			CostOutput:         m.Cost.Output,
			CostCacheRead:      m.Cost.CacheRead,
			CostCacheWrite:     m.Cost.CacheWrite,
			CostReasoning:      m.Cost.Output, // reasoning billed at output rate
			Family:             m.Family,
			InputModalities:    inputModalities,
			OutputModalities:   outputModalities,
			SupportedEndpoints: cloneStrings(m.SupportedEndpoints),
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models
}

// isUndatedAlias returns true if modelID is an undated alias (e.g. "claude-opus-4-5")
// and a dated version (e.g. "claude-opus-4-5-20251101") exists in allIDs.
func isUndatedAlias(modelID string, allIDs map[string]bool) bool {
	for id := range allIDs {
		if id != modelID && strings.HasPrefix(id, modelID+"-") {
			suffix := id[len(modelID)+1:]
			// Check if suffix looks like a date (8 digits).
			if len(suffix) == 8 && isDigits(suffix) {
				return true
			}
		}
	}
	return false
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// openaiCodexModels is the set of models supported by the ChatGPT codex
// endpoint when using OAuth (ChatGPT Pro/Plus subscription).
var openaiCodexModels = map[string]bool{
	"gpt-5.1-codex":      true,
	"gpt-5.1-codex-max":  true,
	"gpt-5.1-codex-mini": true,
	"gpt-5.2":            true,
	"gpt-5.2-codex":      true,
	"gpt-5.3-codex":      true,
	"gpt-5.4":            true,
	"gpt-5.4-mini":       true,
}

// isOpenAICodexModel returns true if the model ID is supported by the codex endpoint.
// Any model with "codex" in its name is also allowed.
func isOpenAICodexModel(id string) bool {
	if openaiCodexModels[id] {
		return true
	}
	return strings.Contains(id, "codex")
}

// loadFromDisk reads the cache file and returns the provider map.
// needsRefresh is true when the cache schema is outdated (version mismatch
// or legacy format). The data is still returned so it can serve as a
// fallback if the models.dev fetch fails.
func (mc *ModelCache) loadFromDisk() (providers map[string]modelsDevProvider, needsRefresh bool) {
	data, err := os.ReadFile(mc.cacheFile)
	if err != nil {
		return nil, true
	}

	// Try versioned envelope first.
	var env cacheEnvelope
	if err := json.Unmarshal(data, &env); err == nil && env.Version > 0 {
		if env.Version != cacheVersion {
			log.Printf("modelcache: cache version %d != %d, will re-fetch", env.Version, cacheVersion)
			return env.Providers, true
		}
		return env.Providers, false
	}

	// Legacy format (unversioned): parse as raw provider map, flag for refresh.
	var legacy map[string]modelsDevProvider
	if err := json.Unmarshal(data, &legacy); err == nil {
		log.Printf("modelcache: legacy cache format, will re-fetch")
		return legacy, true
	}

	return nil, true
}

func (mc *ModelCache) saveToDisk(providers map[string]modelsDevProvider) {
	dir := filepath.Dir(mc.cacheFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("modelcache: failed to create cache dir: %v", err)
		return
	}
	env := cacheEnvelope{Version: cacheVersion, Providers: providers}
	data, err := json.Marshal(env)
	if err != nil {
		log.Printf("modelcache: failed to marshal cache: %v", err)
		return
	}
	if err := os.WriteFile(mc.cacheFile, data, 0o644); err != nil {
		log.Printf("modelcache: failed to write cache: %v", err)
	}
}

func (mc *ModelCache) networkAvailable() bool {
	if mc.networkCheck != nil {
		return mc.networkCheck()
	}
	return hasNetwork()
}

func cloneProviders(src map[string]modelsDevProvider) map[string]modelsDevProvider {
	if src == nil {
		return nil
	}
	dst := make(map[string]modelsDevProvider, len(src))
	for id, provider := range src {
		cloned := provider
		if provider.Models != nil {
			cloned.Models = make(map[string]modelsDevModel, len(provider.Models))
			for modelID, model := range provider.Models {
				if model.Modalities != nil {
					model.Modalities = &modelsDevModality{
						Input:  cloneStrings(model.Modalities.Input),
						Output: cloneStrings(model.Modalities.Output),
					}
				}
				model.SupportedEndpoints = cloneStrings(model.SupportedEndpoints)
				cloned.Models[modelID] = model
			}
		}
		dst[id] = cloned
	}
	return dst
}

func filterLocalProviders(src map[string]modelsDevProvider, locals []LocalProviderEndpoint) map[string]modelsDevProvider {
	if src == nil {
		return nil
	}
	if len(locals) == 0 {
		return cloneProviders(src)
	}
	localIDs := make(map[string]bool, len(locals))
	for _, ep := range locals {
		if ep.ID != "" {
			localIDs[ep.ID] = true
		}
	}
	dst := make(map[string]modelsDevProvider, len(src))
	for id, provider := range src {
		if localIDs[id] {
			continue
		}
		cloned := provider
		if provider.Models != nil {
			cloned.Models = make(map[string]modelsDevModel, len(provider.Models))
			for modelID, model := range provider.Models {
				if model.Modalities != nil {
					model.Modalities = &modelsDevModality{
						Input:  cloneStrings(model.Modalities.Input),
						Output: cloneStrings(model.Modalities.Output),
					}
				}
				model.SupportedEndpoints = cloneStrings(model.SupportedEndpoints)
				cloned.Models[modelID] = model
			}
		}
		dst[id] = cloned
	}
	return dst
}

// ProviderName returns the display name for a provider ID from the cache.
func (mc *ModelCache) ProviderName(providerID string) string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	if p, ok := mc.providers[providerID]; ok {
		return p.Name
	}
	return ""
}

// EnrichModel fills in missing metadata from the cache.
func (mc *ModelCache) EnrichModel(providerID string, m *Model) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	p, ok := mc.providers[providerID]
	if !ok {
		return
	}
	cached, ok := p.Models[m.ID]
	if !ok {
		return
	}
	if m.ContextSize == 0 && cached.Limit.Context > 0 {
		m.ContextSize = cached.Limit.Context
	}
	if m.MaxInputTokens == 0 && cached.Limit.Input > 0 {
		m.MaxInputTokens = cached.Limit.Input
	}
	if m.Name == "" || m.Name == m.ID {
		if cached.Name != "" {
			m.Name = cached.Name
		}
	}
	if m.Family == "" {
		m.Family = cached.Family
	}
	if len(m.InputModalities) == 0 {
		if cached.Modalities != nil {
			m.InputModalities = cloneStrings(cached.Modalities.Input)
		}
	}
	if len(m.OutputModalities) == 0 {
		if cached.Modalities != nil {
			m.OutputModalities = cloneStrings(cached.Modalities.Output)
		}
	}
	if len(m.SupportedEndpoints) == 0 {
		m.SupportedEndpoints = cloneStrings(cached.SupportedEndpoints)
	}
	m.Reasoning = m.Reasoning || cached.Reasoning
	if cached.ToolCallKnown {
		m.ToolCall = m.ToolCall || cached.ToolCall
		m.ToolCallKnown = true
	}
	if cached.AttachmentKnown {
		m.Attachment = m.Attachment || cached.Attachment
		m.AttachmentKnown = true
	}
	if cached.VisionKnown {
		m.Vision = m.Vision || cachedModelVision(cached)
		m.VisionKnown = true
	}
	m.CostInput = cached.Cost.Input
	m.CostOutput = cached.Cost.Output
	m.CostCacheRead = cached.Cost.CacheRead
	m.CostCacheWrite = cached.Cost.CacheWrite
	m.CostReasoning = cached.Cost.Output // reasoning billed at output rate
}

func markCloudModelMetadataKnown(providers map[string]modelsDevProvider) {
	for providerID, providerEntry := range providers {
		if providerEntry.Models == nil {
			continue
		}
		for modelID, model := range providerEntry.Models {
			model.ToolCallKnown = true
			model.AttachmentKnown = true
			model.VisionKnown = true
			providerEntry.Models[modelID] = model
		}
		providers[providerID] = providerEntry
	}
}

func mergeLocalDiscoveredModel(cached, discovered modelsDevModel) modelsDevModel {
	merged := cached
	if merged.Name == "" {
		merged.Name = discovered.Name
	}
	if merged.Limit.Context == 0 && discovered.Limit.Context > 0 {
		merged.Limit.Context = discovered.Limit.Context
	}
	if !merged.ToolCallKnown && discovered.ToolCallKnown {
		merged.ToolCall = discovered.ToolCall
		merged.ToolCallKnown = true
	}
	if !merged.AttachmentKnown && discovered.AttachmentKnown {
		merged.Attachment = discovered.Attachment
		merged.AttachmentKnown = true
	}
	if !merged.VisionKnown && discovered.VisionKnown {
		merged.VisionKnown = true
		if cachedModelVision(discovered) {
			merged.Attachment = true
		}
	}
	return merged
}

func (mc *ModelCache) refreshCopilotProvider(ctx context.Context, modelsDev modelsDevProvider) *modelsDevProvider {
	if mc.copilotToken == nil || mc.fetchCopilot == nil {
		return nil
	}
	token := strings.TrimSpace(mc.copilotToken())
	if token == "" {
		return nil
	}

	models, err := mc.fetchCopilot(ctx, token)
	if err != nil {
		log.Printf("modelcache: copilot fetch failed: %v", err)
		return nil
	}

	providerEntry := modelsDevProvider{
		ID:     "github-copilot",
		Name:   "GitHub Copilot",
		Models: make(map[string]modelsDevModel),
	}
	if modelsDev.Name != "" {
		providerEntry.Name = modelsDev.Name
	}

	for _, model := range models {
		if !model.ModelPickerEnabled {
			continue
		}
		cachedModel := copilotModelToCachedModel(model)
		if existing, ok := modelsDev.Models[cachedModel.ID]; ok {
			cachedModel = mergeCopilotCachedModel(existing, cachedModel)
		}
		providerEntry.Models[cachedModel.ID] = cachedModel
	}
	if len(providerEntry.Models) == 0 {
		return nil
	}
	return &providerEntry
}

func copilotModelToCachedModel(model CopilotModel) modelsDevModel {
	cached := modelsDevModel{
		ID:                 model.ID,
		Name:               model.Name,
		Family:             model.Family,
		Limit:              modelsDevLimit{Context: model.ContextSize, Input: model.MaxInputTokens, Output: model.MaxOutputTokens},
		Reasoning:          model.Reasoning,
		ToolCall:           model.ToolCalls,
		ToolCallKnown:      true,
		VisionKnown:        true,
		SupportedEndpoints: cloneStrings(model.SupportedEndpoints),
	}
	if model.Vision {
		cached.Modalities = &modelsDevModality{Input: []string{"image"}}
	}
	return cached
}

func mergeCopilotCachedModel(modelsDev, copilot modelsDevModel) modelsDevModel {
	merged := copilot
	if merged.Name == "" {
		merged.Name = modelsDev.Name
	}
	if merged.Family == "" {
		merged.Family = modelsDev.Family
	}
	if merged.Limit.Context == 0 {
		merged.Limit.Context = modelsDev.Limit.Context
	}
	if merged.Limit.Input == 0 {
		merged.Limit.Input = modelsDev.Limit.Input
	}
	if merged.Limit.Output == 0 {
		merged.Limit.Output = modelsDev.Limit.Output
	}
	if !merged.Reasoning {
		merged.Reasoning = modelsDev.Reasoning
	}
	if !merged.ToolCallKnown && modelsDev.ToolCallKnown {
		merged.ToolCall = modelsDev.ToolCall
		merged.ToolCallKnown = true
	}
	if !merged.AttachmentKnown && modelsDev.AttachmentKnown {
		merged.Attachment = modelsDev.Attachment
		merged.AttachmentKnown = true
	}
	if !merged.VisionKnown && modelsDev.VisionKnown {
		merged.VisionKnown = true
		if cachedModelVision(modelsDev) {
			merged.Modalities = &modelsDevModality{Input: []string{"image"}}
		}
	}
	if merged.Modalities == nil && modelsDev.Modalities != nil {
		merged.Modalities = &modelsDevModality{
			Input:  cloneStrings(modelsDev.Modalities.Input),
			Output: cloneStrings(modelsDev.Modalities.Output),
		}
	}
	if len(merged.SupportedEndpoints) == 0 {
		merged.SupportedEndpoints = cloneStrings(modelsDev.SupportedEndpoints)
	}
	if merged.Cost == (modelsDevCost{}) {
		merged.Cost = modelsDev.Cost
	}
	return merged
}

func cachedModelVision(model modelsDevModel) bool {
	if model.Attachment {
		return true
	}
	return modelHasImageInput(model)
}

func modelHasImageInput(model modelsDevModel) bool {
	return model.Modalities != nil && slices.Contains(model.Modalities.Input, "image")
}

func init() {
	_ = os.MkdirAll(dataDir(), 0o755)
}
