package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	gitHubCopilotOAuthClientID         = "Iv1.b507a08c87ecfe98"
	defaultGitHubCopilotDeviceCodeURL  = "https://github.com/login/device/code"
	defaultGitHubCopilotAccessTokenURL = "https://github.com/login/oauth/access_token"
	defaultGitHubCopilotTokenURL       = "https://api.github.com/copilot_internal/v2/token"
	defaultGitHubCopilotEditorVersion  = "vscode/1.120.0"
	defaultGitHubCopilotPluginVersion  = "copilot-chat/0.43.0"
)

var (
	ErrGitHubCopilotOAuthStoreRequired       = errors.New("github copilot oauth store is required")
	ErrGitHubCopilotOAuthCredentialsRequired = errors.New("github copilot oauth access or refresh token is required")
	ErrGitHubCopilotNotAvailable             = errors.New("github copilot is not available for this GitHub account")
)

type GitHubCopilotOAuthConfig struct {
	Entry          AuthEntry
	Store          *AuthStore
	DeviceCodeURL  string
	AccessTokenURL string
	TokenURL       string
}

type GitHubCopilotDeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURL string
	ExpiresIn       int
	Interval        int
}

type gitHubCopilotOAuthAuthorizer struct {
	mu         sync.Mutex
	providerID string
	entry      AuthEntry
	store      *AuthStore
	httpClient *http.Client
	tokenURL   string

	tokenUpdateAt      time.Time
	tokenUpdateSource  string
	tokenUpdateForced  bool
	tokenUpdateChanged bool
}

type gitHubCopilotOAuthAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

type gitHubCopilotOAuthServiceTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

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

func newGitHubCopilotOAuthAuthorizer(
	config GitHubCopilotOAuthConfig,
	httpClient *http.Client,
) (*gitHubCopilotOAuthAuthorizer, error) {
	if config.Store == nil {
		return nil, ErrGitHubCopilotOAuthStoreRequired
	}
	if strings.TrimSpace(config.Entry.Access) == "" && strings.TrimSpace(config.Entry.Refresh) == "" {
		return nil, ErrGitHubCopilotOAuthCredentialsRequired
	}
	return &gitHubCopilotOAuthAuthorizer{
		providerID: "github-copilot",
		entry:      config.Entry,
		store:      config.Store,
		httpClient: httpClient,
		tokenURL:   strings.TrimSpace(config.TokenURL),
	}, nil
}

func (a *gitHubCopilotOAuthAuthorizer) Authorize(ctx context.Context, req *http.Request) error {
	entry, err := a.currentEntry(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+entry.Access)
	applyGitHubCopilotCommonHeaders(req)
	return nil
}

func (a *gitHubCopilotOAuthAuthorizer) RefreshAuth(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.refreshLocked(ctx, true)
}

func (a *gitHubCopilotOAuthAuthorizer) AuthDebugState() providerAuthDebugState {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	state := providerAuthDebugState{
		ProviderID:         a.providerID,
		AccessHash:         authTokenHash(a.entry.Access),
		AccessExpiresAt:    a.entry.Expires,
		AccessExpiresInMs:  authExpiryRemainingMillis(a.entry.Expires, now),
		TokenUpdateSource:  strings.TrimSpace(a.tokenUpdateSource),
		TokenUpdateForced:  a.tokenUpdateForced,
		TokenUpdateChanged: a.tokenUpdateChanged,
	}
	if !a.tokenUpdateAt.IsZero() {
		state.TokenUpdateAt = a.tokenUpdateAt.UnixMilli()
		state.TokenUpdateAgeMs = now.Sub(a.tokenUpdateAt).Milliseconds()
	}
	return state
}

func (a *gitHubCopilotOAuthAuthorizer) currentEntry(ctx context.Context) (AuthEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.refreshLocked(ctx, false); err != nil {
		return AuthEntry{}, err
	}
	return a.entry, nil
}

func (a *gitHubCopilotOAuthAuthorizer) refreshLocked(ctx context.Context, force bool) error {
	if current := a.store.Get(a.providerID); current != nil && current.Type == AuthTypeOAuth {
		staleAccess := a.entry.Access
		if force && current.Access != a.entry.Access && !gitHubCopilotOAuthNeedsRefresh(*current) {
			a.entry = *current
			a.recordTokenUpdateLocked("store_force", force, staleAccess != a.entry.Access)
			return nil
		}
		a.entry = *current
		if staleAccess != a.entry.Access {
			a.recordTokenUpdateLocked("store_sync", force, true)
		}
	}
	if !force && !gitHubCopilotOAuthNeedsRefresh(a.entry) {
		return nil
	}

	staleAccess := a.entry.Access
	refreshed, err := refreshGitHubCopilotOAuth(ctx, a.httpClient, a.tokenURL, a.entry)
	if err != nil {
		if force {
			if current := a.store.Get(a.providerID); current != nil && current.Type == AuthTypeOAuth && current.Access != staleAccess && !gitHubCopilotOAuthNeedsRefresh(*current) {
				a.entry = *current
				a.recordTokenUpdateLocked("store_after_exchange_failure", force, staleAccess != a.entry.Access)
				return nil
			}
		}
		return err
	}
	if err := a.store.Set(a.providerID, *refreshed); err != nil {
		return fmt.Errorf("github copilot oauth: persist token: %w", err)
	}
	a.entry = *refreshed
	a.recordTokenUpdateLocked("oauth_exchange", force, staleAccess != a.entry.Access)
	return nil
}

func (a *gitHubCopilotOAuthAuthorizer) recordTokenUpdateLocked(source string, forced bool, changed bool) {
	a.tokenUpdateAt = time.Now()
	a.tokenUpdateSource = strings.TrimSpace(source)
	a.tokenUpdateForced = forced
	a.tokenUpdateChanged = changed
}

func gitHubCopilotOAuthNeedsRefresh(entry AuthEntry) bool {
	if strings.TrimSpace(entry.Access) == "" {
		return true
	}
	return entry.Expires > 0 && entry.Expires < time.Now().UnixMilli()+30_000
}

func refreshGitHubCopilotOAuth(
	ctx context.Context,
	httpClient *http.Client,
	tokenURL string,
	entry AuthEntry,
) (*AuthEntry, error) {
	if strings.TrimSpace(entry.Refresh) == "" {
		return nil, ErrGitHubCopilotOAuthCredentialsRequired
	}
	return exchangeGitHubCopilotServiceToken(ctx, httpClient, tokenURL, entry.Refresh)
}

var (
	errGitHubCopilotAuthorizationPending  = errors.New("authorization pending")
	errGitHubCopilotAuthorizationSlowDown = errors.New("authorization slow_down")
)

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
