package openaiurl

import "strings"

func Normalize(baseURL string) string {
	return strings.TrimRight(Root(baseURL), "/")
}

func Root(baseURL string) string {
	root := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch {
	case strings.HasSuffix(root, "/responses"):
		return strings.TrimSuffix(root, "/responses")
	case strings.HasSuffix(root, "/chat/completions"):
		return strings.TrimSuffix(root, "/chat/completions")
	default:
		return root
	}
}

func ResponsesEndpoint(baseURL, defaultBaseURL string) string {
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
