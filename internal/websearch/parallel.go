package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ParallelBackendConfig struct {
	ID        string
	APIKey    string
	BaseURL   string
	TimeoutMS int
}

type ParallelBackend struct {
	id      string
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewParallelBackend(config ParallelBackendConfig) *ParallelBackend {
	timeout := time.Duration(config.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &ParallelBackend{
		id:      normalizeProviderID(config.ID),
		apiKey:  strings.TrimSpace(config.APIKey),
		baseURL: strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (b *ParallelBackend) ID() string {
	if b == nil {
		return ""
	}
	return b.id
}

func (b *ParallelBackend) Search(ctx context.Context, req Request) (Response, error) {
	if b == nil {
		return Response{}, ErrProviderNotConfigured
	}
	if err := req.Validate(); err != nil {
		return Response{}, err
	}

	endpoint := b.baseURL + "/v1/search"
	payload := parallelSearchRequest{
		Objective:     strings.TrimSpace(req.Query),
		SearchQueries: []string{strings.TrimSpace(req.Query)},
		AdvancedSettings: parallelAdvancedSettings{
			MaxResults: req.Limit,
		},
	}
	if policy := buildParallelSourcePolicy(req); policy != nil {
		payload.AdvancedSettings.SourcePolicy = policy
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "kodacode/web_search")
	request.Header.Set("x-api-key", b.apiKey)

	httpClient := b.client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return Response{}, err
	}
	defer response.Body.Close() //nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return Response{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Response{}, fmt.Errorf("parallel search failed: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(bytes.TrimSpace(data))))
	}

	var decoded parallelSearchResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Response{}, err
	}

	results := make([]Result, 0, len(decoded.Results))
	for _, candidate := range decoded.Results {
		results = append(results, Result{
			Title:       strings.TrimSpace(candidate.Title),
			URL:         strings.TrimSpace(candidate.URL),
			Snippet:     parallelSnippet(candidate.Excerpts),
			Domain:      domainFromURL(candidate.URL),
			PublishedAt: strings.TrimSpace(candidate.PublishDate),
		})
	}

	return Response{
		Provider:  b.id,
		RequestID: strings.TrimSpace(decoded.SearchID),
		Notice:    parallelWarningsNotice(decoded.Warnings),
		Results:   results,
	}, nil
}

type parallelSearchRequest struct {
	SearchQueries    []string                 `json:"search_queries"`
	Objective        string                   `json:"objective,omitempty"`
	AdvancedSettings parallelAdvancedSettings `json:"advanced_settings,omitempty"`
}

type parallelAdvancedSettings struct {
	SourcePolicy *parallelSourcePolicy `json:"source_policy,omitempty"`
	MaxResults   int                   `json:"max_results,omitempty"`
}

type parallelSourcePolicy struct {
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	AfterDate      string   `json:"after_date,omitempty"`
}

type parallelSearchResponse struct {
	SearchID string                 `json:"search_id"`
	Results  []parallelSearchResult `json:"results"`
	Warnings []parallelWarning      `json:"warnings"`
}

type parallelSearchResult struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	PublishDate string   `json:"publish_date"`
	Excerpts    []string `json:"excerpts"`
}

type parallelWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func buildParallelSourcePolicy(req Request) *parallelSourcePolicy {
	if len(req.Domains) == 0 && len(req.ExcludeDomains) == 0 && req.FreshnessDays <= 0 {
		return nil
	}
	policy := &parallelSourcePolicy{
		IncludeDomains: append([]string(nil), req.Domains...),
		ExcludeDomains: append([]string(nil), req.ExcludeDomains...),
	}
	if req.FreshnessDays > 0 {
		policy.AfterDate = time.Now().UTC().AddDate(0, 0, -req.FreshnessDays).Format(time.DateOnly)
	}
	return policy
}

func parallelSnippet(excerpts []string) string {
	if len(excerpts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(excerpts))
	for _, excerpt := range excerpts {
		if trimmed := strings.TrimSpace(excerpt); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return truncateSnippet(strings.Join(parts, " "))
}

func parallelWarningsNotice(warnings []parallelWarning) string {
	if len(warnings) == 0 {
		return ""
	}
	parts := make([]string, 0, len(warnings))
	seen := map[string]struct{}{}
	for _, warning := range warnings {
		part := strings.TrimSpace(warning.Message)
		if part == "" {
			part = strings.TrimSpace(warning.Code)
		}
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}
