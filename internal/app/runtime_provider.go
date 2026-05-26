package app

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

const openAICodexProviderID = "openai-codex"

func buildProviderClient(config Config) (provider.Client, error) {
	clients := make(map[string]provider.Client)
	for _, connected := range connectedProviders(config) {
		providerID := strings.TrimSpace(connected.ProviderID)
		if providerID == "" {
			continue
		}
		if _, ok := clients[providerID]; ok {
			continue
		}
		client, err := buildProviderClientForID(config, providerID)
		if err != nil {
			return nil, err
		}
		clients[providerID] = provider.NewPromptingClient(client)
	}

	return provider.NewRoutedClient(clients)
}

func buildProviderClientForID(config Config, providerID string) (provider.Client, error) {
	switch providerID {
	case "openai":
		return buildOpenAIPlatformProviderClient(config.OpenAI)
	case openAICodexProviderID:
		return buildOpenAICodexProviderClient(config.OpenAI)
	case "anthropic":
		return buildAnthropicProviderClient(config.Anthropic)
	case "google":
		return buildGoogleProviderClient(config.Google)
	case "nvidia":
		return buildNVIDIAProviderClient(config.NVIDIA)
	default:
		if client, handled, err := buildExperimentalProviderClient(providerID); handled {
			return client, err
		}
		return buildConfiguredCompatibleProviderClient(config, providerID)
	}
}

func buildOpenAIPlatformProviderClient(config OpenAIProviderConfig) (provider.Client, error) {
	return provider.NewOpenAIClient(provider.OpenAIConfig{
		APIKey:                   config.APIKey,
		BaseURL:                  openAIPlatformBaseURL(config.BaseURL),
		Backend:                  provider.OpenAIBackendPlatformAPI,
		PromptCacheRetention:     config.PromptCacheRetention,
		ResponsesStore:           config.ResponsesStore,
		EncryptedReasoningReplay: openAIEncryptedReasoningReplayEnabled(config),
	})
}

func buildOpenAICodexProviderClient(config OpenAIProviderConfig) (provider.Client, error) {
	clientConfig := provider.OpenAIConfig{
		BaseURL:                  openAICodexBaseURL(config.BaseURL),
		Backend:                  provider.OpenAIBackendChatGPTCodex,
		PromptCacheRetention:     config.PromptCacheRetention,
		ResponsesStore:           config.ResponsesStore,
		EncryptedReasoningReplay: openAIEncryptedReasoningReplayEnabled(config),
	}

	if entry, store := loadOpenAIOAuth(); entry != nil {
		clientConfig.OAuth = &provider.OpenAIOAuthConfig{
			Entry: *entry,
			Store: store,
		}
	}

	return provider.NewOpenAIClient(clientConfig)
}

func openAIPlatformBaseURL(configured string) string {
	configured = provider.NormalizeOpenAIBaseURL(configured)
	if configured == provider.DefaultOpenAIOAuthBaseURL() || strings.Contains(configured, "chatgpt.com/backend-api/codex") {
		return provider.DefaultOpenAIBaseURL()
	}
	if configured != "" {
		return configured
	}
	return provider.DefaultOpenAIBaseURL()
}

func openAICodexBaseURL(configured string) string {
	configured = provider.NormalizeOpenAIBaseURL(configured)
	if configured == "" {
		return provider.DefaultOpenAIOAuthBaseURL()
	}
	if strings.Contains(configured, "chatgpt.com/backend-api/codex") {
		return configured
	}
	return provider.DefaultOpenAIOAuthBaseURL()
}

func openAIEncryptedReasoningReplayEnabled(config OpenAIProviderConfig) bool {
	return config.EncryptedReasoningReplay == nil || *config.EncryptedReasoningReplay
}

func buildOpenAIEmbedder(config OpenAIProviderConfig) (provider.Embedder, error) {
	embedderConfig := provider.OpenAIEmbeddingConfig{
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
	}
	if entry, store := loadOpenAIOAuth(); entry != nil {
		embedderConfig.OAuth = &provider.OpenAIOAuthConfig{
			Entry: *entry,
			Store: store,
		}
	}
	return provider.NewOpenAIEmbedder(embedderConfig)
}

func buildOpenAICompatibleProviderClient(config OpenAICompatibleProviderConfig) (provider.Client, error) {
	return provider.NewOpenAICompatibleClient(provider.OpenAICompatibleConfig{
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
	})
}

func buildOpenAICompatibleEmbedder(config OpenAICompatibleProviderConfig) (provider.Embedder, error) {
	return provider.NewOpenAICompatibleEmbedder(provider.OpenAICompatibleEmbeddingConfig{
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
	})
}

func buildAnthropicProviderClient(config AnthropicProviderConfig) (provider.Client, error) {
	return provider.NewAnthropicClient(provider.AnthropicConfig{
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
	})
}

func buildGoogleProviderClient(config GoogleProviderConfig) (provider.Client, error) {
	return provider.NewGoogleClient(context.Background(), provider.GoogleConfig{
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
	})
}

