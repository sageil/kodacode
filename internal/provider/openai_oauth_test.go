package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAIClientStreamUsesOAuthHeaders(t *testing.T) {
	var authHeader string
	var accountHeader string
	var originatorHeader string
	var body []byte

	responseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		accountHeader = r.Header.Get("ChatGPT-Account-Id")
		originatorHeader = r.Header.Get("originator")
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer responseServer.Close()

	store := NewAuthStoreAt(filepath.Join(t.TempDir(), "auth.yaml"))
	entry := AuthEntry{
		Type:      AuthTypeOAuth,
		Access:    "oauth-access",
		Refresh:   "oauth-refresh",
		Expires:   time.Now().Add(time.Hour).UnixMilli(),
		AccountID: "acct_123",
	}
	if err := store.Set("openai", entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	client, err := NewOpenAIClient(OpenAIConfig{
		BaseURL: responseServer.URL,
		OAuth: &OpenAIOAuthConfig{
			Entry: entry,
			Store: store,
		},
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		MaxOutputTokens: 8192,
		Instructions:    "be precise",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}

	if authHeader != "Bearer oauth-access" {
		t.Fatalf("authorization = %q", authHeader)
	}
	if accountHeader != "acct_123" {
		t.Fatalf("ChatGPT-Account-Id = %q", accountHeader)
	}
	if originatorHeader != "kodacode" {
		t.Fatalf("originator = %q", originatorHeader)
	}
	if bytes.Contains(body, []byte("max_output_tokens")) {
		t.Fatalf("oauth responses body contains unsupported max_output_tokens: %s", body)
	}
}

func TestOpenAIClientStreamRefreshesExpiredOAuthToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&map[string]any{})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-access","refresh_token":"fresh-refresh","expires_in":3600,"id_token":"` + testJWTAccountID("acct_999") + `"}`))
	}))
	defer tokenServer.Close()

	var authHeader string
	var accountHeader string
	responseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		accountHeader = r.Header.Get("ChatGPT-Account-Id")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer responseServer.Close()

	store := NewAuthStoreAt(filepath.Join(t.TempDir(), "auth.yaml"))
	entry := AuthEntry{
		Type:    AuthTypeOAuth,
		Access:  "stale-access",
		Refresh: "stale-refresh",
		Expires: time.Now().Add(-time.Minute).UnixMilli(),
	}
	if err := store.Set("openai", entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	client, err := NewOpenAIClient(OpenAIConfig{
		BaseURL: responseServer.URL,
		OAuth: &OpenAIOAuthConfig{
			Entry:    entry,
			Store:    store,
			TokenURL: tokenServer.URL,
		},
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}

	if authHeader != "Bearer fresh-access" {
		t.Fatalf("authorization = %q", authHeader)
	}
	if accountHeader != "acct_999" {
		t.Fatalf("ChatGPT-Account-Id = %q", accountHeader)
	}

	refreshed := store.Get("openai")
	if refreshed == nil {
		t.Fatal("Get() = nil, want refreshed entry")
	}
	if refreshed.Access != "fresh-access" || refreshed.Refresh != "fresh-refresh" {
		t.Fatalf("refreshed entry = %#v", refreshed)
	}
	if refreshed.AccountID != "acct_999" || refreshed.Expires <= time.Now().UnixMilli() {
		t.Fatalf("refreshed entry = %#v", refreshed)
	}
}

func TestOpenAIClientStreamPreservesRetryableOAuthRefreshErrors(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After-Ms", "750")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
	}))
	defer tokenServer.Close()

	store := NewAuthStoreAt(filepath.Join(t.TempDir(), "auth.yaml"))
	entry := AuthEntry{
		Type:    AuthTypeOAuth,
		Access:  "stale-access",
		Refresh: "stale-refresh",
		Expires: time.Now().Add(-time.Minute).UnixMilli(),
	}
	if err := store.Set("openai", entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	client, err := NewOpenAIClient(OpenAIConfig{
		BaseURL: "https://api.openai.com/v1/responses",
		OAuth: &OpenAIOAuthConfig{
			Entry:    entry,
			Store:    store,
			TokenURL: tokenServer.URL,
		},
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}

	_, err = client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("Stream() error = nil, want oauth refresh error")
	}

	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T %v, want *ProviderError", err, err)
	}
	if providerErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status code = %d, want 429", providerErr.StatusCode)
	}
	if !providerErr.Retryable {
		t.Fatal("retryable = false, want true")
	}
	if providerErr.RetryAfter != 750*time.Millisecond {
		t.Fatalf("retry after = %s, want 750ms", providerErr.RetryAfter)
	}
}

func testJWTAccountID(accountID string) string {
	payload, _ := json.Marshal(map[string]string{"chatgpt_account_id": accountID})
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
