package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequestGitHubCopilotDeviceCodeParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/device/code" {
			t.Fatalf("path = %q, want /device/code", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("client_id"); got != gitHubCopilotOAuthClientID {
			t.Fatalf("client_id = %q", got)
		}
		if got := r.Form.Get("scope"); got != "read:user" {
			t.Fatalf("scope = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "device-code",
			"user_code":        "ABCD-EFGH",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       900,
			"interval":         5,
		})
	}))
	defer server.Close()

	challenge, err := RequestGitHubCopilotDeviceCode(context.Background(), server.Client(), server.URL+"/device/code")
	if err != nil {
		t.Fatalf("RequestGitHubCopilotDeviceCode() error = %v", err)
	}
	if challenge.DeviceCode != "device-code" {
		t.Fatalf("device code = %q", challenge.DeviceCode)
	}
	if challenge.UserCode != "ABCD-EFGH" {
		t.Fatalf("user code = %q", challenge.UserCode)
	}
	if challenge.VerificationURL != "https://github.com/login/device" {
		t.Fatalf("verification url = %q", challenge.VerificationURL)
	}
	if challenge.ExpiresIn != 900 || challenge.Interval != 5 {
		t.Fatalf("challenge timing = %#v", challenge)
	}
}

func TestExchangeGitHubCopilotServiceTokenReturnsOAuthEntry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/copilot/token" {
			t.Fatalf("path = %q, want /copilot/token", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer github-refresh" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      "gho_live",
			"expires_at": int64(4102444800),
		})
	}))
	defer server.Close()

	entry, err := exchangeGitHubCopilotServiceToken(context.Background(), server.Client(), server.URL+"/copilot/token", "github-refresh")
	if err != nil {
		t.Fatalf("exchangeGitHubCopilotServiceToken() error = %v", err)
	}
	if entry.Type != AuthTypeOAuth {
		t.Fatalf("entry type = %q", entry.Type)
	}
	if entry.Access != "gho_live" {
		t.Fatalf("access = %q", entry.Access)
	}
	if entry.Refresh != "github-refresh" {
		t.Fatalf("refresh = %q", entry.Refresh)
	}
	if entry.Expires != 4102444800*1000 {
		t.Fatalf("expires = %d", entry.Expires)
	}
}

