package provider

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type openAICopilotAuthorizer struct {
	token string
}

func (a openAICopilotAuthorizer) Authorize(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.token)
	applyGitHubCopilotCommonHeaders(req)
	return nil
}

func (a openAICopilotAuthorizer) AuthDebugState() providerAuthDebugState {
	return providerAuthDebugState{
		ProviderID: "github-copilot",
		AccessHash: authTokenHash(a.token),
	}
}

func newGitHubCopilotAuthorizer(config GitHubCopilotConfig, httpClient *http.Client) (openAIRequestAuthorizer, error) {
	if config.OAuth != nil {
		return newGitHubCopilotOAuthAuthorizer(*config.OAuth, httpClient)
	}
	if strings.TrimSpace(config.Token) == "" {
		return nil, ErrGitHubCopilotTokenRequired
	}
	return openAICopilotAuthorizer{token: config.Token}, nil
}

func newOpenAICompatibleAuthorizer(apiKey, baseURL string) (openAIRequestAuthorizer, error) {
	if strings.TrimSpace(apiKey) != "" {
		return openAIAPIKeyAuthorizer{apiKey: apiKey}, nil
	}
	if allowsAnonymousCompatibleAuth(baseURL) {
		return openAINoopAuthorizer{}, nil
	}
	return nil, ErrOpenAICompatibleAPIKeyRequired
}

func allowsAnonymousCompatibleAuth(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, host)
}
