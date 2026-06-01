package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"google.golang.org/genai"
)

var ErrGoogleAPIKeyRequired = errors.New("google api key is required")

const defaultGoogleBaseURL = "https://generativelanguage.googleapis.com"

var googleCallIDCounter atomic.Int64

type GoogleConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type GoogleClient struct {
	client *genai.Client
}

func DefaultGoogleBaseURL() string {
	return defaultGoogleBaseURL
}

func NewGoogleClient(ctx context.Context, config GoogleConfig) (*GoogleClient, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, ErrGoogleAPIKeyRequired
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     strings.TrimSpace(config.APIKey),
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: config.HTTPClient,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: firstNonBlank(strings.TrimSpace(config.BaseURL), defaultGoogleBaseURL),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("google: create client: %w", err)
	}
	return &GoogleClient{client: client}, nil
}

func (c *GoogleClient) Stream(ctx context.Context, req Request) (Stream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	req = normalizeConversationToolCallIDs(req)
	conversation, err := buildNativeConversation(req.Inputs)
	if err != nil {
		return nil, err
	}
	config, err := buildGoogleGenerateContentConfig(req)
	if err != nil {
		return nil, err
	}
	contents, err := buildGoogleConversation(conversation)
	if err != nil {
		return nil, err
	}
	stream := c.client.Models.GenerateContentStream(ctx, req.Model.ModelID, contents, config)
	return newGoogleStream(ctx, stream), nil
}

func (c *GoogleClient) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	if err := req.Validate(); err != nil {
		return 0, "", err
	}
	req = normalizeConversationToolCallIDs(req)
	conversation, err := buildNativeConversation(req.Inputs)
	if err != nil {
		return 0, "", err
	}
	config, err := buildGoogleGenerateContentConfig(req)
	if err != nil {
		return 0, "", err
	}
	contents, err := buildGoogleConversation(conversation)
	if err != nil {
		return 0, "", err
	}

	countConfig := &genai.CountTokensConfig{
		SystemInstruction: config.SystemInstruction,
		Tools:             config.Tools,
	}
	result, err := c.client.Models.CountTokens(ctx, req.Model.ModelID, contents, countConfig)
	if err != nil {
		return 0, "", err
	}
	return int(result.TotalTokens), TokenCountSourceExact, nil
}
