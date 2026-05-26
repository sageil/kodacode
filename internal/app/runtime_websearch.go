package app

import (
	"strings"

	websearchsvc "github.com/sageil/kodacode/internal/websearch"
)

const (
	defaultExaWebSearchBaseURL      = "https://api.exa.ai"
	defaultParallelWebSearchBaseURL = "https://api.parallel.ai"
	defaultWebSearchTimeoutMS       = 15000
)

func buildWebSearchService(config Config) (*websearchsvc.Service, error) {
	if len(config.WebSearch.Providers) == 0 {
		return nil, nil
	}

	defaultProvider := effectiveWebSearchDefaultProvider(config.WebSearch)
	backends := make([]websearchsvc.Backend, 0, len(config.WebSearch.Providers))
	for providerID, providerConfig := range config.WebSearch.Providers {
		backend, err := buildWebSearchBackend(providerID, providerConfig)
		if err != nil {
			return nil, err
		}
		backends = append(backends, backend)
	}
	return websearchsvc.NewService(defaultProvider, backends...)
}

func effectiveWebSearchDefaultProvider(config WebSearchConfig) string {
	if strings.TrimSpace(config.DefaultProvider) != "" {
		return strings.TrimSpace(config.DefaultProvider)
	}
	if len(config.Providers) != 1 {
		return ""
	}
	for providerID := range config.Providers {
		return strings.TrimSpace(providerID)
	}
	return ""
}

func buildWebSearchBackend(providerID string, config WebSearchProviderConfig) (websearchsvc.Backend, error) {
	switch strings.ToLower(strings.TrimSpace(config.Kind)) {
	case "exa":
		return websearchsvc.NewExaBackend(websearchsvc.ExaBackendConfig{
			ID:        strings.TrimSpace(providerID),
			APIKey:    strings.TrimSpace(config.APIKey),
			BaseURL:   firstNonBlank(strings.TrimSpace(config.BaseURL), defaultExaWebSearchBaseURL),
			TimeoutMS: defaultWebSearchTimeout(config.TimeoutMS),
		}), nil
	case "parallel":
		return websearchsvc.NewParallelBackend(websearchsvc.ParallelBackendConfig{
			ID:        strings.TrimSpace(providerID),
			APIKey:    strings.TrimSpace(config.APIKey),
			BaseURL:   firstNonBlank(strings.TrimSpace(config.BaseURL), defaultParallelWebSearchBaseURL),
			TimeoutMS: defaultWebSearchTimeout(config.TimeoutMS),
		}), nil
	default:
		return nil, ErrWebSearchProviderKindUnsupported
	}
}

func defaultWebSearchTimeout(timeoutMS int) int {
	if timeoutMS <= 0 {
		return defaultWebSearchTimeoutMS
	}
	return timeoutMS
}
