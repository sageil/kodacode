package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	openaiClientID       = "app_EMoamEEZ73f0CkXaXp7hrann"
	openaiAuthURL        = "https://auth.openai.com/oauth/authorize"
	openaiTokenURL       = "https://auth.openai.com/oauth/token"
	openaiDeviceAuthURL  = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	openaiDeviceTokenURL = "https://auth.openai.com/api/accounts/deviceauth/token"
	openaiRedirectURI    = "http://localhost:1455/auth/callback"
	openaiScopes         = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	openaiCallbackPort   = 1455
)

// OpenAIOAuthAuthorizeURL builds the browser authorization URL for OpenAI OAuth.
func OpenAIOAuthAuthorizeURL(pkce PKCE) string {
	params := url.Values{
		"response_type":              {"code"},
		"client_id":                  {openaiClientID},
		"redirect_uri":              {openaiRedirectURI},
		"scope":                     {openaiScopes},
		"code_challenge":            {pkce.Challenge},
		"code_challenge_method":     {"S256"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"state":                     {pkce.Verifier},
		"originator":               {"kodacode"},
	}
	return openaiAuthURL + "?" + params.Encode()
}

// OpenAIOAuthListenForCode starts a local HTTP server on localhost:1455 and
// waits for the OAuth callback to deliver the authorization code. It validates
// the returned state parameter.
func OpenAIOAuthListenForCode(expectedState string) (string, error) {
	codeCh := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if state := r.URL.Query().Get("state"); state != expectedState {
			http.Error(w, "invalid state parameter", http.StatusBadRequest)
			log.Printf("openai oauth: ignored callback with invalid state")
			return
		}
		code := r.URL.Query().Get("code")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1>Authorization successful! You can close this tab.</h1>
			<script>setTimeout(function(){window.close()},2000)</script></body></html>`))
		codeCh <- code
	})

	srv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", openaiCallbackPort), Handler: mux}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	shutdownSrv := func() {
		sCtx, sCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer sCancel()
		_ = srv.Shutdown(sCtx)
	}

	select {
	case code := <-codeCh:
		shutdownSrv()
		if code == "" {
			return "", fmt.Errorf("openai oauth: empty code in callback")
		}
		return code, nil
	case err := <-errCh:
		return "", fmt.Errorf("openai oauth: server error: %w", err)
	case <-ctx.Done():
		shutdownSrv()
		return "", fmt.Errorf("openai oauth: timed out waiting for callback (5 minutes)")
	}
}

// openaiTokenResponse is the JSON body returned by the OpenAI token endpoint.
type openaiTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// OpenAIOAuthExchange exchanges an authorization code for tokens.
func OpenAIOAuthExchange(code, verifier string) (*AuthEntry, error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"client_id":     openaiClientID,
		"redirect_uri":  openaiRedirectURI,
		"code_verifier": verifier,
	})

	resp, err := http.Post(openaiTokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai oauth exchange: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai oauth exchange: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var tok openaiTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("openai oauth exchange: decode: %w", err)
	}

	accountID := extractAccountIDFromJWT(tok.IDToken)

	return &AuthEntry{
		Type:      AuthTypeOAuth,
		Access:    tok.AccessToken,
		Refresh:   tok.RefreshToken,
		Expires:   time.Now().UnixMilli() + tok.ExpiresIn*1000,
		AccountID: accountID,
	}, nil
}

// OpenAIOAuthRefresh uses a refresh token to obtain a new access token.
func OpenAIOAuthRefresh(refreshToken string) (*AuthEntry, error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     openaiClientID,
	})

	resp, err := http.Post(openaiTokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai oauth refresh: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai oauth refresh: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var tok openaiTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("openai oauth refresh: decode: %w", err)
	}

	// Preserve existing refresh token if the response doesn't include a new one.
	refresh := tok.RefreshToken
	if refresh == "" {
		refresh = refreshToken
	}

	accountID := extractAccountIDFromJWT(tok.IDToken)

	return &AuthEntry{
		Type:      AuthTypeOAuth,
		Access:    tok.AccessToken,
		Refresh:   refresh,
		Expires:   time.Now().UnixMilli() + tok.ExpiresIn*1000,
		AccountID: accountID,
	}, nil
}

// extractAccountIDFromJWT decodes the payload of a JWT id_token and extracts
// the ChatGPT account ID. There is no signature verification because we trust the token
// endpoint (received over HTTPS).
func extractAccountIDFromJWT(idToken string) string {
	if idToken == "" {
		return ""
	}
	parts := strings.SplitN(idToken, ".", 3)
	if len(parts) < 2 {
		return ""
	}

	// Base64url decode the payload (middle segment).
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	// Try various claim locations for the account ID.
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}

	// 1. Root-level chatgpt_account_id
	if id, ok := claims["chatgpt_account_id"].(string); ok && id != "" {
		return id
	}

	// 2. Nested under https://api.openai.com/auth
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if id, ok := auth["chatgpt_account_id"].(string); ok && id != "" {
			return id
		}
	}

	// 3. First organization ID
	if orgs, ok := claims["organizations"].([]any); ok && len(orgs) > 0 {
		if org, ok := orgs[0].(map[string]any); ok {
			if id, ok := org["id"].(string); ok && id != "" {
				return id
			}
		}
	}

	return ""
}
