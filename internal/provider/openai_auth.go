package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

const defaultOpenAIOAuthBaseURL = "https://chatgpt.com/backend-api/codex"

var (
	ErrOpenAIAuthRequired             = errors.New("openai api key or oauth auth is required")
	ErrOpenAIOAuthStoreRequired       = errors.New("openai oauth store is required")
	ErrOpenAIOAuthCredentialsRequired = errors.New("openai oauth access or refresh token is required")
)

type OpenAIOAuthConfig struct {
	Entry    AuthEntry
	Store    *AuthStore
	TokenURL string
}

type openAIRequestAuthorizer interface {
	Authorize(context.Context, *http.Request) error
}

type openAIAPIKeyAuthorizer struct {
	apiKey string
}

type openAINoopAuthorizer struct{}

type openAIOAuthAuthorizer struct {
	mu         sync.Mutex
	providerID string
	entry      AuthEntry
	store      *AuthStore
	httpClient *http.Client
	tokenURL   string
}

func DefaultOpenAIOAuthBaseURL() string {
	return defaultOpenAIOAuthBaseURL
}

func newOpenAIAuthorizer(config OpenAIConfig, httpClient *http.Client) (openAIRequestAuthorizer, error) {
	if config.OAuth != nil {
		return newOpenAIOAuthAuthorizer(*config.OAuth, httpClient)
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, ErrOpenAIAuthRequired
	}
	return openAIAPIKeyAuthorizer{apiKey: config.APIKey}, nil
}

func newOpenAIOAuthAuthorizer(config OpenAIOAuthConfig, httpClient *http.Client) (*openAIOAuthAuthorizer, error) {
	if config.Store == nil {
		return nil, ErrOpenAIOAuthStoreRequired
	}
	if strings.TrimSpace(config.Entry.Access) == "" && strings.TrimSpace(config.Entry.Refresh) == "" {
		return nil, ErrOpenAIOAuthCredentialsRequired
	}
	return &openAIOAuthAuthorizer{
		providerID: "openai",
		entry:      config.Entry,
		store:      config.Store,
		httpClient: httpClient,
		tokenURL:   config.TokenURL,
	}, nil
}

func (a openAIAPIKeyAuthorizer) Authorize(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (openAINoopAuthorizer) Authorize(context.Context, *http.Request) error {
	return nil
}

func (a *openAIOAuthAuthorizer) Authorize(ctx context.Context, req *http.Request) error {
	entry, err := a.currentEntry(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+entry.Access)
	req.Header.Set("originator", "kodacode")
	req.Header.Set("User-Agent", openAIUserAgent())
	if entry.AccountID != "" {
		req.Header.Set("ChatGPT-Account-Id", entry.AccountID)
	}
	return nil
}

func (a *openAIOAuthAuthorizer) RefreshAuth(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.refreshLocked(ctx, true)
}

func (a *openAIOAuthAuthorizer) currentEntry(ctx context.Context) (AuthEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.refreshLocked(ctx, false); err != nil {
		return AuthEntry{}, err
	}
	return a.entry, nil
}

func (a *openAIOAuthAuthorizer) refreshLocked(ctx context.Context, force bool) error {
	if current := a.store.Get(a.providerID); current != nil && current.Type == AuthTypeOAuth {
		if force && current.Access != a.entry.Access && !openAIOAuthNeedsRefresh(*current) {
			a.entry = *current
			return nil
		}
		a.entry = *current
	}
	if !force && !openAIOAuthNeedsRefresh(a.entry) {
		return nil
	}

	staleAccess := a.entry.Access
	refreshed, err := refreshOpenAIOAuth(ctx, a.httpClient, a.tokenURL, a.entry)
	if err != nil {
		if force {
			if current := a.store.Get(a.providerID); current != nil && current.Type == AuthTypeOAuth && current.Access != staleAccess && !openAIOAuthNeedsRefresh(*current) {
				a.entry = *current
				return nil
			}
		}
		return err
	}
	if refreshed.AccountID == "" {
		refreshed.AccountID = a.entry.AccountID
	}
	if refreshed.Project == "" {
		refreshed.Project = a.entry.Project
	}
	if err := a.store.Set(a.providerID, *refreshed); err != nil {
		return fmt.Errorf("openai oauth: persist token: %w", err)
	}
	a.entry = *refreshed
	return nil
}

func openAIOAuthNeedsRefresh(entry AuthEntry) bool {
	if strings.TrimSpace(entry.Access) == "" {
		return true
	}
	return entry.Expires > 0 && entry.Expires < time.Now().UnixMilli()+30_000
}

func openAIUserAgent() string {
	return fmt.Sprintf("kodacode/1.0 (%s %s)", runtime.GOOS, runtime.GOARCH)
}
