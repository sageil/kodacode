package app

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
)

type modelCatalog interface {
	ProviderName(providerID string) string
	ModelsForProvider(providerID string) []provider.CatalogModel
	EnsureFresh(ctx context.Context) error
	Refresh(ctx context.Context) error
}

func buildModelCatalog(config Config, logger *observability.Logger) modelCatalog {
	modelCatalogLogger := logger.With("component", "model_catalog")
	base := provider.NewModelCatalog(provider.ModelCatalogConfig{
		CacheFile:       modelCatalogPath(),
		ExpiryDays:      max(config.ModelCache.ExpiryDays, 0),
		OpenAIOAuth:     hasOpenAIOAuth(),
		OpenAIAPIKey:    strings.TrimSpace(config.OpenAI.APIKey) != "",
		RemoteProviders: remoteModelCatalogProviders(config),
		LocalProviders:  localModelCatalogProviders(config),
		ReportError: func(message string, err error) {
			if modelCatalogLogger != nil {
				modelCatalogLogger.Error(message, err)
			}
		},
	})
	base.Init(context.Background())
	return wrapModelCatalogOverrides(base, config.ModelOverrides)
}

func remoteModelCatalogProviders(config Config) []provider.RemoteModelCatalogProvider {
	var providers []provider.RemoteModelCatalogProvider

	providers = append(providers, openAIPlatformRemoteModelCatalogProvider(config))
	providers = append(providers, openAICodexRemoteModelCatalogProvider(config))
	if strings.TrimSpace(config.Anthropic.APIKey) != "" {
		providers = append(providers, provider.RemoteModelCatalogProvider{
			ID:      "anthropic",
			Name:    "Anthropic",
			Kind:    provider.RemoteModelCatalogProviderAnthropic,
			BaseURL: firstNonBlank(strings.TrimSpace(config.Anthropic.BaseURL), provider.DefaultAnthropicBaseURL()),
			APIKey:  strings.TrimSpace(config.Anthropic.APIKey),
		})
	}
	if strings.TrimSpace(config.Google.APIKey) != "" {
		providers = append(providers, provider.RemoteModelCatalogProvider{
			ID:      "google",
			Name:    "Google",
			Kind:    remoteModelCatalogKind("google"),
			BaseURL: firstNonBlank(strings.TrimSpace(config.Google.BaseURL), provider.DefaultGoogleBaseURL()),
			APIKey:  strings.TrimSpace(config.Google.APIKey),
		})
	}
	if compatible, ok := compatibleProviderConfig(config, "nvidia"); ok && strings.TrimSpace(compatible.APIKey) != "" {
		providers = append(providers, provider.RemoteModelCatalogProvider{
			ID:      "nvidia",
			Name:    "NVIDIA",
			Kind:    provider.RemoteModelCatalogProviderOpenAICompatible,
			BaseURL: compatible.BaseURL,
			APIKey:  compatible.APIKey,
		})
	}
	if copilotProvider := gitHubCopilotRemoteModelCatalogProvider(config); strings.TrimSpace(copilotProvider.ID) != "" {
		providers = append(providers, copilotProvider)
	}
	if compatible, ok := compatibleProviderConfig(config, "deepseek"); ok && strings.TrimSpace(compatible.APIKey) != "" {
		providers = append(providers, provider.RemoteModelCatalogProvider{
			ID:      "deepseek",
			Name:    "DeepSeek",
			Kind:    remoteModelCatalogKind("deepseek"),
			BaseURL: compatible.BaseURL,
			APIKey:  compatible.APIKey,
		})
	}

	compatibleIDs := make([]string, 0, len(config.CompatibleProviders))
	for providerID := range config.CompatibleProviders {
		compatibleIDs = append(compatibleIDs, providerID)
	}
	sort.Strings(compatibleIDs)
	for _, providerID := range compatibleIDs {
		compatible := config.CompatibleProviders[providerID]
		baseURL := compatibleProviderBaseURL(providerID, compatible.BaseURL)
		if strings.TrimSpace(baseURL) == "" || compatibleProviderAllowsEmptyAPIKey(baseURL) {
			continue
		}
		if strings.TrimSpace(compatible.APIKey) == "" {
			continue
		}
		kind := remoteModelCatalogKind(providerID)
		providers = append(providers, provider.RemoteModelCatalogProvider{
			ID:      providerID,
			Name:    remoteModelCatalogProviderName(providerID),
			Kind:    kind,
			BaseURL: baseURL,
			APIKey:  strings.TrimSpace(compatible.APIKey),
		})
	}
	if compatible, ok := config.configuredCompatibleProvider(strings.TrimSpace(config.OpenAICompatible.ProviderID)); ok {
		baseURL := compatibleProviderBaseURL(compatible.ProviderID, compatible.BaseURL)
		if strings.TrimSpace(baseURL) != "" &&
			!compatibleProviderAllowsEmptyAPIKey(baseURL) &&
			strings.TrimSpace(compatible.APIKey) != "" &&
			!containsRemoteModelCatalogProvider(providers, compatible.ProviderID) {
			providers = append(providers, provider.RemoteModelCatalogProvider{
				ID:      compatible.ProviderID,
				Name:    remoteModelCatalogProviderName(compatible.ProviderID),
				Kind:    remoteModelCatalogKind(compatible.ProviderID),
				BaseURL: baseURL,
				APIKey:  strings.TrimSpace(compatible.APIKey),
			})
		}
	}

	return dedupeRemoteModelCatalogProviders(providers)
}

