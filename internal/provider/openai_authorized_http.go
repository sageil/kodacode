package provider

import (
	"context"
	"fmt"
	"net/http"
)

type openAIRequestAuthRefresher interface {
	RefreshAuth(context.Context) error
}

func doOpenAIAuthorizedRequest(
	ctx context.Context,
	httpClient *http.Client,
	authorizer openAIRequestAuthorizer,
	errorPrefix string,
	buildRequest func(context.Context) (*http.Request, error),
) (*http.Response, error) {
	for attempt := 0; attempt < 2; attempt++ {
		req, err := buildRequest(ctx)
		if err != nil {
			return nil, err
		}
		if err := authorizer.Authorize(ctx, req); err != nil {
			return nil, err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		message, err := readAndCloseOpenAIErrorResponse(resp, errorPrefix)
		if err != nil {
			return nil, err
		}
		if attempt == 0 {
			retried, refreshErr := refreshOpenAIAuthAfterFailure(ctx, authorizer, resp.StatusCode, message)
			if refreshErr != nil {
				return nil, fmt.Errorf("%s: refresh auth: %w", errorPrefix, refreshErr)
			}
			if retried {
				continue
			}
		}
		return nil, annotateAuthProviderError(
			authorizer,
			"http_response",
			newProviderHTTPError(errorPrefix, resp.StatusCode, message, resp.Header),
		)
	}
	return nil, fmt.Errorf("%s: retry exhausted", errorPrefix)
}

func refreshOpenAIAuthAfterFailure(ctx context.Context, authorizer openAIRequestAuthorizer, statusCode int, message string) (bool, error) {
	if !LooksLikeAuthProviderResponse(statusCode, message) {
		return false, nil
	}
	refresher, ok := authorizer.(openAIRequestAuthRefresher)
	if !ok {
		return false, nil
	}
	if err := refresher.RefreshAuth(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func readAndCloseOpenAIErrorResponse(resp *http.Response, errorPrefix string) (string, error) {
	message, readErr := readOpenAIError(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		if readErr != nil {
			return "", fmt.Errorf("%w: close body: %w", readErr, closeErr)
		}
		return "", fmt.Errorf("%s: close body: %w", errorPrefix, closeErr)
	}
	if readErr != nil {
		return "", readErr
	}
	return message, nil
}
