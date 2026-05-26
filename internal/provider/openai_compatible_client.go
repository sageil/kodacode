package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
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

func compatibleAPIFromBaseURL(baseURL, name string) openAICompatibleAPI {
	trimmed := compatibleAPIRoot(baseURL)
	switch {
	case strings.HasSuffix(trimmed, "/responses"):
		return openAICompatibleAPI{
			Mode:       openAICompatibleModeResponses,
			Endpoint:   trimmed,
			ErrorLabel: name + " responses api",
		}
	case strings.HasSuffix(trimmed, "/chat/completions"):
		return openAICompatibleAPI{
			Mode:       openAICompatibleModeChatCompletions,
			Endpoint:   trimmed,
			ErrorLabel: name + " chat completions api",
		}
	default:
		return openAICompatibleAPI{
			Mode:       openAICompatibleModeChatCompletions,
			Endpoint:   trimmed + "/chat/completions",
			ErrorLabel: name + " chat completions api",
		}
	}
}

func compatibleAPIRoot(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func gitHubCopilotAPIs(baseURL, model string) []openAICompatibleAPI {
	trimmed := compatibleAPIRoot(baseURL)
	explicitMode := openAICompatibleAPIMode("")
	switch {
	case strings.HasSuffix(trimmed, "/responses"):
		explicitMode = openAICompatibleModeResponses
	case strings.HasSuffix(trimmed, "/chat/completions"):
		explicitMode = openAICompatibleModeChatCompletions
	}

	root := gitHubCopilotRoot(baseURL)

	responses := openAICompatibleAPI{
		Mode:       openAICompatibleModeResponses,
		Endpoint:   root + "/responses",
		ErrorLabel: "github copilot responses api",
	}
	chat := openAICompatibleAPI{
		Mode:       openAICompatibleModeChatCompletions,
		Endpoint:   root + "/chat/completions",
		ErrorLabel: "github copilot chat completions api",
	}

	switch explicitMode {
	case openAICompatibleModeResponses:
		return []openAICompatibleAPI{responses, chat}
	case openAICompatibleModeChatCompletions:
		return []openAICompatibleAPI{chat, responses}
	default:
		if prefersResponsesOnGitHubCopilot(model) {
			return []openAICompatibleAPI{responses, chat}
		}
		return []openAICompatibleAPI{chat, responses}
	}
}

func gitHubCopilotRoot(baseURL string) string {
	root := compatibleAPIRoot(baseURL)
	if root == "" {
		return defaultGitHubCopilotBaseURL
	}
	switch {
	case strings.HasSuffix(root, "/responses"):
		return strings.TrimSuffix(root, "/responses")
	case strings.HasSuffix(root, "/chat/completions"):
		return strings.TrimSuffix(root, "/chat/completions")
	default:
		return root
	}
}

func prefersResponsesOnGitHubCopilot(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	major, ok := gitHubCopilotGPTMajor(model)
	return ok && major >= 5
}

func gitHubCopilotGPTMajor(model string) (int, bool) {
	rest, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-")
	if !ok {
		return 0, false
	}
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	major, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0, false
	}
	return major, true
}

func shouldFallbackCompatibleAPI(mode openAICompatibleAPIMode, err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	switch mode {
	case openAICompatibleModeResponses:
		return isUnsupportedResponsesAPIError(lower)
	case openAICompatibleModeChatCompletions:
		return isUnsupportedChatCompletionsAPIError(lower)
	default:
		return false
	}
}

func isUnsupportedResponsesAPIError(lower string) bool {
	return strings.Contains(lower, "unsupported_api_for_model") ||
		strings.Contains(lower, "not accessible via the /responses endpoint") ||
		strings.Contains(lower, "does not support responses api") ||
		strings.Contains(lower, "is not supported via responses api") ||
		(strings.Contains(lower, "/responses") && strings.Contains(lower, "404 not found"))
}

func isUnsupportedChatCompletionsAPIError(lower string) bool {
	return (strings.Contains(lower, "unsupported_api_for_model") ||
		strings.Contains(lower, "not accessible via the /chat/completions endpoint") ||
		strings.Contains(lower, "does not support chat completions api") ||
		strings.Contains(lower, "is not supported via chat completions api")) &&
		(strings.Contains(lower, "/chat/completions") || strings.Contains(lower, "chat completions api"))
}

func shouldRetryChatCompletionsWithoutUsage(err error) bool {
	if err == nil {
		return false
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return false
	}
	switch providerErr.StatusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
	default:
		return false
	}
	lower := strings.ToLower(providerErr.Error())
	return strings.Contains(lower, "stream_options") ||
		strings.Contains(lower, "include_usage") ||
		strings.Contains(lower, "unknown field") ||
		strings.Contains(lower, "unknown parameter") ||
		strings.Contains(lower, "additional properties")
}

func compatibleInputTokensAPIs(apis []openAICompatibleAPI) []openAICompatibleAPI {
	if len(apis) == 0 {
		return nil
	}
	out := make([]openAICompatibleAPI, 0, len(apis))
	seen := make(map[string]struct{}, len(apis))
	for _, api := range apis {
		endpoint := compatibleInputTokensEndpoint(api)
		if endpoint == "" {
			continue
		}
		if _, ok := seen[endpoint]; ok {
			continue
		}
		seen[endpoint] = struct{}{}
		out = append(out, openAICompatibleAPI{
			Mode:       openAICompatibleModeResponses,
			Endpoint:   endpoint,
			ErrorLabel: compatibleInputTokensErrorLabel(api),
		})
	}
	return out
}

func compatibleInputTokensEndpoint(api openAICompatibleAPI) string {
	endpoint := strings.TrimRight(strings.TrimSpace(api.Endpoint), "/")
	switch api.Mode {
	case openAICompatibleModeResponses:
		return endpoint + "/input_tokens"
	case openAICompatibleModeChatCompletions:
		root := strings.TrimSuffix(endpoint, "/chat/completions")
		if root == endpoint {
			return ""
		}
		return root + "/responses/input_tokens"
	default:
		return ""
	}
}

func compatibleInputTokensErrorLabel(api openAICompatibleAPI) string {
	label := strings.TrimSpace(api.ErrorLabel)
	switch {
	case strings.HasSuffix(label, " responses api"):
		return strings.TrimSuffix(label, " responses api") + " responses input tokens api"
	case strings.HasSuffix(label, " chat completions api"):
		return strings.TrimSuffix(label, " chat completions api") + " responses input tokens api"
	case label != "":
		return label + " input tokens"
	default:
		return "openai compatible responses input tokens api"
	}
}

func shouldFallbackCompatibleInputTokens(err error) bool {
	if err == nil {
		return false
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		switch providerErr.StatusCode {
		case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
			return true
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			lower := strings.ToLower(providerErr.Error())
			return isUnsupportedResponsesAPIError(lower) ||
				strings.Contains(lower, "/responses/input_tokens") && strings.Contains(lower, "not found")
		}
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "/responses/input_tokens") && strings.Contains(lower, "404 not found")
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
