package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

func streamOpenAIResponses(
	ctx context.Context,
	httpClient *http.Client,
	authorizer openAIRequestAuthorizer,
	baseURL string,
	errorPrefix string,
	req Request,
	capabilities openAIRequestCapabilities,
) (Stream, error) {
	payload, err := buildOpenAIRequestWithCapabilities(req, capabilities)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	//nolint:bodyclose // The returned stream owns and closes resp.Body.
	resp, err := doOpenAIAuthorizedRequest(ctx, httpClient, authorizer, errorPrefix, func(ctx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
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

	return newOpenAIStreamWithReasoningModeAndAuthDebugAndRawSSEObserver(
		resp.Body,
		streamReasoningModeForRequest(req),
		func() (providerAuthDebugState, bool) { return authDebugStateFor(authorizer) },
		req.RawSSEObserver,
	), nil
}
