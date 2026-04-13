package main

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"
)

type syncTestProvider struct {
	id   string
	name string
}

func (p *syncTestProvider) ID() string { return p.id }

func (p *syncTestProvider) Name() string { return p.name }

func (p *syncTestProvider) Models(context.Context) ([]provider.Model, error) {
	return nil, nil
}

func (p *syncTestProvider) Chat(context.Context, string, []provider.Message, provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk)
	close(ch)
	return ch, nil
}

func TestProviderSyncerRegistersOnlyNewProviders(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Providers: []config.ProviderConfig{{
			ID: "openai",
		}},
	}
	registry := provider.NewRegistry()
	if err := registry.Register(&syncTestProvider{id: "openai", name: "OpenAI"}); err != nil {
		t.Fatalf("Register(openai) error = %v", err)
	}

	syncer := &providerSyncer{
		cfg:       cfg,
		authStore: provider.NewAuthStoreAt(t.TempDir() + "/auth.yaml"),
		registry:  registry,
		loadConfig: func(string) (*config.Config, error) {
			return &config.Config{
				Providers: []config.ProviderConfig{
					{ID: "openai"},
					{ID: "github-copilot", BaseURL: "https://api.githubcopilot.com"},
				},
			}, nil
		},
		newProvider: func(_ context.Context, pc config.ProviderConfig, _ *provider.AuthStore) (provider.Provider, bool, error) {
			return &syncTestProvider{id: pc.ID, name: pc.ID}, false, nil
		},
		isLocalProvider: func(config.ProviderConfig) bool { return false },
	}

	activated, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(activated) != 1 || activated[0] != "github-copilot" {
		t.Fatalf("activated = %#v, want [github-copilot]", activated)
	}
	if _, ok := registry.Get("github-copilot"); !ok {
		t.Fatal("github-copilot was not registered")
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("len(cfg.Providers) = %d, want 2", len(cfg.Providers))
	}
}
