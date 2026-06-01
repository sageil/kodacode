package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GitHubCopilotDeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURL string
	ExpiresIn       int
	Interval        int
}

type gitHubCopilotOAuthAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

var (
	errGitHubCopilotAuthorizationPending  = errors.New("authorization pending")
	errGitHubCopilotAuthorizationSlowDown = errors.New("authorization slow_down")
)

func RequestGitHubCopilotDeviceCode(
	ctx context.Context,
	httpClient *http.Client,
	deviceCodeURL string,
) (*GitHubCopilotDeviceCode, error) {
	deviceCodeURL = firstNonBlank(strings.TrimSpace(deviceCodeURL), defaultGitHubCopilotDeviceCodeURL)
	body := url.Values{}
	body.Set("client_id", gitHubCopilotOAuthClientID)
	body.Set("scope", "read:user")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyGitHubCopilotOAuthHeaders(req)

	resp, err := gitHubCopilotOAuthHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, fmt.Errorf("github copilot device code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		message, readErr := readGitHubCopilotOAuthError(resp.Body)
		if readErr != nil {
			return nil, readErr
		}
		return nil, fmt.Errorf("github copilot device code: HTTP %d: %s", resp.StatusCode, message)
	}

	var payload struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("github copilot device code: decode: %w", err)
	}

	challenge := &GitHubCopilotDeviceCode{
		DeviceCode:      strings.TrimSpace(payload.DeviceCode),
		UserCode:        strings.TrimSpace(payload.UserCode),
		VerificationURL: strings.TrimSpace(payload.VerificationURI),
		ExpiresIn:       payload.ExpiresIn,
		Interval:        payload.Interval,
	}
	switch {
	case challenge.DeviceCode == "":
		return nil, fmt.Errorf("github copilot device code: missing device_code")
	case challenge.UserCode == "":
		return nil, fmt.Errorf("github copilot device code: missing user_code")
	case challenge.VerificationURL == "":
		return nil, fmt.Errorf("github copilot device code: missing verification_uri")
	}
	return challenge, nil
}

func PollGitHubCopilotDeviceCode(
	ctx context.Context,
	httpClient *http.Client,
	challenge GitHubCopilotDeviceCode,
	accessTokenURL string,
	tokenURL string,
) (*AuthEntry, error) {
	interval := max(challenge.Interval, 5)
	deadline := time.Now().Add(time.Duration(max(challenge.ExpiresIn, 1)) * time.Second)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}

		githubToken, err := tryGitHubCopilotOAuthAccessToken(ctx, httpClient, accessTokenURL, challenge.DeviceCode)
		switch {
		case err == nil:
			return exchangeGitHubCopilotServiceToken(ctx, httpClient, tokenURL, githubToken)
		case errors.Is(err, errGitHubCopilotAuthorizationPending):
			continue
		case errors.Is(err, errGitHubCopilotAuthorizationSlowDown):
			interval += 5
			ticker.Reset(time.Duration(interval) * time.Second)
			continue
		default:
			return nil, err
		}
	}

	return nil, fmt.Errorf("github copilot authorization timed out")
}

func tryGitHubCopilotOAuthAccessToken(
	ctx context.Context,
	httpClient *http.Client,
	accessTokenURL string,
	deviceCode string,
) (string, error) {
	accessTokenURL = firstNonBlank(strings.TrimSpace(accessTokenURL), defaultGitHubCopilotAccessTokenURL)

	body := url.Values{}
	body.Set("client_id", gitHubCopilotOAuthClientID)
	body.Set("device_code", strings.TrimSpace(deviceCode))
	body.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, accessTokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyGitHubCopilotOAuthHeaders(req)

	resp, err := gitHubCopilotOAuthHTTPClient(httpClient).Do(req)
	if err != nil {
		return "", fmt.Errorf("github copilot device token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var payload gitHubCopilotOAuthAccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("github copilot device token: decode: %w", err)
	}

	switch strings.TrimSpace(payload.Error) {
	case "":
		if strings.TrimSpace(payload.AccessToken) == "" {
			return "", errGitHubCopilotAuthorizationPending
		}
		return strings.TrimSpace(payload.AccessToken), nil
	case "authorization_pending":
		return "", errGitHubCopilotAuthorizationPending
	case "slow_down":
		return "", errGitHubCopilotAuthorizationSlowDown
	default:
		return "", fmt.Errorf("github copilot device token: %s", payload.Error)
	}
}
