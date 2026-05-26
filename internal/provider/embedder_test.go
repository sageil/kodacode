package provider

import (
	"context"
	"errors"
	"testing"
)

type stubEmbedder struct {
	vectors [][]float32
	err     error
	reqs    []EmbeddingRequest
}

func (s *stubEmbedder) Embed(_ context.Context, req EmbeddingRequest) ([][]float32, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.vectors, nil
}

func TestRoutedEmbedderRoutesByProvider(t *testing.T) {
	openai := &stubEmbedder{vectors: [][]float32{{1, 2}}}
	routed := NewRoutedEmbedder(map[string]Embedder{"openai": openai})

	vectors, err := routed.Embed(context.Background(), EmbeddingRequest{
		Model:  ModelRef{ProviderID: "openai", ModelID: "text-embedding-3-small"},
		Inputs: []string{"hello"},
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || vectors[0][0] != 1 {
		t.Fatalf("vectors = %#v", vectors)
	}
	if len(openai.reqs) != 1 || openai.reqs[0].Model.ModelID != "text-embedding-3-small" {
		t.Fatalf("requests = %#v", openai.reqs)
	}
}

func TestRoutedEmbedderRejectsUnknownProvider(t *testing.T) {
	routed := NewRoutedEmbedder(nil)

	_, err := routed.Embed(context.Background(), EmbeddingRequest{
		Model:  ModelRef{ProviderID: "missing", ModelID: "text-embedding-3-small"},
		Inputs: []string{"hello"},
	})
	if !errors.Is(err, ErrEmbeddingProviderNotConfigured) {
		t.Fatalf("Embed() error = %v, want ErrEmbeddingProviderNotConfigured", err)
	}
}
