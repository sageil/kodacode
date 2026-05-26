package provider

import "strings"

func NormalizeOpenAIBaseURL(baseURL string) string {
	return strings.TrimRight(modelCatalogRoot(baseURL), "/")
}

func openAIResponsesEndpoint(baseURL, defaultBaseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = strings.TrimRight(strings.TrimSpace(defaultBaseURL), "/")
	}
	switch {
	case trimmed == "":
		return ""
	case strings.HasSuffix(trimmed, "/responses"):
		return trimmed
	case strings.HasSuffix(trimmed, "/chat/completions"):
		return strings.TrimSuffix(trimmed, "/chat/completions") + "/responses"
	default:
		return trimmed + "/responses"
	}
}
