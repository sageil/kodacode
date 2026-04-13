package main

import (
	"context"
	"log"
	"strings"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/provider/openai"
)

// isLocalProvider returns true if the provider's base URL points to a local
// server (Ollama, LMStudio, etc.) whose models must be discovered via API.
func isLocalProvider(pc config.ProviderConfig) bool {
	u := strings.ToLower(pc.BaseURL)
	return strings.Contains(u, "127.0.0.1") ||
		strings.Contains(u, "localhost") ||
		strings.Contains(u, "[::1]")
}

// configModels converts config model entries to provider.Model slice.
func configModels(models []config.ProviderModelConfig) []provider.Model {
	if len(models) == 0 {
		return nil
	}
	out := make([]provider.Model, len(models))
	for i, m := range models {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		out[i] = provider.Model{
			ID:             m.ID,
			Name:           name,
			ContextSize:    m.ContextSize,
			ThinkingBudget: m.ThinkingBudget,
		}
	}
	return out
}

// newProvider creates a Provider from a single ProviderConfig.
// Returns (nil, false, nil) when auth is missing and the provider can be skipped
// (e.g. github-copilot with no credentials). The bool signals the caller to
// call modelCache.SetOAuthProvider("openai").
func newProvider(ctx context.Context, pc config.ProviderConfig, authStore *provider.AuthStore) (provider.Provider, bool, error) {
	switch pc.ID {
	case "anthropic":
		ap := provider.NewAnthropicProvider(pc.APIKey)
		ap.SetConfiguredModels(configModels(pc.Models))
		return ap, false, nil

	case "google":
		gp, err := provider.NewGoogleProvider(ctx, pc.APIKey)
		if gp != nil {
			gp.SetConfiguredModels(configModels(pc.Models))
		}
		return gp, false, err

	case "openai":
		if auth := authStore.Get("openai"); auth != nil && auth.Type == provider.AuthTypeOAuth {
			return openai.NewWithOAuth(auth, authStore), true, nil
		}
		return openai.NewOpenAI(pc.APIKey, pc.BaseURL, configModels(pc.Models)), false, nil

	case "github-copilot":
		if auth := authStore.Get("github-copilot"); auth != nil && auth.Access != "" {
			copilotAuth := provider.NewCopilotAuth(auth.Access)
			return openai.NewWithCopilotAuth(copilotAuth, configModels(pc.Models)), false, nil
		}
		if pc.APIKey != "" {
			baseURL := pc.BaseURL
			if baseURL == "" {
				baseURL = "https://api.githubcopilot.com"
			}
			return openai.New(pc.ID, "GitHub Copilot", pc.APIKey, baseURL, nil), false, nil
		}
		log.Printf("github-copilot: no auth configured, use /connect to set up")
		return nil, false, nil

	default:
		baseURL := pc.BaseURL
		if baseURL == "" {
			if known, ok := knownBaseURLs[pc.ID]; ok {
				baseURL = known
			} else {
				baseURL = "https://api.openai.com/v1"
			}
		}
		name := pc.ID
		if known, ok := knownNames[pc.ID]; ok {
			name = known
		}
		return openai.New(pc.ID, name, pc.APIKey, baseURL, configModels(pc.Models)), false, nil
	}
}

var knownBaseURLs = map[string]string{
	"openrouter":      "https://openrouter.ai/api/v1",
	"together":        "https://api.together.xyz/v1",
	"groq":            "https://api.groq.com/openai/v1",
	"fireworks":       "https://api.fireworks.ai/inference/v1",
	"mistral":         "https://api.mistral.ai/v1",
	"deepseek":        "https://api.deepseek.com",
	"deepinfra":       "https://api.deepinfra.com/v1/openai",
	"cerebras":        "https://api.cerebras.ai/v1",
	"venice":          "https://api.venice.ai/api/v1",
	"moonshot":        "https://api.moonshot.ai/v1",
	"zai-coding-plan": "https://api.z.ai/api/coding/paas/v4",
	"ollama":          "http://localhost:11434/v1",
	"ollama-cloud":    "https://ollama.com/v1",
	"lmstudio":        "http://localhost:1234/v1",
	"llamacpp":        "http://localhost:8080/v1",
}

var knownNames = map[string]string{
	"openrouter":      "OpenRouter",
	"together":        "Together AI",
	"groq":            "Groq",
	"fireworks":       "Fireworks AI",
	"mistral":         "Mistral",
	"deepseek":        "DeepSeek",
	"deepinfra":       "Deep Infra",
	"cerebras":        "Cerebras",
	"venice":          "Venice AI",
	"moonshot":        "Moonshot AI (Kimi)",
	"zai-coding-plan": "Z.AI",
	"ollama":          "Ollama",
	"ollama-cloud":    "Ollama Cloud",
	"lmstudio":        "LM Studio",
	"llamacpp":        "llama.cpp",
}