func openAIPlatformRemoteModelCatalogProvider(config Config) provider.RemoteModelCatalogProvider {
	apiKey := strings.TrimSpace(config.OpenAI.APIKey)
	if apiKey == "" {
		return provider.RemoteModelCatalogProvider{}
	}

	return provider.RemoteModelCatalogProvider{
		ID:      "openai",
		Name:    "OpenAI",
		Kind:    provider.RemoteModelCatalogProviderOpenAI,
		BaseURL: openAIPlatformBaseURL(config.OpenAI.BaseURL),
		APIKey:  apiKey,
	}
}

func openAICodexRemoteModelCatalogProvider(config Config) provider.RemoteModelCatalogProvider {
	oauthEntry, oauthStore := loadOpenAIOAuth()
	hasOAuth := oauthEntry != nil && oauthStore != nil
	if !hasOAuth {
		return provider.RemoteModelCatalogProvider{}
	}

	return provider.RemoteModelCatalogProvider{
		ID:      openAICodexProviderID,
		Name:    "OpenAI Codex",
		Kind:    provider.RemoteModelCatalogProviderOpenAI,
		BaseURL: openAICodexBaseURL(config.OpenAI.BaseURL),
		OAuth: &provider.OpenAIOAuthConfig{
			Entry: *oauthEntry,
			Store: oauthStore,
		},
	}
}

func gitHubCopilotRemoteModelCatalogProvider(config Config) provider.RemoteModelCatalogProvider {
	oauthEntry, oauthStore := loadGitHubCopilotOAuth()
	token := strings.TrimSpace(config.GitHubCopilot.Token)
	if token == "" && oauthEntry == nil {
		return provider.RemoteModelCatalogProvider{}
	}

	var oauth *provider.GitHubCopilotOAuthConfig
	if oauthEntry != nil && oauthStore != nil {
		oauth = &provider.GitHubCopilotOAuthConfig{
			Entry: *oauthEntry,
			Store: oauthStore,
		}
	}

	return provider.RemoteModelCatalogProvider{
		ID:                 "github-copilot",
		Name:               "GitHub Copilot",
		Kind:               provider.RemoteModelCatalogProviderGitHubCopilot,
		BaseURL:            compatibleProviderBaseURL("github-copilot", config.GitHubCopilot.BaseURL),
		APIKey:             token,
		GitHubCopilotOAuth: oauth,
	}
}

func containsRemoteModelCatalogProvider(providers []provider.RemoteModelCatalogProvider, providerID string) bool {
	providerID = strings.TrimSpace(providerID)
	for _, providerEntry := range providers {
		if strings.TrimSpace(providerEntry.ID) == providerID {
			return true
		}
	}
	return false
}

