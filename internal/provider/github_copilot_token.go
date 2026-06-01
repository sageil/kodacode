package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type gitHubCopilotOAuthServiceTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

func exchangeGitHubCopilotServiceToken(
	ctx context.Context,
	httpClient *http.Client,
	tokenURL string,
	githubToken string,
) (*AuthEntry, error) {
	tokenURL = firstNonBlank(strings.TrimSpace(tokenURL), defaultGitHubCopilotTokenURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(githubToken))
	applyGitHubCopilotCommonHeaders(req)

	resp, err := gitHubCopilotOAuthHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("github copilot token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		return nil, ErrGitHubCopilotNotAvailable
	}
	if resp.StatusCode != http.StatusOK {
		message, readErr := readGitHubCopilotOAuthError(resp.Body)
		if readErr != nil {
			return nil, readErr
		}
		return nil, fmt.Errorf("github copilot token exchange: HTTP %d: %s", resp.StatusCode, message)
	}

	var payload gitHubCopilotOAuthServiceTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("github copilot token exchange: decode: %w", err)
	}
	if strings.TrimSpace(payload.Token) == "" {
		return nil, fmt.Errorf("github copilot token exchange: token is required")
	}

	return &AuthEntry{
		Type:    AuthTypeOAuth,
		Access:  strings.TrimSpace(payload.Token),
		Refresh: strings.TrimSpace(githubToken),
		Expires: payload.ExpiresAt * 1000,
	}, nil
}

func applyGitHubCopilotCommonHeaders(req *http.Request) {
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Editor-Version", defaultGitHubCopilotEditorVersion)
	req.Header.Set("Editor-Plugin-Version", defaultGitHubCopilotPluginVersion)
	req.Header.Set("User-Agent", gitHubCopilotUserAgent())
}

func applyGitHubCopilotOAuthHeaders(req *http.Request) {
	req.Header.Set("User-Agent", gitHubCopilotUserAgent())
}

func gitHubCopilotUserAgent() string {
	version := strings.TrimPrefix(strings.TrimSpace(defaultGitHubCopilotPluginVersion), "copilot-chat/")
	if version == "" {
		return "GitHubCopilotChat"
	}
	return "GitHubCopilotChat/" + version
}

func readGitHubCopilotOAuthError(body io.Reader) (string, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "unknown error", nil
	}
	return text, nil
}

func gitHubCopilotOAuthHTTPClient(httpClient *http.Client) *http.Client {
	if httpClient != nil {
		return httpClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}
