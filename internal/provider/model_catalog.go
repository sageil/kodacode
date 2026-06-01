package provider

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	modelsDevURL                 = "https://models.dev/api.json"
	modelCatalogCacheVersion     = 2
	modelCatalogRetryInterval    = 30 * time.Second
	defaultModelCatalogTimeout   = 5 * time.Second
	localCapabilityProbeParallel = 4
)

type CatalogModel struct {
	ID                         string
	Name                       string
	Family                     string
	ContextSize                int
	MaxInputTokens             int
	MaxOutputTokens            int
	Reasoning                  bool
	ReasoningKnown             bool
	SupportedReasoningVariants []string
	SupportsThinkingOutput     bool
	ToolCalls                  bool
	ToolCallsKnown             bool
	Vision                     bool
	VisionKnown                bool
	CostInput                  float64
	CostInputKnown             bool
	CostOutput                 float64
	CostOutputKnown            bool
	CostCacheRead              float64
	CostCacheReadKnown         bool
	CostCacheWrite             float64
	CostCacheWriteKnown        bool
	InputModalities            []string
	OutputModalities           []string
	SupportedEndpoints         []string
}

type LocalModelCatalogProvider struct {
	ID      string
	Name    string
	BaseURL string
}

type RemoteModelCatalogProviderKind string

const (
	RemoteModelCatalogProviderOpenAI           RemoteModelCatalogProviderKind = "openai"
	RemoteModelCatalogProviderAnthropic        RemoteModelCatalogProviderKind = "anthropic"
	RemoteModelCatalogProviderGoogle           RemoteModelCatalogProviderKind = "google"
	RemoteModelCatalogProviderOpenAICompatible RemoteModelCatalogProviderKind = "openai_compatible"
	RemoteModelCatalogProviderGitHubCopilot    RemoteModelCatalogProviderKind = "github_copilot"
	RemoteModelCatalogProviderModelsDev        RemoteModelCatalogProviderKind = "models_dev"
)

type RemoteModelCatalogProvider struct {
	ID                 string
	Name               string
	Kind               RemoteModelCatalogProviderKind
	BaseURL            string
	APIKey             string
	OAuth              *OpenAIOAuthConfig
	GitHubCopilotOAuth *GitHubCopilotOAuthConfig
	HTTPClient         *http.Client
}

type ModelCatalogConfig struct {
	CacheFile       string
	ExpiryDays      int
	OpenAIOAuth     bool
	OpenAIAPIKey    bool
	RemoteProviders []RemoteModelCatalogProvider
	LocalProviders  []LocalModelCatalogProvider
	ReportError     func(message string, err error)
}

type ModelCatalog struct {
	mu                sync.RWMutex
	refreshMu         sync.Mutex
	cacheFile         string
	expiry            time.Duration
	openAIOAuth       bool
	openAIAPIKey      bool
	remoteProviders   []RemoteModelCatalogProvider
	remoteSources     map[string]catalogRemoteSource
	localProviders    []LocalModelCatalogProvider
	providers         map[string]catalogProvider
	needsCloudRefresh bool
	lastCloudAttempt  time.Time
	reportError       func(message string, err error)

	fetchEnrichment func(context.Context) (map[string]catalogProvider, error)
	fetchRemote     func(context.Context, RemoteModelCatalogProvider) (*catalogProvider, error)
	fetchLocal      func(context.Context, LocalModelCatalogProvider) ([]CatalogModel, error)
}

func NewModelCatalog(config ModelCatalogConfig) *ModelCatalog {
	catalog := &ModelCatalog{
		cacheFile:       strings.TrimSpace(config.CacheFile),
		expiry:          time.Duration(max(config.ExpiryDays, 0)) * 24 * time.Hour,
		openAIOAuth:     config.OpenAIOAuth,
		openAIAPIKey:    config.OpenAIAPIKey,
		remoteProviders: append([]RemoteModelCatalogProvider(nil), config.RemoteProviders...),
		localProviders:  append([]LocalModelCatalogProvider(nil), config.LocalProviders...),
		reportError:     config.ReportError,
	}
	catalog.fetchEnrichment = catalog.fetchModelsDevData
	catalog.fetchRemote = catalog.fetchRemoteProvider
	catalog.fetchLocal = catalog.fetchLocalModels
	return catalog
}

func (c *ModelCatalog) Init(ctx context.Context) {
	if c == nil {
		return
	}
	cached, remoteSources, needsRefresh := c.loadFromDisk()
	if cached == nil {
		cached = make(map[string]catalogProvider)
	}
	cached = filterLocalProviders(cached, c.localProviders)
	cached, remoteSources = filterConfiguredRemoteCache(cached, remoteSources, c.remoteProviders)

	c.mu.Lock()
	c.providers = cached
	c.remoteSources = remoteSources
	c.needsCloudRefresh = needsRefresh || remoteProviderCacheMismatch(cached, remoteSources, c.remoteProviders)
	c.mu.Unlock()

	c.refreshLocal(ctx)
}

func (c *ModelCatalog) EnsureFresh(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	if !c.shouldRefreshCloud(false) {
		return nil
	}
	return c.refreshCloudLocked(ctx, false)
}

func (c *ModelCatalog) Refresh(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	var errs []error
	if err := c.refreshCloudLocked(ctx, true); err != nil {
		errs = append(errs, err)
	}
	if err := c.refreshLocalLocked(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (c *ModelCatalog) ProviderName(providerID string) string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, key := range catalogProviderKeys(providerID) {
		if providerEntry, ok := c.providers[key]; ok && strings.TrimSpace(providerEntry.Name) != "" {
			return strings.TrimSpace(providerEntry.Name)
		}
	}
	return ""
}

func (c *ModelCatalog) ModelsForProvider(providerID string) []CatalogModel {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	var providerEntry catalogProvider
	found := false
	for _, key := range catalogProviderKeys(providerID) {
		entry, ok := c.providers[key]
		if !ok {
			continue
		}
		providerEntry = entry
		found = true
		break
	}
	if !found {
		return nil
	}

	allIDs := make(map[string]struct{}, len(providerEntry.Models))
	for modelID := range providerEntry.Models {
		allIDs[modelID] = struct{}{}
	}

	models := make([]CatalogModel, 0, len(providerEntry.Models))
	for _, model := range providerEntry.Models {
		if shouldHideCatalogModel(providerID, model, allIDs, c.openAIOAuth, c.openAIAPIKey) {
			continue
		}
		model = NormalizeCatalogModelCapabilities(providerID, model)
		model.Name = strings.TrimSpace(strings.TrimSuffix(firstNonBlank(model.Name, model.ID), " (latest)"))
		model.InputModalities = cloneStrings(model.InputModalities)
		model.OutputModalities = cloneStrings(model.OutputModalities)
		model.SupportedEndpoints = cloneStrings(model.SupportedEndpoints)
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})
	return models
}
