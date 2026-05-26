package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultOpenAIAuthorizeURL = "https://auth.openai.com/oauth/authorize"

type OpenAIOAuthCodeExchangeRequest struct {
	Code          string
	RedirectURI   string
	CodeVerifier  string
	ClientVersion string
}

func OpenAIOAuthCodeChallenge(codeVerifier string) string {
	sum := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func BuildOpenAIOAuthAuthorizeURL(redirectURI, state, codeChallenge, originator string) (string, error) {
	redirectURI = strings.TrimSpace(redirectURI)
	state = strings.TrimSpace(state)
	codeChallenge = strings.TrimSpace(codeChallenge)
	originator = strings.TrimSpace(originator)
	if redirectURI == "" {
		return "", fmt.Errorf("openai oauth authorize url: redirect uri is required")
	}
	if state == "" {
		return "", fmt.Errorf("openai oauth authorize url: state is required")
	}
	if codeChallenge == "" {
		return "", fmt.Errorf("openai oauth authorize url: code challenge is required")
	}
	if originator == "" {
		originator = "codex_cli"
	}
	endpoint, err := url.Parse(defaultOpenAIAuthorizeURL)
	if err != nil {
		return "", fmt.Errorf("openai oauth authorize url: %w", err)
	}
	query := endpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", openAIOAuthClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", "openid profile email offline_access")
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)
	query.Set("originator", originator)
	query.Set("id_token_add_organizations", "true")
	query.Set("codex_cli_simplified_flow", "true")
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func ExchangeOpenAIOAuthCode(
	ctx context.Context,
	httpClient *http.Client,
	tokenURL string,
	request OpenAIOAuthCodeExchangeRequest,
) (*AuthEntry, error) {
	if strings.TrimSpace(request.Code) == "" {
		return nil, fmt.Errorf("openai oauth exchange: code is required")
	}
	if strings.TrimSpace(request.RedirectURI) == "" {
		return nil, fmt.Errorf("openai oauth exchange: redirect uri is required")
	}
	if strings.TrimSpace(request.CodeVerifier) == "" {
		return nil, fmt.Errorf("openai oauth exchange: code verifier is required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(tokenURL) == "" {
		tokenURL = defaultOpenAITokenURL
	}

	body, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          strings.TrimSpace(request.Code),
		"redirect_uri":  strings.TrimSpace(request.RedirectURI),
		"code_verifier": strings.TrimSpace(request.CodeVerifier),
		"client_id":     openAIOAuthClientID,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai oauth exchange: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		message, readErr := readOpenAIError(resp.Body)
		if readErr != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				return nil, fmt.Errorf("openai oauth exchange: %w", errors.Join(readErr, closeErr))
			}
			return nil, readErr
		}
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("openai oauth exchange: close body: %w", closeErr)
		}
		return nil, newProviderHTTPError("openai oauth exchange", resp.StatusCode, message, resp.Header)
	}

	var payload openAITokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("openai oauth exchange: decode: %w", err)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		return nil, fmt.Errorf("openai oauth exchange: close body: %w", closeErr)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return nil, fmt.Errorf("openai oauth exchange: access_token is required")
	}

	return &AuthEntry{
		Type:          AuthTypeOAuth,
		Access:        payload.AccessToken,
		Refresh:       payload.RefreshToken,
		Expires:       time.Now().UnixMilli() + payload.ExpiresIn*1000,
		AccountID:     extractOpenAIAccountID(payload.IDToken),
		ClientVersion: strings.TrimSpace(request.ClientVersion),
	}, nil
}