func TestGitHubCopilotClientRefreshesExpiredOAuthToken(t *testing.T) {
	store := NewAuthStoreAt(filepath.Join(t.TempDir(), "auth.yaml"))
	entry := AuthEntry{
		Type:    AuthTypeOAuth,
		Access:  "stale",
		Refresh: "github-refresh",
		Expires: time.Now().Add(-time.Minute).UnixMilli(),
	}
	if err := store.Set("github-copilot", entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/copilot/token":
			if got := r.Header.Get("Authorization"); got != "Bearer github-refresh" {
				t.Fatalf("refresh authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      "gho_fresh",
				"expires_at": int64(4102444800),
			})
		case "/responses":
			if got := r.Header.Get("Authorization"); got != "Bearer gho_fresh" {
				t.Fatalf("request authorization = %q", got)
			}
			if got := r.Header.Get("Editor-Plugin-Version"); got != defaultGitHubCopilotPluginVersion {
				t.Fatalf("editor plugin version = %q", got)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: response.output_text.delta\n"))
			_, _ = w.Write([]byte("data: {\"delta\":\"hello\"}\n\n"))
			_, _ = w.Write([]byte("event: response.completed\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		OAuth: &GitHubCopilotOAuthConfig{
			Entry:    entry,
			Store:    store,
			TokenURL: server.URL + "/copilot/token",
		},
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5-mini"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}

	refreshed := store.Get("github-copilot")
	if refreshed == nil {
		t.Fatal("Get() = nil, want refreshed oauth entry")
	}
	if refreshed.Access != "gho_fresh" {
		t.Fatalf("stored access = %q", refreshed.Access)
	}
	if refreshed.Refresh != "github-refresh" {
		t.Fatalf("stored refresh = %q", refreshed.Refresh)
	}
}

func TestFetchGitHubCopilotModelsRefreshesExpiredOAuthToken(t *testing.T) {
	store := NewAuthStoreAt(filepath.Join(t.TempDir(), "auth.yaml"))
	entry := AuthEntry{
		Type:    AuthTypeOAuth,
		Access:  "stale",
		Refresh: "github-refresh",
		Expires: time.Now().Add(-time.Minute).UnixMilli(),
	}
	if err := store.Set("github-copilot", entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/copilot/token":
			if got := r.Header.Get("Authorization"); got != "Bearer github-refresh" {
				t.Fatalf("refresh authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      "gho_fresh",
				"expires_at": int64(4102444800),
			})
		case "/models":
			if got := r.Header.Get("Authorization"); got != "Bearer gho_fresh" {
				t.Fatalf("request authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(gitHubCopilotModelsResponse{
				Data: []gitHubCopilotModelPayload{{
					ID:                 "gpt-5-mini",
					Name:               "GPT-5 Mini",
					ModelPickerEnabled: true,
					SupportedEndpoints: []string{"chat/completions"},
					Capabilities: struct {
						Family string `json:"family"`
						Limits struct {
							MaxContextWindowTokens int `json:"max_context_window_tokens"`
							MaxPromptTokens        int `json:"max_prompt_tokens"`
							MaxOutputTokens        int `json:"max_output_tokens"`
						} `json:"limits"`
						Supports struct {
							AdaptiveThinking  bool     `json:"adaptive_thinking"`
							MaxThinkingBudget int      `json:"max_thinking_budget"`
							MinThinkingBudget int      `json:"min_thinking_budget"`
							ReasoningEffort   []string `json:"reasoning_effort"`
							ToolCalls         bool     `json:"tool_calls"`
							Vision            bool     `json:"vision"`
						} `json:"supports"`
					}{
						Family: "gpt-5",
						Limits: struct {
							MaxContextWindowTokens int `json:"max_context_window_tokens"`
							MaxPromptTokens        int `json:"max_prompt_tokens"`
							MaxOutputTokens        int `json:"max_output_tokens"`
						}{
							MaxContextWindowTokens: 128000,
							MaxPromptTokens:        128000,
							MaxOutputTokens:        16384,
						},
						Supports: struct {
							AdaptiveThinking  bool     `json:"adaptive_thinking"`
							MaxThinkingBudget int      `json:"max_thinking_budget"`
							MinThinkingBudget int      `json:"min_thinking_budget"`
							ReasoningEffort   []string `json:"reasoning_effort"`
							ToolCalls         bool     `json:"tool_calls"`
							Vision            bool     `json:"vision"`
						}{
							ToolCalls: true,
						},
					},
				}},
			})
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	models, err := FetchGitHubCopilotModels(context.Background(), GitHubCopilotConfig{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		OAuth: &GitHubCopilotOAuthConfig{
			Entry:    entry,
			Store:    store,
			TokenURL: server.URL + "/copilot/token",
		},
	})
	if err != nil {
		t.Fatalf("FetchGitHubCopilotModels() error = %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt-5-mini" {
		t.Fatalf("models = %#v", models)
	}

	refreshed := store.Get("github-copilot")
	if refreshed == nil || refreshed.Access != "gho_fresh" {
		t.Fatalf("stored oauth = %#v", refreshed)
	}
}

func TestTryGitHubCopilotOAuthAccessTokenUsesDeviceCodeGrant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		values := url.Values(r.Form)
		if got := values.Get("client_id"); got != gitHubCopilotOAuthClientID {
			t.Fatalf("client_id = %q", got)
		}
		if got := values.Get("device_code"); got != "device-code" {
			t.Fatalf("device_code = %q", got)
		}
		if got := values.Get("grant_type"); !strings.Contains(got, "device_code") {
			t.Fatalf("grant_type = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "github-token"})
	}))
	defer server.Close()

	token, err := tryGitHubCopilotOAuthAccessToken(context.Background(), server.Client(), server.URL, "device-code")
	if err != nil {
		t.Fatalf("tryGitHubCopilotOAuthAccessToken() error = %v", err)
	}
	if token != "github-token" {
		t.Fatalf("token = %q", token)
	}
}
