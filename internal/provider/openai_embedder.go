package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const defaultOpenAIEmbeddingsBaseURL = "https://api.openai.com/v1/embeddings"

type OpenAIEmbeddingConfig struct {
	APIKey     string
	OAuth      *OpenAIOAuthConfig
	BaseURL    string
	HTTPClient *http.Client
}

type OpenAICompatibleEmbeddingConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type OpenAIEmbedder struct {
	authorizer openAIRequestAuthorizer
	baseURL    string
	httpClient *http.Client
	errorLabel string
}

func NewOpenAIEmbedder(config OpenAIEmbeddingConfig) (*OpenAIEmbedder, error) {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	authorizer, err := newOpenAIAuthorizer(OpenAIConfig{
		APIKey: config.APIKey,
		OAuth:  config.OAuth,
	}, httpClient)
	if err != nil {
		return nil, err
	}
	return &OpenAIEmbedder{
		authorizer: authorizer,
		baseURL:    embeddingBaseURL(config.BaseURL, defaultOpenAIEmbeddingsBaseURL),
		httpClient: httpClient,
		errorLabel: "openai embeddings api",
	}, nil
}

func NewOpenAICompatibleEmbedder(config OpenAICompatibleEmbeddingConfig) (*OpenAIEmbedder, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, ErrOpenAICompatibleBaseURLRequired
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	authorizer, err := newOpenAICompatibleAuthorizer(config.APIKey, config.BaseURL)
	if err != nil {
		return nil, err
	}
	return &OpenAIEmbedder{
		authorizer: authorizer,
		baseURL:    embeddingBaseURL(config.BaseURL, ""),
		httpClient: httpClient,
		errorLabel: "openai compatible embeddings api",
	}, nil
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, req EmbeddingRequest) ([][]float32, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(openAIEmbeddingRequest{
		Model:          req.Model.ModelID,
		Input:          append([]string(nil), req.Inputs...),
		Dimensions:     req.Dimensions,
		EncodingFormat: "float",
	})
	if err != nil {
		return nil, err
	}

	resp, err := doOpenAIAuthorizedRequest(ctx, e.httpClient, e.authorizer, e.errorLabel, func(ctx context.Context) (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")
		return httpReq, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	var payload openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Data) != len(req.Inputs) {
		return nil, fmt.Errorf("%w: got %d vectors for %d inputs", ErrEmbeddingVectorCountMismatch, len(payload.Data), len(req.Inputs))
	}

	vectors := make([][]float32, len(payload.Data))
	for _, item := range payload.Data {
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, fmt.Errorf("embedding index out of range: %d", item.Index)
		}
		if req.Dimensions > 0 && len(item.Embedding) != req.Dimensions {
			return nil, fmt.Errorf("%w: got %d want %d", ErrEmbeddingVectorDimensionsMismatch, len(item.Embedding), req.Dimensions)
		}
		vectors[item.Index] = append([]float32(nil), item.Embedding...)
	}
	for idx, vector := range vectors {
		if len(vector) == 0 {
			return nil, fmt.Errorf("missing embedding vector at index %d", idx)
		}
	}
	return vectors, nil
}

type openAIEmbeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	Dimensions     int      `json:"dimensions,omitempty"`
	EncodingFormat string   `json:"encoding_format"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func embeddingBaseURL(baseURL, defaultURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return defaultURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if strings.HasSuffix(baseURL, "/embeddings") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/responses") {
		return strings.TrimSuffix(baseURL, "/responses") + "/embeddings"
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return strings.TrimSuffix(baseURL, "/chat/completions") + "/embeddings"
	}
	return baseURL + "/embeddings"
}
