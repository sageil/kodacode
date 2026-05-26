// Package websearch provides web search backend implementations.
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ExaBackendConfig struct {
	ID        string
	APIKey    string
	BaseURL   string
	TimeoutMS int
}

type ExaBackend struct {
	id      string
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewExaBackend(config ExaBackendConfig) *ExaBackend {
	timeout := time.Duration(config.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &ExaBackend{
		id:      normalizeProviderID(config.ID),
		apiKey:  strings.TrimSpace(config.APIKey),
		baseURL: strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (b *ExaBackend) ID() string {
	if b == nil {
		return ""
	}
	return b.id
}

func (b *ExaBackend) Search(ctx context.Context, req Request) (Response, error) {
	if b == nil {
		return Response{}, ErrProviderNotConfigured
	}
	if err := req.Validate(); err != nil {
		return Response{}, err
	}
	endpoint := b.baseURL + "/search"
	payload := exaSearchRequest{
		Query:          strings.TrimSpace(req.Query),
		NumResults:     req.Limit,
		IncludeDomains: append([]string(nil), req.Domains...),
		ExcludeDomains: append([]string(nil), req.ExcludeDomains...),
		Text:           true,
	}
	if req.FreshnessDays > 0 {
		payload.StartPublishedDate = time.Now().UTC().AddDate(0, 0, -req.FreshnessDays).Format(time.RFC3339Nano)
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
		return Response{}, fmt.Errorf("exa search failed: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(bytes.TrimSpace(data))))
	}

	var decoded exaSearchResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Response{}, err
	}
	results := make([]Result, 0, len(decoded.Results))
	for _, candidate := range decoded.Results {
		results = append(results, Result{
			Title:       strings.TrimSpace(candidate.Title),
			URL:         strings.TrimSpace(candidate.URL),
			Snippet:     exaSnippet(candidate),
			Domain:      domainFromURL(candidate.URL),
			PublishedAt: strings.TrimSpace(candidate.PublishedDate),
			Author:      strings.TrimSpace(candidate.Author),
			Score:       candidate.Score,
		})
	}
	return Response{
		Provider:  b.id,
		RequestID: strings.TrimSpace(decoded.RequestID),
		Results:   results,
	}, nil
}

type exaSearchRequest struct {
	Query              string   `json:"query"`
	NumResults         int      `json:"numResults,omitempty"`
	IncludeDomains     []string `json:"includeDomains,omitempty"`
	ExcludeDomains     []string `json:"excludeDomains,omitempty"`
	StartPublishedDate string   `json:"startPublishedDate,omitempty"`
	Text               bool     `json:"text,omitempty"`
}

type exaSearchResponse struct {
	RequestID string            `json:"requestId"`
	Results   []exaSearchResult `json:"results"`
}

type exaSearchResult struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	PublishedDate string  `json:"publishedDate"`
	Author        string  `json:"author"`
	Text          string  `json:"text"`
	Summary       string  `json:"summary"`
	Score         float64 `json:"score"`
}

func exaSnippet(result exaSearchResult) string {
	switch {
	case strings.TrimSpace(result.Summary) != "":
		return truncateSnippet(result.Summary)
	case strings.TrimSpace(result.Text) != "":
		return truncateSnippet(result.Text)
	default:
		return ""
	}
}

func truncateSnippet(value string) string {
	const maxSnippetRunes = 280

	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if normalized == "" {
		return ""
	}
	runes := []rune(normalized)
	if len(runes) <= maxSnippetRunes {
		return normalized
	}
	return strings.TrimSpace(string(runes[:maxSnippetRunes])) + "..."
}

func domainFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}
