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
	models     []APIProviderModels
}

const providerConnectBusyMessage = "Finish active turns before connecting a new provider. Restart is required for provider updates or removals."

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
			return a.applyCopilotConnection(result)
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

		existing := providerExists(cfg, providerID)

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

		if existing {
			return providerConnectedMsg{
				providerID: providerID,
				message:    fmt.Sprintf("Provider %q updated. Restart kodacode to apply.", providerID),
			}
		}
		if a.providerSyncBlocked() {
			return providerConnectedMsg{
				providerID: providerID,
				message:    fmt.Sprintf("Provider %q saved. %s", providerID, providerConnectBusyMessage),
			}
		}
		return a.syncProvidersAndRefresh(providerID)
	}
}

func (a App) runOAuthFlow(providerID string) tea.Msg {
	store := provider.NewAuthStore()

	_ = store
	return SSEErrorMsg{Err: fmt.Errorf("OAuth login is not supported for provider %q", providerID)}
}

func (a App) applyCopilotConnection(result ProviderConnectResult) tea.Msg {
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

	if found {
		return providerConnectedMsg{
			providerID: "github-copilot",
			message:    `Provider "github-copilot" updated. Restart kodacode to apply.`,
		}
	}
	if a.providerSyncBlocked() {
		return providerConnectedMsg{
			providerID: "github-copilot",
			message:    `Provider "github-copilot" saved. ` + providerConnectBusyMessage,
		}
	}
	return a.syncProvidersAndRefresh("github-copilot")
}

func (a App) syncProvidersAndRefresh(providerID string) tea.Msg {
	activated, err := a.api.SyncProviders(a.ctx)
	if err != nil {
		return SSEErrorMsg{Err: fmt.Errorf("sync providers: %w", err)}
	}

	message := fmt.Sprintf("Provider %q saved and activated.", providerID)
	if !containsString(activated, providerID) {
		message = fmt.Sprintf("Provider %q saved. Restart kodacode to apply changes to the active provider.", providerID)
	}

	models, err := a.api.RefreshModels(a.ctx)
	if err != nil {
		return providerConnectedMsg{
			providerID: providerID,
			message:    message + " Model list refresh failed: " + err.Error(),
		}
	}

	return providerConnectedMsg{
		providerID: providerID,
		message:    message,
		models:     models,
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func providerExists(cfg *config.Config, providerID string) bool {
	if cfg == nil {
		return false
	}
	for _, pc := range cfg.Providers {
		if pc.ID == providerID {
			return true
		}
	}
	return false
}
