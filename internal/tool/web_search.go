package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	websearchsvc "github.com/sageil/kodacode/internal/websearch"
)

const (
	WebSearchToolName     = "web_search"
	webSearchDefaultLimit = 5
	webSearchMaxLimit     = 10
)

var (
	ErrWebSearchQueryRequired        = errors.New("query is required")
	ErrWebSearchLimitInvalid         = fmt.Errorf("limit must be between 1 and %d", webSearchMaxLimit)
	ErrWebSearchFreshnessDaysInvalid = errors.New("freshness_days must be greater than zero")
	ErrWebSearchServiceRequired      = errors.New("web search service is required")
	ErrWebSearchProviderRequired     = errors.New("web search provider is required")
	ErrWebSearchProviderInvalid      = errors.New("provider must not be blank")
)

type WebSearchTool struct{}

type webSearchInput struct {
	Query          string
	Limit          int
	Domains        []string
	ExcludeDomains []string
	FreshnessDays  int
	Provider       string
	Notice         string
}

func NewWebSearchTool() WebSearchTool {
	return WebSearchTool{}
}

func (WebSearchTool) Definition() Definition {
	return Definition{
		Name:                WebSearchToolName,
		Description:         "Search the public web from a query and return candidate pages with lightweight metadata. Use this to discover URLs first, then use web_fetch on specific pages when you need actual page content. `max_results` is accepted as an alias for `limit`.",
		ProviderDescription: "Search the web from a query and return candidate page URLs plus lightweight metadata. Use web_fetch after this when you need page content. `max_results` is accepted as an alias for `limit`.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Natural-language web search query."},"limit":{"type":["integer","string","null"],"description":"Optional maximum results to return. Use null or omit this field to accept the default limit of 5. Values above 10 are clamped to 10."},"max_results":{"type":["integer","string","null"],"description":"Alias for limit. Prefer limit for new calls."},"domains":{"type":["array","string","null"],"items":{"type":"string"},"description":"Optional domains to include, such as [\"arxiv.org\",\"openai.com\"]. A single string is also accepted."},"exclude_domains":{"type":["array","string","null"],"items":{"type":"string"},"description":"Optional domains to exclude, such as [\"youtube.com\"]. A single string is also accepted."},"freshness_days":{"type":["integer","string","null"],"description":"Optional published-date recency filter in days. Use null or omit this field when no recency filter is needed."}},"required":["query"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"query":"latest research on code agents","limit":5,"domains":["arxiv.org","openai.com"],"exclude_domains":["youtube.com"],"freshness_days":30}`},
	}
}

func NormalizeWebSearchArguments(args json.RawMessage, service *websearchsvc.Service) (json.RawMessage, error) {
	if service == nil || !service.Enabled() {
		return nil, ErrWebSearchServiceRequired
	}
	var raw map[string]json.RawMessage
	if err := DecodeArgs(WebSearchToolName, args, &raw); err != nil {
		return nil, err
	}
	provider := service.DefaultProvider()
	if strings.TrimSpace(provider) == "" {
		return nil, ErrWebSearchProviderRequired
	}
	raw["provider"] = json.RawMessage(`"` + provider + `"`)
	normalized, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func (WebSearchTool) NormalizedInputKey(args json.RawMessage) (string, error) {
	input, err := parseWebSearchInput(args)
	if err != nil {
		return "", err
	}
	key, err := json.Marshal(struct {
		Query          string   `json:"query"`
		Limit          int      `json:"limit"`
		Domains        []string `json:"domains,omitempty"`
		ExcludeDomains []string `json:"exclude_domains,omitempty"`
		FreshnessDays  int      `json:"freshness_days,omitempty"`
		Provider       string   `json:"provider,omitempty"`
	}{
		Query:          input.Query,
		Limit:          input.Limit,
		Domains:        input.Domains,
		ExcludeDomains: input.ExcludeDomains,
		FreshnessDays:  input.FreshnessDays,
		Provider:       input.Provider,
	})
	if err != nil {
		return "", err
	}
	return string(key), nil
}

func (WebSearchTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	input, err := parseWebSearchInput(args)
	if err != nil {
		return Result{}, err
	}
	if ectx.WebSearch == nil || !ectx.WebSearch.Enabled() {
		return Result{}, ErrWebSearchServiceRequired
	}
	response, err := ectx.WebSearch.Search(ctx, input.Provider, websearchsvc.Request{
		Query:          input.Query,
		Limit:          input.Limit,
		Domains:        input.Domains,
		ExcludeDomains: input.ExcludeDomains,
		FreshnessDays:  input.FreshnessDays,
	})
	if err != nil {
		return Result{}, err
	}
	response.Notice = joinWebSearchNotice(input.Notice, response.Notice)
	structured, err := MarshalStructuredResult(structuredWebSearchResponse(response, input.Query))
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output:           formatWebSearchResponse(response, input.Query),
		StructuredResult: structured,
		Execution:        &ExecutionRuntime{Backend: response.Provider},
	}, nil
}

func parseWebSearchInput(args json.RawMessage) (_ webSearchInput, err error) {
	defer func() {
		err = normalizeToolInputError(WebSearchToolName, err)
	}()
	var raw struct {
		Query          string          `json:"query"`
		Limit          json.RawMessage `json:"limit"`
		MaxResults     json.RawMessage `json:"max_results"`
		Domains        json.RawMessage `json:"domains"`
		ExcludeDomains json.RawMessage `json:"exclude_domains"`
		FreshnessDays  json.RawMessage `json:"freshness_days"`
		Provider       *string         `json:"provider"`
	}
	if err := DecodeArgs(WebSearchToolName, args, &raw); err != nil {
		return webSearchInput{}, err
	}
	query := strings.TrimSpace(raw.Query)
	if query == "" {
		return webSearchInput{}, ErrWebSearchQueryRequired
	}
	limit := webSearchDefaultLimit
	notice := ""
	limitRaw := raw.Limit
	limitField := "limit"
	if !hasNonNullRawJSON(limitRaw) && hasNonNullRawJSON(raw.MaxResults) {
		limitRaw = raw.MaxResults
		limitField = "max_results"
	}
	if value, ok, err := decodeOptionalIntArg(WebSearchToolName, limitRaw, limitField); err != nil {
		return webSearchInput{}, err
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		return webSearchInput{}, ErrWebSearchLimitInvalid
	}
	if limit > webSearchMaxLimit {
		limit = webSearchMaxLimit
		notice = fmt.Sprintf("limit clamped to %d", webSearchMaxLimit)
	}
	domains, _, err := decodeOptionalStringArrayArg(WebSearchToolName, raw.Domains, "domains")
	if err != nil {
		return webSearchInput{}, err
	}
	excludeDomains, _, err := decodeOptionalStringArrayArg(WebSearchToolName, raw.ExcludeDomains, "exclude_domains")
	if err != nil {
		return webSearchInput{}, err
	}
	freshnessDays := 0
	if value, ok, err := decodeOptionalIntArg(WebSearchToolName, raw.FreshnessDays, "freshness_days"); err != nil {
		return webSearchInput{}, err
	} else if ok {
		if value <= 0 {
			return webSearchInput{}, ErrWebSearchFreshnessDaysInvalid
		}
		freshnessDays = value
	}
	provider := ""
	if raw.Provider != nil {
		provider = strings.ToLower(strings.TrimSpace(*raw.Provider))
		if provider == "" {
			return webSearchInput{}, ErrWebSearchProviderInvalid
		}
	}
	return webSearchInput{
		Query:          query,
		Limit:          limit,
		Domains:        normalizeWebSearchDomains(domains),
		ExcludeDomains: normalizeWebSearchDomains(excludeDomains),
		FreshnessDays:  freshnessDays,
		Provider:       provider,
		Notice:         notice,
	}, nil
}

func normalizeWebSearchDomains(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		candidate := normalizeWebSearchDomain(value)
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		normalized = append(normalized, candidate)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeWebSearchDomain(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil && parsed != nil && strings.TrimSpace(parsed.Hostname()) != "" {
			return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		}
	}
	if idx := strings.IndexRune(trimmed, '/'); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(trimmed), "."))
}

func joinWebSearchNotice(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "; ")
}

type webSearchStructuredResult struct {
	Provider string                           `json:"provider"`
	Query    string                           `json:"query"`
	Notice   string                           `json:"notice,omitempty"`
	Results  []webSearchStructuredResultEntry `json:"results"`
}

type webSearchStructuredResultEntry struct {
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Domain      string  `json:"domain"`
	PublishedAt string  `json:"published_at,omitempty"`
	Author      string  `json:"author,omitempty"`
	Score       float64 `json:"score,omitempty"`
}

func structuredWebSearchResponse(response websearchsvc.Response, query string) webSearchStructuredResult {
	out := webSearchStructuredResult{
		Provider: response.Provider,
		Query:    query,
		Notice:   response.Notice,
		Results:  make([]webSearchStructuredResultEntry, 0, len(response.Results)),
	}
	for _, result := range response.Results {
		out.Results = append(out.Results, webSearchStructuredResultEntry{
			Title:       result.Title,
			URL:         result.URL,
			Snippet:     result.Snippet,
			Domain:      result.Domain,
			PublishedAt: result.PublishedAt,
			Author:      result.Author,
			Score:       result.Score,
		})
	}
	return out
}

func formatWebSearchResponse(response websearchsvc.Response, query string) string {
	lines := make([]string, 0, len(response.Results)*4+3)
	lines = append(lines, fmt.Sprintf(`Search results for %q`, query))
	if strings.TrimSpace(response.Provider) != "" {
		lines = append(lines, "Provider: "+response.Provider)
	}
	if strings.TrimSpace(response.Notice) != "" {
		lines = append(lines, "Note: "+response.Notice)
	}
	if len(response.Results) == 0 {
		lines = append(lines, "No results found.")
		return strings.Join(lines, "\n")
	}
	for index, result := range response.Results {
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, firstNonBlankString(result.Title, result.URL)))
		lines = append(lines, "   "+result.URL)
		if result.Domain != "" {
			lines = append(lines, "   "+result.Domain)
		}
		if result.Snippet != "" {
			lines = append(lines, "   "+truncateOutputSnippet(result.Snippet))
		}
	}
	return strings.Join(lines, "\n")
}

func truncateOutputSnippet(value string) string {
	const maxOutputSnippetRunes = 160

	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if normalized == "" {
		return ""
	}
	runes := []rune(normalized)
	if len(runes) <= maxOutputSnippetRunes {
		return normalized
	}
	return strings.TrimSpace(string(runes[:maxOutputSnippetRunes])) + "..."
}

func firstNonBlankString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
