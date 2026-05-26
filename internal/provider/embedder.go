package provider

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrEmbeddingModelRequired            = errors.New("embedding model is required")
	ErrEmbeddingInputsRequired           = errors.New("embedding inputs are required")
	ErrEmbeddingProviderNotConfigured    = errors.New("embedding provider is not configured")
	ErrEmbeddingVectorCountMismatch      = errors.New("embedding vector count mismatch")
	ErrEmbeddingVectorDimensionsMismatch = errors.New("embedding vector dimensions mismatch")
)

type EmbeddingRequest struct {
	Model      ModelRef
	Inputs     []string
	Dimensions int
}

func (r EmbeddingRequest) Validate() error {
	if err := r.Model.Validate(); err != nil {
		return ErrEmbeddingModelRequired
	}
	if len(r.Inputs) == 0 {
		return ErrEmbeddingInputsRequired
	}
	return nil
}

type Embedder interface {
	Embed(context.Context, EmbeddingRequest) ([][]float32, error)
}

type RoutedEmbedder struct {
	embedders map[string]Embedder
}

func NewRoutedEmbedder(embedders map[string]Embedder) *RoutedEmbedder {
	copied := make(map[string]Embedder, len(embedders))
	for providerID, embedder := range embedders {
		copied[providerID] = embedder
	}
	return &RoutedEmbedder{embedders: copied}
}

func (r *RoutedEmbedder) Embed(ctx context.Context, req EmbeddingRequest) ([][]float32, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	embedder := r.embedders[req.Model.ProviderID]
	if embedder == nil {
		return nil, fmt.Errorf("%w: %s", ErrEmbeddingProviderNotConfigured, req.Model.ProviderID)
	}
	return embedder.Embed(ctx, req)
}
