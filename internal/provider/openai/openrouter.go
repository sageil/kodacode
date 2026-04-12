package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/openai/openai-go/v2/option"
)

// openRouterMiddleware injects OpenRouter-specific provider routing preferences
// into the request body. When tools are present in the request, it adds
// {"provider": {"require_parameters": true}} to ensure OpenRouter only routes
// to providers that support tool/function calling.
func openRouterMiddleware(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	if req.Body == nil || req.Method != http.MethodPost {
		return next(req)
	}

	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return next(req)
	}

	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		return next(req)
	}

	// Only inject when tools are present — no need for simple chat requests.
	if tools, ok := payload["tools"]; ok && tools != nil {
		if _, hasProvider := payload["provider"]; !hasProvider {
			payload["provider"] = map[string]any{
				"require_parameters": true,
			}
		}
	}

	out, err := json.Marshal(payload)
	if err != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		return next(req)
	}

	req.Body = io.NopCloser(bytes.NewReader(out))
	req.ContentLength = int64(len(out))
	return next(req)
}
