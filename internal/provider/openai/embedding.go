package openai

import (
	"context"
	"fmt"
	"sort"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/sageil/kodacode/v1/internal/provider"
)

// compile-time interface check
var _ provider.EmbeddingProvider = (*Client)(nil)

// Embed implements provider.EmbeddingProvider using the OpenAI embeddings API.
// Compatible with any OpenAI-compatible endpoint (OpenAI, Ollama, Together AI, etc.).
func (c *Client) Embed(ctx context.Context, model string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	resp, err := c.sdkClient.Embeddings.New(ctx, openaisdk.EmbeddingNewParams{
		Input: openaisdk.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
		Model: openaisdk.EmbeddingModel(model),
	})
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}

	// Sort by Index. The API does not guarantee response ordering,
	// and compatible servers (Ollama, vLLM) may return out of order.
	sort.Slice(resp.Data, func(i, j int) bool {
		return resp.Data[i].Index < resp.Data[j].Index
	})

	vectors := make([][]float32, len(resp.Data))
	for i, emb := range resp.Data {
		vectors[i] = float64sToFloat32s(emb.Embedding)
	}
	return vectors, nil
}

func float64sToFloat32s(src []float64) []float32 {
	dst := make([]float32, len(src))
	for i, v := range src {
		dst[i] = float32(v)
	}
	return dst
}
