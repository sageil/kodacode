package provider_test

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

// fakeEmbeddingProvider implements both Provider and EmbeddingProvider.
type fakeEmbeddingProvider struct {
	fakeProvider
	vectors [][]float32
}

func (f *fakeEmbeddingProvider) Embed(_ context.Context, _ string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	return f.vectors[:len(texts)], nil
}

func TestRegistry_EmbeddingProvider(t *testing.T) {
	r := provider.NewRegistry()

	ep := &fakeEmbeddingProvider{
		fakeProvider: fakeProvider{id: "openai", name: "OpenAI"},
		vectors:      [][]float32{{0.1, 0.2, 0.3}},
	}
	r.Register(ep) //nolint:errcheck

	got, ok := r.EmbeddingProvider("openai")
	if !ok {
		t.Fatal("EmbeddingProvider(\"openai\") = _, false; want true")
	}

	vecs, err := got.Embed(context.Background(), "text-embedding-3-small", []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 3 {
		t.Errorf("Embed() returned %d vectors, want 1 with 3 dims", len(vecs))
	}
}

func TestRegistry_EmbeddingProvider_NotImplemented(t *testing.T) {
	r := provider.NewRegistry()
	r.Register(&fakeProvider{id: "anthropic", name: "Anthropic"}) //nolint:errcheck

	_, ok := r.EmbeddingProvider("anthropic")
	if ok {
		t.Error("EmbeddingProvider(\"anthropic\") = _, true; want false for non-embedding provider")
	}
}

func TestRegistry_EmbeddingProvider_NotFound(t *testing.T) {
	r := provider.NewRegistry()

	_, ok := r.EmbeddingProvider("missing")
	if ok {
		t.Error("EmbeddingProvider(\"missing\") = _, true; want false")
	}
}
