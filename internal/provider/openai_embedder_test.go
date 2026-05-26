package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIEmbedderBuildsEmbeddingRequest(t *testing.T) {
	var authorization string
	var contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2]},{"index":1,"embedding":[0.3,0.4]}]}`))
	}))
	defer server.Close()

	embedder, err := NewOpenAIEmbedder(OpenAIEmbeddingConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL + "/v1/embeddings",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewOpenAIEmbedder() error = %v", err)
	}

	vectors, err := embedder.Embed(context.Background(), EmbeddingRequest{
		Model:  ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"},
		Inputs: []string{"one", "two"},
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if authorization != "Bearer test-key" {
		t.Fatalf("authorization = %q", authorization)
	}
	if contentType != "application/json" {
		t.Fatalf("content type = %q", contentType)
	}
	if len(vectors) != 2 || len(vectors[0]) != 2 || vectors[1][1] != 0.4 {
		t.Fatalf("vectors = %#v", vectors)
	}
}

func TestOpenAICompatibleEmbedderBuildsEmbeddingRequestFromBareRoot(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()

	embedder, err := NewOpenAICompatibleEmbedder(OpenAICompatibleEmbeddingConfig{
		BaseURL:    server.URL + "/v1",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedder() error = %v", err)
	}

	vectors, err := embedder.Embed(context.Background(), EmbeddingRequest{
		Model:  ModelRef{ProviderID: "lmstudio", ModelID: "nomic-embed-text-v1.5"},
		Inputs: []string{"one"},
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if authorization != "" {
		t.Fatalf("authorization = %q, want empty for localhost compatible embedder", authorization)
	}
	if len(vectors) != 1 || len(vectors[0]) != 2 || vectors[0][1] != 0.2 {
		t.Fatalf("vectors = %#v", vectors)
	}
}

func TestEmbeddingBaseURLConvertsResponsesEndpoint(t *testing.T) {
	got := embeddingBaseURL("https://example.invalid/v1/responses", "")
	if got != "https://example.invalid/v1/embeddings" {
		t.Fatalf("embeddingBaseURL() = %q", got)
	}
}

func TestEmbeddingBaseURLConvertsChatCompletionsEndpoint(t *testing.T) {
	got := embeddingBaseURL("https://example.invalid/v1/chat/completions", "")
	if got != "https://example.invalid/v1/embeddings" {
		t.Fatalf("embeddingBaseURL() = %q", got)
	}
}

func TestEmbeddingBaseURLAppendsEmbeddingsForBareCompatibleRoot(t *testing.T) {
	got := embeddingBaseURL("https://example.invalid/v1", "")
	if got != "https://example.invalid/v1/embeddings" {
		t.Fatalf("embeddingBaseURL() = %q", got)
	}
}

func TestEmbeddingBaseURLPreservesExplicitEmbeddingsEndpoint(t *testing.T) {
	got := embeddingBaseURL("https://example.invalid/v1/embeddings", "")
	if got != "https://example.invalid/v1/embeddings" {
		t.Fatalf("embeddingBaseURL() = %q", got)
	}
}
