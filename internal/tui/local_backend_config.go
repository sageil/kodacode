package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
	tuitheme "github.com/sageil/kodacode/internal/tui/theme"
)

func (b *LocalBackend) SetThemeName(_ context.Context, themeName string) error {
	if _, err := tuitheme.Load(strings.TrimSpace(themeName)); err != nil {
		return err
	}
	return app.NewConfigStore().SetTheme(strings.TrimSpace(themeName))
}

func (b *LocalBackend) SetTUILayout(_ context.Context, layout string) error {
	return app.NewConfigStore().SetTUILayout(layout)
}

func (b *LocalBackend) SaveProvider(_ context.Context, input app.ProviderConnectionInput) error {
	providerID := strings.TrimSpace(input.ProviderID)
	if providerID == "" {
		return app.ErrOpenAICompatibleProviderIDRequired
	}
	return b.applyReconfigurableMutation(func(configStore *app.ConfigStore, auth *provider.AuthStore) error {
		if err := configStore.UpsertProvider(input); err != nil {
			return err
		}
		if apiKey := strings.TrimSpace(input.APIKey); apiKey != "" {
			return auth.Set(providerID, provider.AuthEntry{
				Type:   provider.AuthTypeAPI,
				Access: apiKey,
			})
		}
		return nil
	})
}

func (b *LocalBackend) BeginGitHubCopilotAuth(ctx context.Context, baseURL string) (app.GitHubCopilotAuthChallenge, error) {
	if b == nil || b.runtime == nil {
		return app.GitHubCopilotAuthChallenge{}, nil
	}
	beginCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	deviceCode, err := b.requestGitHubCopilotDeviceCode(beginCtx)
	if err != nil {
		return app.GitHubCopilotAuthChallenge{}, err
	}

	normalizedBaseURL := strings.TrimSpace(baseURL)
	if normalizedBaseURL == "" {
		normalizedBaseURL = "https://api.githubcopilot.com"
	}

	return app.GitHubCopilotAuthChallenge{
		BaseURL:         normalizedBaseURL,
		DeviceCode:      deviceCode.DeviceCode,
		UserCode:        deviceCode.UserCode,
		VerificationURL: deviceCode.VerificationURL,
		ExpiresIn:       deviceCode.ExpiresIn,
		Interval:        deviceCode.Interval,
	}, nil
}

func (b *LocalBackend) CompleteGitHubCopilotAuth(ctx context.Context, challenge app.GitHubCopilotAuthChallenge) (app.DialogState, error) {
	if b == nil || b.runtime == nil {
		return app.DialogState{}, nil
	}

	entry, err := b.pollGitHubCopilotDeviceCode(ctx, provider.GitHubCopilotDeviceCode{
		DeviceCode:      strings.TrimSpace(challenge.DeviceCode),
		UserCode:        strings.TrimSpace(challenge.UserCode),
		VerificationURL: strings.TrimSpace(challenge.VerificationURL),
		ExpiresIn:       challenge.ExpiresIn,
		Interval:        challenge.Interval,
	})
	if err != nil {
		return app.DialogState{}, err
	}

	normalizedBaseURL := strings.TrimSpace(challenge.BaseURL)
	if normalizedBaseURL == "" {
		normalizedBaseURL = "https://api.githubcopilot.com"
	}

	if err := b.applyReconfigurableMutation(func(configStore *app.ConfigStore, auth *provider.AuthStore) error {
		if err := configStore.UpsertProvider(app.ProviderConnectionInput{
			ProviderID: "github-copilot",
			BaseURL:    normalizedBaseURL,
		}); err != nil {
			return err
		}
		return auth.Set("github-copilot", *entry)
	}); err != nil {
		return app.DialogState{}, err
	}

	return b.runtime.DialogState()
}

func (b *LocalBackend) RemoveProvider(_ context.Context, providerID string) error {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil
	}
	return b.applyReconfigurableMutation(func(configStore *app.ConfigStore, auth *provider.AuthStore) error {
		if err := configStore.RemoveProvider(providerID); err != nil {
			return err
		}
		return auth.Remove(providerID)
	})
}

func (b *LocalBackend) applyReconfigurableMutation(
	mutate func(configStore *app.ConfigStore, auth *provider.AuthStore) error,
) error {
	if b == nil || b.runtime == nil {
		return nil
	}

	configStore := app.NewConfigStore()
	authStore := provider.NewAuthStore()
	snapshot, err := snapshotConfigFiles(configStore.Path(), authStore.Path())
	if err != nil {
		return err
	}

	stagedConfig, stagedAuth, cleanup, err := stagedConfigStores(snapshot)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := mutate(stagedConfig, stagedAuth); err != nil {
		return err
	}

	config, err := app.LoadRuntimeConfigWithSources(b.getenv, stagedConfig, stagedAuth)
	if err != nil {
		return err
	}
	preserveRuntimeOnlyConfig(&config, b.runtime.Config)

	if err := installStagedConfig(
		stagedConfig.Path(),
		configStore.Path(),
		stagedAuth.Path(),
		authStore.Path(),
	); err != nil {
		restoreErr := restoreConfigFiles(snapshot)
		return errors.Join(err, restoreErr)
	}
	if err := b.runtime.Reconfigure(config); err != nil {
		restoreErr := restoreConfigFiles(snapshot)
		return errors.Join(err, restoreErr)
	}
	return nil
}

func preserveRuntimeOnlyConfig(config *app.Config, current app.Config) {
	if config == nil {
		return
	}
	if strings.TrimSpace(config.Sessions.DBPath) == "" {
		config.Sessions.DBPath = current.Sessions.DBPath
	}
}
