package websearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrQueryRequired         = errors.New("query is required")
	ErrLimitInvalid          = errors.New("limit must be greater than zero")
	ErrFreshnessDaysInvalid  = errors.New("freshness_days must be greater than zero")
	ErrProviderRequired      = errors.New("provider is required")
	ErrProviderNotConfigured = errors.New("web search provider is not configured")
)

type Request struct {
	Query          string
	Limit          int
	Domains        []string
	ExcludeDomains []string
	FreshnessDays  int
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.Query) == "" {
		return ErrQueryRequired
	}
	if r.Limit <= 0 {
		return ErrLimitInvalid
	}
	if r.FreshnessDays < 0 {
		return ErrFreshnessDaysInvalid
	}
	for _, value := range r.Domains {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("domains must not contain blank values")
		}
	}
	for _, value := range r.ExcludeDomains {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("exclude_domains must not contain blank values")
		}
	}
	return nil
}

type Result struct {
	Title       string
	URL         string
	Snippet     string
	Domain      string
	PublishedAt string
	Author      string
	Score       float64
}

type Response struct {
	Provider  string
	RequestID string
	Notice    string
	Results   []Result
}

type Backend interface {
	ID() string
	Search(context.Context, Request) (Response, error)
}

type Service struct {
	defaultProvider string
	backends        map[string]Backend
}

func NewService(defaultProvider string, backends ...Backend) (*Service, error) {
	service := &Service{
		defaultProvider: normalizeProviderID(defaultProvider),
		backends:        make(map[string]Backend, len(backends)),
	}
	for _, backend := range backends {
		if backend == nil {
			continue
		}
		providerID := normalizeProviderID(backend.ID())
		if providerID == "" {
			return nil, ErrProviderRequired
		}
		if _, exists := service.backends[providerID]; exists {
			return nil, fmt.Errorf("duplicate web search provider %q", providerID)
		}
		service.backends[providerID] = backend
	}
	if len(service.backends) == 0 {
		return service, nil
	}
	if service.defaultProvider == "" {
		if len(service.backends) == 1 {
			for providerID := range service.backends {
				service.defaultProvider = providerID
			}
			return service, nil
		}
		return nil, ErrProviderRequired
	}
	if _, ok := service.backends[service.defaultProvider]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotConfigured, service.defaultProvider)
	}
	return service, nil
}

func (s *Service) Enabled() bool {
	return s != nil && strings.TrimSpace(s.defaultProvider) != "" && len(s.backends) > 0
}

func (s *Service) DefaultProvider() string {
	if s == nil {
		return ""
	}
	return s.defaultProvider
}

func (s *Service) Search(ctx context.Context, providerID string, req Request) (Response, error) {
	if !s.Enabled() {
		return Response{}, ErrProviderNotConfigured
	}
	if err := req.Validate(); err != nil {
		return Response{}, err
	}
	resolvedProvider := normalizeProviderID(providerID)
	if resolvedProvider == "" {
		resolvedProvider = s.defaultProvider
	}
	if resolvedProvider == "" {
		return Response{}, ErrProviderRequired
	}
	backend, ok := s.backends[resolvedProvider]
	if !ok {
		return Response{}, fmt.Errorf("%w: %s", ErrProviderNotConfigured, resolvedProvider)
	}
	response, err := backend.Search(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if strings.TrimSpace(response.Provider) == "" {
		response.Provider = resolvedProvider
	}
	return response, nil
}

func normalizeProviderID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
