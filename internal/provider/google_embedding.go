package provider

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// compile-time interface check
var _ EmbeddingProvider = (*GoogleProvider)(nil)

// Embed implements provider.EmbeddingProvider using the Google Gemini
// embedding API. Each text becomes a separate Content entry, allowing
// batch embedding in a single API call.
func (p *GoogleProvider) Embed(ctx context.Context, model string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	contents := make([]*genai.Content, len(texts))
	for i, t := range texts {
		contents[i] = &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(t)},
		}
	}

	resp, err := p.client.Models.EmbedContent(ctx, model, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("google embed: %w", err)
	}

	vectors := make([][]float32, len(resp.Embeddings))
	for i, emb := range resp.Embeddings {
		vectors[i] = emb.Values
	}
	return vectors, nil
}
