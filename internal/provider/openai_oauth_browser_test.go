package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBuildOpenAIOAuthAuthorizeURLIncludesPKCEAndOfflineAccess(t *testing.T) {
	rawURL, err := BuildOpenAIOAuthAuthorizeURL("http://localhost:1455/auth/callback", "state-123", "challenge-456", "")
	if err != nil {
		t.Fatalf("BuildOpenAIOAuthAuthorizeURL() error = %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	query := parsed.Query()
	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; got != defaultOpenAIAuthorizeURL {
		t.Fatalf("authorize endpoint = %q", got)
	}
	if got := query.Get("client_id"); got != openAIOAuthClientID {
		t.Fatalf("client_id = %q", got)
	}
	if got := query.Get("redirect_uri"); got != "http://localhost:1455/auth/callback" {
		t.Fatalf("redirect_uri = %q", got)
	}
	if got := query.Get("scope"); !strings.Contains(got, "offline_access") {
		t.Fatalf("scope = %q, want offline_access", got)
	}
	if got := query.Get("code_challenge"); got != "challenge-456" {
		t.Fatalf("code_challenge = %q", got)
	}
	if got := query.Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method = %q", got)
	}
	if got := query.Get("originator"); got != "codex_cli" {
		t.Fatalf("originator = %q", got)
	}
	if got := query.Get("id_token_add_organizations"); got != "true" {
		t.Fatalf("id_token_add_organizations = %q", got)
	}
	if got := query.Get("codex_cli_simplified_flow"); got != "true" {
		t.Fatalf("codex_cli_simplified_flow = %q", got)
	}
}

func TestExchangeOpenAIOAuthCodeStoresRefreshAndAccountID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if got := body["grant_type"]; got != "authorization_code" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := body["code"]; got != "auth-code" {
			t.Fatalf("code = %q", got)
		}
		if got := body["redirect_uri"]; got != "http://localhost:1455/auth/callback" {
			t.Fatalf("redirect_uri = %q", got)
		}
		if got := body["code_verifier"]; got != "verifier" {
			t.Fatalf("code_verifier = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_in":    3600,
			"id_token":      testJWTAccountID("acct_browser"),
		})
	}))
	defer server.Close()

	entry, err := ExchangeOpenAIOAuthCode(context.Background(), server.Client(), server.URL, OpenAIOAuthCodeExchangeRequest{
		Code:          "auth-code",
		RedirectURI:   "http://localhost:1455/auth/callback",
		CodeVerifier:  "verifier",
		ClientVersion: "1.2.3",
	})
	if err != nil {
		t.Fatalf("ExchangeOpenAIOAuthCode() error = %v", err)
	}
	if entry.Type != AuthTypeOAuth {
		t.Fatalf("entry.Type = %q", entry.Type)
	}
	if entry.Access != "access-token" || entry.Refresh != "refresh-token" {
		t.Fatalf("entry = %#v", entry)
	}
	if entry.AccountID != "acct_browser" {
		t.Fatalf("entry.AccountID = %q", entry.AccountID)
	}
	if entry.ClientVersion != "1.2.3" {
		t.Fatalf("entry.ClientVersion = %q", entry.ClientVersion)
	}
	if entry.Expires == 0 {
		t.Fatalf("entry.Expires = %d, want non-zero", entry.Expires)
	}
}
