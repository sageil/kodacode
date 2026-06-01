package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
)

var (
	ErrOpenAICompatibleAPIKeyRequired  = errors.New("openai compatible api key is required")
	ErrOpenAICompatibleBaseURLRequired = errors.New("openai compatible base_url is required")
	ErrGitHubCopilotTokenRequired      = errors.New("github copilot token is required")
)

type OpenAICompatibleConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type GitHubCopilotConfig struct {
	Token      string
	BaseURL    string
	OAuth      *GitHubCopilotOAuthConfig
	HTTPClient *http.Client
}

type OpenAICompatibleClient struct {
	authorizer             openAIRequestAuthorizer
	httpClient             *http.Client
	resolveAPIs            func(Request) []openAICompatibleAPI
	unsupportedInputTokens sync.Map
}

type openAICompatibleAPIMode string

const (
	openAICompatibleModeResponses       openAICompatibleAPIMode = "responses"
	openAICompatibleModeChatCompletions openAICompatibleAPIMode = "chat_completions"
	defaultGitHubCopilotBaseURL                                 = "https://api.githubcopilot.com"
)

type openAICompatibleAPI struct {
	Mode       openAICompatibleAPIMode
	Endpoint   string
	ErrorLabel string
}

func NewOpenAICompatibleClient(config OpenAICompatibleConfig) (*OpenAICompatibleClient, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, ErrOpenAICompatibleBaseURLRequired
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	authorizer, err := newOpenAICompatibleAuthorizer(config.APIKey, config.BaseURL)
	if err != nil {
		return nil, err
	}
	api := compatibleAPIFromBaseURL(config.BaseURL, "openai compatible")
	return &OpenAICompatibleClient{
		authorizer: authorizer,
		httpClient: httpClient,
		resolveAPIs: func(Request) []openAICompatibleAPI {
			return []openAICompatibleAPI{api}
		},
	}, nil
}

func NewGitHubCopilotClient(config GitHubCopilotConfig) (*OpenAICompatibleClient, error) {
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = defaultGitHubCopilotBaseURL
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	authorizer, err := newGitHubCopilotAuthorizer(config, httpClient)
	if err != nil {
		return nil, err
	}
	root := strings.TrimRight(baseURL, "/")
	return &OpenAICompatibleClient{
		authorizer: authorizer,
		httpClient: httpClient,
		resolveAPIs: func(req Request) []openAICompatibleAPI {
			return gitHubCopilotAPIs(root, req.Model.ModelID)
		},
	}, nil
}

func (c *OpenAICompatibleClient) Stream(ctx context.Context, req Request) (Stream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apis := c.resolveAPIs(req)
	if len(apis) == 0 {
		return nil, errors.New("openai compatible api is required")
	}

	var errs []error
	for idx, api := range apis {
		stream, err := c.streamWithAPI(ctx, req, api)
		if err == nil {
			return withRequestTrace(stream, openAICompatibleRequestTrace(req, api)), nil
		}
		errs = append(errs, err)
		if idx == len(apis)-1 || !shouldFallbackCompatibleAPI(api.Mode, err) {
			return nil, errors.Join(errs...)
		}
	}
	return nil, errors.Join(errs...)
}

func openAICompatibleRequestTrace(req Request, api openAICompatibleAPI) RequestTrace {
	trace := RequestTrace{
		APIMode: string(api.Mode),
	}
	switch api.Mode {
	case openAICompatibleModeResponses:
		trace.ParallelToolCalls = len(req.Tools) > 0
	case openAICompatibleModeChatCompletions:
		trace.ParallelToolCalls = len(req.Tools) > 0 && !omitGitHubCopilotParallelToolCalls(req.Model)
	}
	return trace
}

func (c *OpenAICompatibleClient) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	if err := req.Validate(); err != nil {
		return 0, "", err
	}
	apis := c.resolveAPIs(req)
	if len(apis) == 0 {
		return 0, "", errors.New("openai compatible api is required")
	}

	endpoints := compatibleInputTokensAPIs(apis)
	if len(endpoints) == 0 {
		return EstimateRequestTokens(req), TokenCountSourceEstimated, nil
	}

	var errs []error
	for _, api := range endpoints {
		if _, unsupported := c.unsupportedInputTokens.Load(api.Endpoint); unsupported {
			continue
		}
		tokens, source, err := countOpenAIInputTokens(ctx, c.httpClient, c.authorizer, api.Endpoint, api.ErrorLabel, req, defaultOpenAIRequestCapabilities())
		if err == nil {
			return tokens, source, nil
		}
		if shouldFallbackCompatibleInputTokens(err) {
			c.unsupportedInputTokens.Store(api.Endpoint, struct{}{})
			continue
		}
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return EstimateRequestTokens(req), TokenCountSourceEstimated, errors.Join(errs...)
	}
	return EstimateRequestTokens(req), TokenCountSourceEstimated, nil
}

func (c *OpenAICompatibleClient) streamWithAPI(ctx context.Context, req Request, api openAICompatibleAPI) (Stream, error) {
	switch api.Mode {
	case openAICompatibleModeResponses:
		return streamOpenAIResponses(ctx, c.httpClient, c.authorizer, api.Endpoint, api.ErrorLabel, req, defaultOpenAIRequestCapabilities())
	case openAICompatibleModeChatCompletions:
		stream, err := streamOpenAIChatCompletions(ctx, c.httpClient, c.authorizer, api.Endpoint, api.ErrorLabel, req, true)
		if err == nil {
			return stream, nil
		}
		if shouldRetryChatCompletionsWithoutUsage(err) {
			return streamOpenAIChatCompletions(ctx, c.httpClient, c.authorizer, api.Endpoint, api.ErrorLabel, req, false)
		}
		return nil, err
	default:
		return nil, fmt.Errorf("unsupported openai compatible api mode %q", api.Mode)
	}
}

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
