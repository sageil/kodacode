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
		log.Printf("github-copilot: no auth configured — use /connect to set up")
		return nil, false, nil

	default:
		baseURL := pc.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return openai.New(pc.ID, pc.ID, pc.APIKey, baseURL, configModels(pc.Models)), false, nil
	}
}
