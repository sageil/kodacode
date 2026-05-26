package provider

import "testing"

func TestNormalizeOpenAIBaseURLStripsTransportEndpoints(t *testing.T) {
	t.Run("responses", func(t *testing.T) {
		got := NormalizeOpenAIBaseURL("https://api.openai.com/v1/responses")
		if got != "https://api.openai.com/v1" {
			t.Fatalf("NormalizeOpenAIBaseURL() = %q", got)
		}
	})

	t.Run("chat completions", func(t *testing.T) {
		got := NormalizeOpenAIBaseURL("https://example.invalid/v1/chat/completions")
		if got != "https://example.invalid/v1" {
			t.Fatalf("NormalizeOpenAIBaseURL() = %q", got)
		}
	})
}

func TestOpenAIResponsesEndpointNormalizesConfiguredBaseURL(t *testing.T) {
	t.Run("bare root", func(t *testing.T) {
		got := openAIResponsesEndpoint("https://api.openai.com/v1", "")
		if got != "https://api.openai.com/v1/responses" {
			t.Fatalf("openAIResponsesEndpoint() = %q", got)
		}
	})

	t.Run("explicit responses", func(t *testing.T) {
		got := openAIResponsesEndpoint("https://api.openai.com/v1/responses", "")
		if got != "https://api.openai.com/v1/responses" {
			t.Fatalf("openAIResponsesEndpoint() = %q", got)
		}
	})

	t.Run("chat completions", func(t *testing.T) {
		got := openAIResponsesEndpoint("https://example.invalid/v1/chat/completions", "")
		if got != "https://example.invalid/v1/responses" {
			t.Fatalf("openAIResponsesEndpoint() = %q", got)
		}
	})
}
