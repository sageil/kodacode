package provider

import (
	"context"
	"net/http"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type OpenAIConfig struct {
	APIKey                   string
	OAuth                    *OpenAIOAuthConfig
	Backend                  OpenAIBackend
	BaseURL                  string
	PromptCacheRetention     string
	ResponsesStore           bool
	EncryptedReasoningReplay bool
	HTTPClient               *http.Client
}

type OpenAIBackend string

const (
	OpenAIBackendPlatformAPI  OpenAIBackend = "platform_api"
	OpenAIBackendChatGPTCodex OpenAIBackend = "chatgpt_codex"
)

type OpenAIClient struct {
	authorizer               openAIRequestAuthorizer
	baseURL                  string
	capabilities             openAIRequestCapabilities
	promptCacheRetention     string
	responsesStore           bool
	encryptedReasoningReplay bool
	httpClient               *http.Client
}

func DefaultOpenAIBaseURL() string {
	return defaultOpenAIBaseURL
}

func NewOpenAIClient(config OpenAIConfig) (*OpenAIClient, error) {
	if err := ValidateOpenAIPromptCacheRetention(config.PromptCacheRetention); err != nil {
		return nil, err
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	authorizer, err := newOpenAIAuthorizer(config, httpClient)
	if err != nil {
		return nil, err
	}
	backend := normalizeOpenAIBackend(config.Backend, config.OAuth != nil)
	defaultBaseURL := defaultOpenAIBaseURL
	if backend == OpenAIBackendChatGPTCodex {
		defaultBaseURL = defaultOpenAIOAuthBaseURL
	}
	return &OpenAIClient{
		authorizer:               authorizer,
		baseURL:                  openAIResponsesEndpoint(config.BaseURL, defaultBaseURL),
		capabilities:             openAIBackendCapabilities(backend),
		promptCacheRetention:     normalizeOpenAIPromptCacheRetention(config.PromptCacheRetention),
		responsesStore:           config.ResponsesStore,
		encryptedReasoningReplay: config.EncryptedReasoningReplay,
		httpClient:               httpClient,
	}, nil
}

func (c *OpenAIClient) Stream(ctx context.Context, req Request) (Stream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	req.PromptCacheRetention = c.promptCacheRetention
	req.OpenAIResponsesStore = c.responsesStore
	req.OpenAIEncryptedReasoningReplay = c.encryptedReasoningReplay
	stream, err := streamOpenAIResponses(ctx, c.httpClient, c.authorizer, c.baseURL, "openai responses api", req, c.capabilities)
	if err != nil {
		return nil, err
	}
	return withRequestTrace(stream, RequestTrace{
		APIMode:           "responses",
		ParallelToolCalls: len(req.Tools) > 0,
	}), nil
}

func (c *OpenAIClient) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	if err := req.Validate(); err != nil {
		return 0, "", err
	}
	if !c.capabilities.InputTokensEndpoint {
		return EstimateRequestTokens(req), TokenCountSourceEstimated, nil
	}
	req.PromptCacheRetention = c.promptCacheRetention
	req.OpenAIResponsesStore = c.responsesStore
	req.OpenAIEncryptedReasoningReplay = c.encryptedReasoningReplay
	return countOpenAIInputTokens(
		ctx,
		c.httpClient,
		c.authorizer,
		openAIInputTokensEndpoint(c.baseURL),
		"openai responses input tokens api",
		req,
		c.capabilities,
	)
}

type openAIRequestCapabilities struct {
	MaxOutputTokens     bool
	InputTokensEndpoint bool
}

func normalizeOpenAIBackend(backend OpenAIBackend, hasOAuth bool) OpenAIBackend {
	switch backend {
	case OpenAIBackendPlatformAPI, OpenAIBackendChatGPTCodex:
		return backend
	default:
		if hasOAuth {
			return OpenAIBackendChatGPTCodex
		}
		return OpenAIBackendPlatformAPI
	}
}

func openAIBackendCapabilities(backend OpenAIBackend) openAIRequestCapabilities {
	switch backend {
	case OpenAIBackendChatGPTCodex:
		return openAIRequestCapabilities{
			MaxOutputTokens:     false,
			InputTokensEndpoint: false,
		}
	default:
		return defaultOpenAIRequestCapabilities()
	}
}

func defaultOpenAIRequestCapabilities() openAIRequestCapabilities {
	return openAIRequestCapabilities{
		MaxOutputTokens:     true,
		InputTokensEndpoint: true,
	}
}
