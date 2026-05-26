package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type openAIInputTokensRequest struct {
	Model             string `json:"model"`
	Instructions      string `json:"instructions,omitempty"`
	Input             []any  `json:"input"`
	Tools             []any  `json:"tools,omitempty"`
	ParallelToolCalls bool   `json:"parallel_tool_calls,omitempty"`
	PromptCacheKey    string `json:"prompt_cache_key,omitempty"`
}

type openAIInputTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

func buildOpenAIInputTokensRequestWithCapabilities(req Request, capabilities openAIRequestCapabilities) (openAIInputTokensRequest, error) {
	payload, err := buildOpenAIRequestWithCapabilities(req, capabilities)
	if err != nil {
		return openAIInputTokensRequest{}, err
	}
	return openAIInputTokensRequest{
		Model:             payload.Model,
		Instructions:      payload.Instructions,
		Input:             payload.Input,
		Tools:             payload.Tools,
		ParallelToolCalls: payload.ParallelToolCalls,
		PromptCacheKey:    payload.PromptCacheKey,
	}, nil
}

func countOpenAIInputTokens(
	ctx context.Context,
	httpClient *http.Client,
	authorizer openAIRequestAuthorizer,
	endpoint string,
	errorPrefix string,
	req Request,
	capabilities openAIRequestCapabilities,
) (int, TokenCountSource, error) {
	payload, err := buildOpenAIInputTokensRequestWithCapabilities(req, capabilities)
	if err != nil {
		return 0, "", err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, "", err
	}
	resp, err := doOpenAIAuthorizedRequest(ctx, httpClient, authorizer, errorPrefix, func(ctx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		applyGitHubCopilotRequestHeaders(req, httpReq)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")
		return httpReq, nil
	})
	if err != nil {
		return 0, "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var result openAIInputTokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, "", fmt.Errorf("%s: decode: %w", errorPrefix, err)
	}
	if result.InputTokens < 0 {
		result.InputTokens = 0
	}
	return result.InputTokens, TokenCountSourceExact, nil
}

func openAIInputTokensEndpoint(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/input_tokens"
}
