package tui

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"

	tea "charm.land/bubbletea/v2"
)

type providerConnectedMsg struct {
	providerID string
	message    string
}

// removeProvider deletes a provider from config and its OAuth credentials.
func removeProvider(providerID string) tea.Msg {
	cfg, err := config.Load("")
	if err != nil {
		return SSEErrorMsg{Err: fmt.Errorf("load config: %w", err)}
	}
	filtered := cfg.Providers[:0]
	for _, p := range cfg.Providers {
		if p.ID != providerID {
			filtered = append(filtered, p)
		}
	}
	cfg.Providers = filtered
	if err := config.Save(cfg); err != nil {
		return SSEErrorMsg{Err: fmt.Errorf("save config: %w", err)}
	}

	store := provider.NewAuthStore()
	_ = store.Remove(providerID)

	return providerConnectedMsg{
		providerID: providerID,
		message:    fmt.Sprintf("Provider %q removed. Restart kodacode to apply.", providerID),
	}
}

// applyProviderConnection saves the provider credentials and config.
// For OAuth, it triggers the browser-based OAuth flow.
// For API key, it saves the key to config immediately.
func (a App) applyProviderConnection(result ProviderConnectResult) tea.Cmd {
	return func() tea.Msg {
		if result.Remove {
			return removeProvider(result.ProviderID)
		}

		if result.AuthType == "oauth" {
			return a.runOAuthFlow(result.ProviderID)
		}

		// Copilot token import or manual entry.
		if strings.HasPrefix(result.AuthType, "copilot-") {
			return applyCopilotConnection(result)
		}

		// API key: save to config.
		cfg, err := config.Load("")
		if err != nil {
			return SSEErrorMsg{Err: fmt.Errorf("load config: %w", err)}
		}

		providerID := result.ProviderID
		if providerID == "custom" && result.BaseURL != "" {
			// Derive provider ID from base URL host.
			providerID = result.ProviderID
		}

		found := false
		for i := range cfg.Providers {
			if cfg.Providers[i].ID == providerID {
				cfg.Providers[i].APIKey = result.APIKey
				if result.BaseURL != "" {
					cfg.Providers[i].BaseURL = result.BaseURL
				}
				found = true
				break
			}
		}
		if !found {
			pc := config.ProviderConfig{
				ID:     providerID,
				APIKey: result.APIKey,
			}
			if result.BaseURL != "" {
				pc.BaseURL = result.BaseURL
			}
			cfg.Providers = append(cfg.Providers, pc)
		}
		if err := config.Save(cfg); err != nil {
			return SSEErrorMsg{Err: fmt.Errorf("save config: %w", err)}
		}

		return providerConnectedMsg{providerID: providerID, message: fmt.Sprintf("Provider %q saved. Restart kodacode to activate.", providerID)}
	}
}

func (a App) runOAuthFlow(providerID string) tea.Msg {
	store := provider.NewAuthStore()

	_ = store
	return SSEErrorMsg{Err: fmt.Errorf("OAuth login is not supported for provider %q", providerID)}
}

func applyCopilotConnection(result ProviderConnectResult) tea.Msg {
	var ghoToken string
	var err error

	switch result.AuthType {
	case "copilot-neovim":
		ghoToken, err = provider.ReadCopilotTokenFromNeovim()
	case "copilot-opencode":
		ghoToken, err = provider.ReadCopilotTokenFromOpencode()
	case "copilot-manual":
		ghoToken = strings.TrimSpace(result.APIKey)
		if ghoToken == "" {
			return SSEErrorMsg{Err: fmt.Errorf("no token provided")}
		}
		if !strings.HasPrefix(ghoToken, "gho_") {
			return SSEErrorMsg{Err: fmt.Errorf("invalid token: must start with gho_")}
		}
	}
	if err != nil {
		return SSEErrorMsg{Err: err}
	}

	// Store the gho_ token in kodacode's auth store.
	store := provider.NewAuthStore()
	if err := store.Set("github-copilot", provider.AuthEntry{
		Type:   provider.AuthTypeOAuth,
		Access: ghoToken,
	}); err != nil {
		return SSEErrorMsg{Err: fmt.Errorf("save auth: %w", err)}
	}

	// Ensure provider exists in config with the Copilot base URL.
	cfg, _ := config.Load("")
	found := false
	for i, p := range cfg.Providers {
		if p.ID == "github-copilot" {
			cfg.Providers[i].BaseURL = "https://api.githubcopilot.com"
			cfg.Providers[i].APIKey = "" // auth store handles it
			found = true
			break
		}
	}
	if !found {
		cfg.Providers = append(cfg.Providers, config.ProviderConfig{
			ID:      "github-copilot",
			BaseURL: "https://api.githubcopilot.com",
		})
	}
	_ = config.Save(cfg)

	return providerConnectedMsg{
		providerID: "github-copilot",
		message:    "GitHub Copilot connected. Restart kodacode to activate.",
	}
}
