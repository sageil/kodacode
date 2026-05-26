package tui

import (
	"context"
	"sync"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
)

type LocalBackendConfig struct {
	Runtime *app.Runtime
	Getenv  func(string) string
}

type LocalBackend struct {
	runtime *app.Runtime
	getenv  func(string) string

	requestGitHubCopilotDeviceCode func(ctx context.Context) (*provider.GitHubCopilotDeviceCode, error)
	pollGitHubCopilotDeviceCode    func(ctx context.Context, challenge provider.GitHubCopilotDeviceCode) (*provider.AuthEntry, error)
	exchangeOpenAIOAuthCode        func(ctx context.Context, request provider.OpenAIOAuthCodeExchangeRequest) (*provider.AuthEntry, error)

	openAIAuthMu   sync.Mutex
	openAIAuthFlow *openAIAuthFlow
}

func NewLocalBackend(cfg LocalBackendConfig) *LocalBackend {
	return &LocalBackend{
		runtime: cfg.Runtime,
		getenv:  cfg.Getenv,
		requestGitHubCopilotDeviceCode: func(ctx context.Context) (*provider.GitHubCopilotDeviceCode, error) {
			return provider.RequestGitHubCopilotDeviceCode(ctx, nil, "")
		},
		pollGitHubCopilotDeviceCode: func(ctx context.Context, challenge provider.GitHubCopilotDeviceCode) (*provider.AuthEntry, error) {
			return provider.PollGitHubCopilotDeviceCode(ctx, nil, challenge, "", "")
		},
		exchangeOpenAIOAuthCode: func(ctx context.Context, request provider.OpenAIOAuthCodeExchangeRequest) (*provider.AuthEntry, error) {
			return provider.ExchangeOpenAIOAuthCode(ctx, nil, "", request)
		},
	}
}

func (b *LocalBackend) Close() error {
	if b != nil {
		b.closeOpenAIAuthFlow("")
	}
	if b == nil || b.runtime == nil {
		return nil
	}
	return b.runtime.Close()
}
