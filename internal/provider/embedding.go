package provider

import "context"

// EmbeddingProvider is an optional capability for providers that support
// text embedding APIs. Providers that offer embeddings implement this
// interface in addition to Provider.
//
// Detection at runtime:
//
//	if ep, ok := prov.(EmbeddingProvider); ok { ... }
type EmbeddingProvider interface {
	Embed(ctx context.Context, model string, texts []string) ([][]float32, error)
}
