package provider

import "github.com/sageil/kodacode/internal/provider/openaiurl"

func NormalizeOpenAIBaseURL(baseURL string) string {
	return openaiurl.Normalize(baseURL)
}

func openAIResponsesEndpoint(baseURL, defaultBaseURL string) string {
	return openaiurl.ResponsesEndpoint(baseURL, defaultBaseURL)
}
