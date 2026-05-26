package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchGitHubCopilotModelsFiltersAndMergesModelPickerEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer gho_test" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Copilot-Integration-Id"); got != "vscode-chat" {
			t.Fatalf("copilot integration id = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{"id":"gpt-5","name":"GPT-5","model_picker_enabled":false},
				{"id":"gpt-5","name":"GPT-5","model_picker_enabled":true},
				{"id":"claude-sonnet-4","name":"Claude Sonnet 4","model_picker_enabled":true},
				{"id":"hidden-model","name":"Hidden","model_picker_enabled":false}
			]
		}`))
	}))
	defer server.Close()

	models, err := FetchGitHubCopilotModels(context.Background(), GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("FetchGitHubCopilotModels() error = %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("models = %#v", models)
	}
	if models[0].ID != "gpt-5" && models[1].ID != "gpt-5" {
		t.Fatalf("models = %#v, want gpt-5 present", models)
	}
	if models[0].ID == "hidden-model" || models[1].ID == "hidden-model" {
		t.Fatalf("models = %#v, hidden model should be filtered", models)
	}
}

func TestFetchGitHubCopilotModelsUsesBaseURLRootWhenConfiguredWithResponsesEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/models" {
			t.Fatalf("path = %q, want /models", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5","name":"GPT-5","model_picker_enabled":true}]}`))
	}))
	defer server.Close()

	models, err := FetchGitHubCopilotModels(context.Background(), GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: strings.TrimRight(server.URL, "/") + "/responses",
	})
	if err != nil {
		t.Fatalf("FetchGitHubCopilotModels() error = %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt-5" {
		t.Fatalf("models = %#v", models)
	}
}
