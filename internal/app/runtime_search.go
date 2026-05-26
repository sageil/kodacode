package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
	searchsvc "github.com/sageil/kodacode/internal/search"
)

const defaultSearchRefreshInterval = 10 * time.Second

func buildSearchService(config Config, logger *observability.Logger) (*searchsvc.Service, error) {
	embedder, model, err := buildSearchEmbedder(config)
	if err != nil {
		return nil, err
	}
	return searchsvc.NewService(
		embedder,
		model,
		config.Search.EmbeddingsDimensions,
		searchIndexDir(config.Search),
		logger,
		searchsvc.WithSkipDirs(config.Search.SkipDirs),
	), nil
}

func searchTrackOptions(config SearchConfig) searchsvc.TrackOptions {
	return searchsvc.TrackOptions{
		RefreshInterval:   defaultSearchRefreshInterval,
		PrewarmEmbeddings: config.PrewarmEmbeddings,
	}
}

func buildSearchEmbedder(config Config) (provider.Embedder, provider.ModelRef, error) {
	raw := strings.TrimSpace(config.Search.EmbeddingsModel)
	if raw == "" {
		return nil, provider.ModelRef{}, nil
	}
	model, err := provider.ParseModelRef(raw)
	if err != nil {
		return nil, provider.ModelRef{}, err
	}

	embedders := map[string]provider.Embedder{}
	if model.ProviderID == "openai" {
		embedder, err := buildOpenAIEmbedder(config.OpenAI)
		if err != nil {
			return nil, provider.ModelRef{}, err
		}
		embedders["openai"] = embedder
	}
	if compatible, ok := compatibleProviderConfig(config, model.ProviderID); ok {
		embedder, err := buildOpenAICompatibleEmbedder(compatible)
		if err != nil {
			return nil, provider.ModelRef{}, err
		}
		embedders[model.ProviderID] = embedder
	}
	return provider.NewRoutedEmbedder(embedders), model, nil
}

func searchIndexDir(config SearchConfig) string {
	if strings.TrimSpace(config.IndexDir) != "" {
		return config.IndexDir
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdg != "" {
		return filepath.Join(xdg, "kodacode", "search")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "kodacode-search")
	}
	return filepath.Join(home, ".local", "state", "kodacode", "search")
}
