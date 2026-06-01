package provider

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

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
