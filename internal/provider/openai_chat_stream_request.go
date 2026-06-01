package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

func streamOpenAIChatCompletions(
	ctx context.Context,
	httpClient *http.Client,
	authorizer openAIRequestAuthorizer,
	endpoint string,
	errorPrefix string,
	req Request,
	includeUsage bool,
) (Stream, error) {
	payload, err := buildOpenAIChatCompletionsRequest(req, includeUsage)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	//nolint:bodyclose // The returned stream owns and closes resp.Body.
	resp, err := doOpenAIAuthorizedRequest(ctx, httpClient, authorizer, errorPrefix, func(ctx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		applyGitHubCopilotRequestHeaders(req, httpReq)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		return httpReq, nil
	})
	if err != nil {
		return nil, err
	}

	return newOpenAIChatCompletionsStreamWithConfig(
		resp.Body,
		streamReasoningModeForRequest(req),
		func() (providerAuthDebugState, bool) { return authDebugStateFor(authorizer) },
		req.RawSSEObserver,
		openAIChatCompletionsStreamConfigForRequest(req),
	), nil
}

func openAIChatCompletionsStreamConfigForRequest(req Request) openAIChatCompletionsStreamConfig {
	return openAIChatCompletionsStreamConfig{
		FlushToolCallsOnStop: isGeminiModelID(req.Model.ModelID),
	}
}
