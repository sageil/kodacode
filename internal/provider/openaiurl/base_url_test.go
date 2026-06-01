package openaiurl

import "testing"

func TestNormalizeStripsTransportEndpoints(t *testing.T) {
	t.Run("responses", func(t *testing.T) {
		got := Normalize("https://api.openai.com/v1/responses")
		if got != "https://api.openai.com/v1" {
			t.Fatalf("Normalize() = %q", got)
		}
	})

	t.Run("chat completions", func(t *testing.T) {
		got := Normalize("https://example.invalid/v1/chat/completions")
		if got != "https://example.invalid/v1" {
			t.Fatalf("Normalize() = %q", got)
		}
	})
}

func TestResponsesEndpointNormalizesConfiguredBaseURL(t *testing.T) {
	t.Run("bare root", func(t *testing.T) {
		got := ResponsesEndpoint("https://api.openai.com/v1", "")
		if got != "https://api.openai.com/v1/responses" {
			t.Fatalf("ResponsesEndpoint() = %q", got)
		}
	})

	t.Run("explicit responses", func(t *testing.T) {
		got := ResponsesEndpoint("https://api.openai.com/v1/responses", "")
		if got != "https://api.openai.com/v1/responses" {
			t.Fatalf("ResponsesEndpoint() = %q", got)
		}
	})

	t.Run("chat completions", func(t *testing.T) {
		got := ResponsesEndpoint("https://example.invalid/v1/chat/completions", "")
		if got != "https://example.invalid/v1/responses" {
			t.Fatalf("ResponsesEndpoint() = %q", got)
		}
	})
}
