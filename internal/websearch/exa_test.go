package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExaBackendSearchMapsNormalizedRequestAndResponse(t *testing.T) {
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
			"requestId": "req_123",
			"results": []map[string]any{{
				"title":         "Code Agents Survey",
				"url":           "https://example.com/code-agents",
				"publishedDate": "2026-05-01T00:00:00Z",
				"author":        "Ada",
				"text":          "A survey of agent systems and workflows.",
				"score":         0.91,
			}},
		})
	}))
	defer server.Close()

	backend := NewExaBackend(ExaBackendConfig{
		ID:        "exa",
		APIKey:    "exa-key",
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
	if gotAPIKey != "exa-key" {
		t.Fatalf("x-api-key = %q", gotAPIKey)
	}
	if gotBody["query"] != "code agents" {
		t.Fatalf("request body = %#v", gotBody)
	}
	if gotBody["numResults"] != float64(3) {
		t.Fatalf("request body = %#v", gotBody)
	}
	if gotBody["text"] != true {
		t.Fatalf("request body = %#v", gotBody)
	}
	startPublishedDate, _ := gotBody["startPublishedDate"].(string)
	if startPublishedDate == "" {
		t.Fatalf("request body = %#v", gotBody)
	}
	parsed, err := time.Parse(time.RFC3339Nano, startPublishedDate)
	if err != nil {
		t.Fatalf("Parse(startPublishedDate) error = %v", err)
	}
	if parsed.After(time.Now().UTC()) || parsed.Before(time.Now().UTC().AddDate(0, 0, -8)) {
		t.Fatalf("startPublishedDate = %s", startPublishedDate)
	}
	if response.Provider != "exa" || response.RequestID != "req_123" {
		t.Fatalf("response = %#v", response)
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
