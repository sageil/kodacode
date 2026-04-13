package main

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"
)

// providerSyncer hot-adds newly configured providers into the live registry.
// It does not remove or replace already registered providers.
type providerSyncer struct {
	mu sync.Mutex

	projectDir string
	cfg        *config.Config
	authStore  *provider.AuthStore
	registry   *provider.Registry
	modelCache *provider.ModelCache

	loadConfig      func(string) (*config.Config, error)
	newProvider     func(context.Context, config.ProviderConfig, *provider.AuthStore) (provider.Provider, bool, error)
	isLocalProvider func(config.ProviderConfig) bool
}

func newProviderSyncer(
	projectDir string,
	cfg *config.Config,
	authStore *provider.AuthStore,
	registry *provider.Registry,
	modelCache *provider.ModelCache,
) *providerSyncer {
	return &providerSyncer{
		projectDir:      projectDir,
		cfg:             cfg,
		authStore:       authStore,
		registry:        registry,
		modelCache:      modelCache,
		loadConfig:      config.Load,
		newProvider:     newProvider,
		isLocalProvider: isLocalProvider,
	}
}

func (s *providerSyncer) Sync(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.loadConfig == nil {
		return nil, nil
	}

	nextCfg, err := s.loadConfig(s.projectDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	var activated []string
	for _, pc := range nextCfg.Providers {
		if s.modelCache != nil && s.isLocalProvider != nil && s.isLocalProvider(pc) {
			s.modelCache.RegisterLocal(provider.LocalProviderEndpoint{
				ID:      pc.ID,
				Name:    pc.ID,
				BaseURL: pc.BaseURL,
			})
		}

		if s.registry == nil {
			continue
		}
		if _, exists := s.registry.Get(pc.ID); exists {
			continue
		}

		p, oauthOpenAI, err := s.newProvider(ctx, pc, s.authStore)
		if err != nil {
			return nil, fmt.Errorf("init provider %q: %w", pc.ID, err)
		}
		if p == nil {
			continue
		}
		if s.modelCache != nil && oauthOpenAI {
			s.modelCache.SetOAuthProvider("openai")
		}
		if err := s.registry.Register(p); err != nil {
			return nil, fmt.Errorf("register provider %q: %w", pc.ID, err)
		}
		activated = append(activated, pc.ID)
	}

	if s.cfg != nil {
		*s.cfg = *nextCfg
	}

	sort.Strings(activated)
	return activated, nil
}