func dedupeRemoteModelCatalogProviders(providers []provider.RemoteModelCatalogProvider) []provider.RemoteModelCatalogProvider {
	if len(providers) == 0 {
		return nil
	}
	deduped := make([]provider.RemoteModelCatalogProvider, 0, len(providers))
	seen := map[string]struct{}{}
	for _, providerEntry := range providers {
		providerID := strings.TrimSpace(providerEntry.ID)
		if providerID == "" {
			continue
		}
		if _, ok := seen[providerID]; ok {
			continue
		}
		seen[providerID] = struct{}{}
		deduped = append(deduped, providerEntry)
	}
	return deduped
}

func remoteModelCatalogKind(providerID string) provider.RemoteModelCatalogProviderKind {
	switch provider.CanonicalProviderID(providerID) {
	case "openai":
		return provider.RemoteModelCatalogProviderOpenAI
	case "anthropic":
		return provider.RemoteModelCatalogProviderAnthropic
	case "github-copilot":
		return provider.RemoteModelCatalogProviderGitHubCopilot
	case "google":
		return provider.RemoteModelCatalogProviderGoogle
	case "openrouter",
		"togetherai",
		"groq",
		"fireworks-ai",
		"mistral",
		"deepseek",
		"deepinfra",
		"cerebras",
		"venice",
		"moonshotai",
		"zai-coding-plan",
		"ollama-cloud":
		return provider.RemoteModelCatalogProviderModelsDev
	case "":
		return ""
	default:
		return provider.RemoteModelCatalogProviderOpenAICompatible
	}
}

func remoteModelCatalogProviderName(providerID string) string {
	switch strings.TrimSpace(providerID) {
	case "openrouter":
		return "OpenRouter"
	case "togetherai":
		return "Together AI"
	case "groq":
		return "Groq"
	case "fireworks-ai":
		return "Fireworks AI"
	case "mistral":
		return "Mistral"
	case "deepinfra":
		return "Deep Infra"
	case "qwencloud":
		return "QwenCloud"
	case "cerebras":
		return "Cerebras"
	case "venice":
		return "Venice AI"
	case "moonshotai":
		return "Moonshot AI (Kimi)"
	case "zai-coding-plan":
		return "Z.AI"
	case "ollama-cloud":
		return "Ollama Cloud"
	default:
		return strings.TrimSpace(providerID)
	}
}

func localModelCatalogProviders(config Config) []provider.LocalModelCatalogProvider {
	var providers []provider.LocalModelCatalogProvider

	addCompatible := func(providerID, baseURL string) {
		baseURL = compatibleProviderBaseURL(providerID, baseURL)
		if !compatibleProviderAllowsEmptyAPIKey(baseURL) {
			return
		}
		providers = append(providers, provider.LocalModelCatalogProvider{
			ID:      providerID,
			Name:    providerID,
			BaseURL: baseURL,
		})
	}

	compatibleIDs := make([]string, 0, len(config.CompatibleProviders))
	for providerID := range config.CompatibleProviders {
		compatibleIDs = append(compatibleIDs, providerID)
	}
	sort.Strings(compatibleIDs)
	for _, providerID := range compatibleIDs {
		addCompatible(providerID, config.CompatibleProviders[providerID].BaseURL)
	}
	if compatible, ok := config.configuredCompatibleProvider(strings.TrimSpace(config.OpenAICompatible.ProviderID)); ok {
		addCompatible(compatible.ProviderID, compatible.BaseURL)
	}

	deduped := make([]provider.LocalModelCatalogProvider, 0, len(providers))
	seen := map[string]struct{}{}
	for _, providerEntry := range providers {
		if _, ok := seen[providerEntry.ID]; ok {
			continue
		}
		seen[providerEntry.ID] = struct{}{}
		deduped = append(deduped, providerEntry)
	}
	return deduped
}

func modelCatalogPath() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "kodacode", "models-cache.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "kodacode-models-cache.json")
	}
	return filepath.Join(home, ".local", "share", "kodacode", "models-cache.json")
}
