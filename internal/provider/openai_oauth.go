package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	openAIOAuthClientID   = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultOpenAITokenURL = "https://auth.openai.com/oauth/token"
)

type openAITokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func refreshOpenAIOAuth(
	ctx context.Context,
	httpClient *http.Client,
	tokenURL string,
	entry AuthEntry,
) (*AuthEntry, error) {
	if strings.TrimSpace(entry.Refresh) == "" {
		return nil, ErrOpenAIOAuthCredentialsRequired
	}
	if strings.TrimSpace(tokenURL) == "" {
		tokenURL = defaultOpenAITokenURL
	}

	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": entry.Refresh,
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
		return nil, fmt.Errorf("openai oauth refresh: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		message, readErr := readOpenAIError(resp.Body)
		if readErr != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				return nil, errors.Join(readErr, closeErr)
			}
			return nil, readErr
		}
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("openai oauth refresh: close body: %w", closeErr)
		}
		return nil, newProviderHTTPError("openai oauth refresh", resp.StatusCode, message, resp.Header)
	}

	var payload openAITokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("openai oauth refresh: decode: %w", err)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		return nil, fmt.Errorf("openai oauth refresh: close body: %w", closeErr)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return nil, fmt.Errorf("openai oauth refresh: access_token is required")
	}

	refresh := payload.RefreshToken
	if strings.TrimSpace(refresh) == "" {
		refresh = entry.Refresh
	}

	return &AuthEntry{
		Type:          AuthTypeOAuth,
		Access:        payload.AccessToken,
		Refresh:       refresh,
		Expires:       time.Now().UnixMilli() + payload.ExpiresIn*1000,
		Project:       entry.Project,
		AccountID:     extractOpenAIAccountID(payload.IDToken),
		ClientVersion: entry.ClientVersion,
	}, nil
}

func extractOpenAIAccountID(idToken string) string {
	if idToken == "" {
		return ""
	}

	parts := strings.SplitN(idToken, ".", 3)
	if len(parts) < 2 {
		return ""
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	if accountID, ok := claims["chatgpt_account_id"].(string); ok && accountID != "" {
		return accountID
	}
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if accountID, ok := auth["chatgpt_account_id"].(string); ok && accountID != "" {
			return accountID
		}
	}
	if orgs, ok := claims["organizations"].([]any); ok && len(orgs) > 0 {
		if org, ok := orgs[0].(map[string]any); ok {
			if accountID, ok := org["id"].(string); ok && accountID != "" {
				return accountID
			}
		}
	}
	return ""
}
