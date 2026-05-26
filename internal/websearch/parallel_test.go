package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParallelBackendSearchMapsNormalizedRequestAndResponse(t *testing.T) {
	var gotMethod string
	var gotAPIKey string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAPIKey = r.Header.Get("x-api-key")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode(request body) error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"search_id": "search_123",
			"results": []map[string]any{{
				"url":          "https://example.com/code-agents",
				"title":        "Code Agents Survey",
				"publish_date": "2026-05-01",
				"excerpts": []string{
					"A survey of agent systems and workflows.",
					"Includes evaluation and deployment tradeoffs.",
				},
			}},
			"warnings": []map[string]any{{
				"message": "location ignored",
			}},
		})
	}))
	defer server.Close()

	backend := NewParallelBackend(ParallelBackendConfig{
		ID:        "parallel",
		APIKey:    "parallel-key",
		BaseURL:   server.URL,
		TimeoutMS: 1000,
	})
	response, err := backend.Search(context.Background(), Request{
		Query:          "code agents",
		Limit:          3,
		Domains:        []string{"arxiv.org"},
		ExcludeDomains: []string{"youtube.com"},
		FreshnessDays:  7,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotAPIKey != "parallel-key" {
		t.Fatalf("x-api-key = %q", gotAPIKey)
	}
	if gotBody["objective"] != "code agents" {
		t.Fatalf("request body = %#v", gotBody)
	}

	searchQueries, ok := gotBody["search_queries"].([]any)
	if !ok || len(searchQueries) != 1 || searchQueries[0] != "code agents" {
		t.Fatalf("request body = %#v", gotBody)
	}

	advancedSettings, ok := gotBody["advanced_settings"].(map[string]any)
	if !ok {
		t.Fatalf("request body = %#v", gotBody)
	}
	if advancedSettings["max_results"] != float64(3) {
		t.Fatalf("request body = %#v", gotBody)
	}

	sourcePolicy, ok := advancedSettings["source_policy"].(map[string]any)
	if !ok {
		t.Fatalf("request body = %#v", gotBody)
	}
	includeDomains, ok := sourcePolicy["include_domains"].([]any)
	if !ok || len(includeDomains) != 1 || includeDomains[0] != "arxiv.org" {
		t.Fatalf("request body = %#v", gotBody)
	}
	excludeDomains, ok := sourcePolicy["exclude_domains"].([]any)
	if !ok || len(excludeDomains) != 1 || excludeDomains[0] != "youtube.com" {
		t.Fatalf("request body = %#v", gotBody)
	}
	afterDate, _ := sourcePolicy["after_date"].(string)
	if afterDate == "" {
		t.Fatalf("request body = %#v", gotBody)
	}
	parsed, err := time.Parse(time.DateOnly, afterDate)
	if err != nil {
		t.Fatalf("Parse(after_date) error = %v", err)
	}
	lowerBound := time.Now().UTC().AddDate(0, 0, -8)
	upperBound := time.Now().UTC().AddDate(0, 0, -6)
	if parsed.Before(time.Date(lowerBound.Year(), lowerBound.Month(), lowerBound.Day(), 0, 0, 0, 0, time.UTC)) ||
		parsed.After(time.Date(upperBound.Year(), upperBound.Month(), upperBound.Day(), 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("after_date = %s", afterDate)
	}

	if response.Provider != "parallel" || response.RequestID != "search_123" {
		t.Fatalf("response = %#v", response)
	}
	if response.Notice != "location ignored" {
		t.Fatalf("response notice = %q", response.Notice)
	}
	if len(response.Results) != 1 {
		t.Fatalf("response results = %#v", response.Results)
	}
	result := response.Results[0]
	if result.URL != "https://example.com/code-agents" || result.Domain != "example.com" {
		t.Fatalf("result = %#v", result)
	}
	if result.Snippet == "" {
		t.Fatalf("result = %#v", result)
	}
}