func buildNVIDIAProviderClient(config NVIDIAProviderConfig) (provider.Client, error) {
	return provider.NewOpenAICompatibleClient(provider.OpenAICompatibleConfig{
		APIKey:  config.APIKey,
		BaseURL: compatibleProviderBaseURL("nvidia", config.BaseURL),
	})
}

func buildGitHubCopilotProviderClient(config GitHubCopilotProviderConfig) (provider.Client, error) {
	clientConfig := provider.GitHubCopilotConfig{
		Token:   strings.TrimSpace(config.Token),
		BaseURL: compatibleProviderBaseURL("github-copilot", config.BaseURL),
	}
	if oauthEntry, oauthStore := loadGitHubCopilotOAuth(); oauthEntry != nil && oauthStore != nil {
		clientConfig.OAuth = &provider.GitHubCopilotOAuthConfig{
			Entry: *oauthEntry,
			Store: oauthStore,
		}
	}
	if clientConfig.Token == "" {
		clientConfig.Token = loadGitHubCopilotToken()
	}
	return provider.NewGitHubCopilotClient(clientConfig)
}

func buildConfiguredCompatibleProviderClient(config Config, providerID string) (provider.Client, error) {
	switch providerID {
	case "github-copilot":
		return buildGitHubCopilotProviderClient(config.GitHubCopilot)
	default:
		compatible, ok := compatibleProviderConfig(config, providerID)
		if !ok {
			return nil, provider.ErrProviderNotConfigured
		}
		return buildOpenAICompatibleProviderClient(compatible)
	}
}

func compatibleProviderConfig(config Config, providerID string) (OpenAICompatibleProviderConfig, bool) {
	switch providerID {
	case "deepseek":
		return OpenAICompatibleProviderConfig{
			ProviderID: providerID,
			APIKey:     config.DeepSeek.APIKey,
			BaseURL:    compatibleProviderBaseURL(providerID, config.DeepSeek.BaseURL),
		}, true
	case "nvidia":
		return OpenAICompatibleProviderConfig{
			ProviderID: providerID,
			APIKey:     config.NVIDIA.APIKey,
			BaseURL:    compatibleProviderBaseURL(providerID, config.NVIDIA.BaseURL),
		}, true
	case "github-copilot":
		token := strings.TrimSpace(config.GitHubCopilot.Token)
		if token == "" {
			token = loadGitHubCopilotToken()
		}
		return OpenAICompatibleProviderConfig{
			ProviderID: providerID,
			APIKey:     token,
			BaseURL:    compatibleProviderBaseURL(providerID, config.GitHubCopilot.BaseURL),
		}, true
	default:
		compatible, ok := config.configuredCompatibleProvider(providerID)
		if !ok {
			return OpenAICompatibleProviderConfig{}, false
		}
		compatible.ProviderID = providerID
		compatible.BaseURL = compatibleProviderBaseURL(providerID, compatible.BaseURL)
		return compatible, true
	}
}

func compatibleProviderBaseURL(providerID, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return configured
	}
	switch providerID {
	case "nvidia":
		return "https://integrate.api.nvidia.com/v1"
	case "github-copilot":
		return "https://api.githubcopilot.com"
	case "deepseek":
		return "https://api.deepseek.com"
	case "qwencloud":
		return "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "togetherai":
		return "https://api.together.xyz/v1"
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "fireworks-ai":
		return "https://api.fireworks.ai/inference/v1"
	case "mistral":
		return "https://api.mistral.ai/v1"
	case "deepinfra":
		return "https://api.deepinfra.com/v1/openai"
	case "cerebras":
		return "https://api.cerebras.ai/v1"
	case "venice":
		return "https://api.venice.ai/api/v1"
	case "moonshotai":
		return "https://api.moonshot.ai/v1"
	case "zai-coding-plan":
		return "https://api.z.ai/api/coding/paas/v4"
	case "ollama":
		return "http://localhost:11434/v1"
	case "ollama-cloud":
		return "https://ollama.com/v1"
	case "lmstudio":
		return "http://localhost:1234/v1"
	case "llamacpp":
		return "http://localhost:8080/v1"
	default:
		return configured
	}
}

func compatibleProviderAllowsEmptyAPIKey(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, host)
}

func loadOpenAIOAuth() (*provider.AuthEntry, *provider.AuthStore) {
	store := provider.NewAuthStore()
	entry := store.Get("openai")
	if entry == nil || entry.Type != provider.AuthTypeOAuth {
		return nil, nil
	}
	if strings.TrimSpace(entry.Access) == "" && strings.TrimSpace(entry.Refresh) == "" {
		return nil, nil
	}
	return entry, store
}

func loadGitHubCopilotToken() string {
	store := provider.NewAuthStore()
	entry := store.Get("github-copilot")
	if entry == nil {
		return ""
	}
	return strings.TrimSpace(entry.Access)
}

func loadGitHubCopilotOAuth() (*provider.AuthEntry, *provider.AuthStore) {
	store := provider.NewAuthStore()
	entry := store.Get("github-copilot")
	if entry == nil || entry.Type != provider.AuthTypeOAuth {
		return nil, nil
	}
	if strings.TrimSpace(entry.Access) == "" && strings.TrimSpace(entry.Refresh) == "" {
		return nil, nil
	}
	return entry, store
}
