package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

var ErrAnthropicAPIKeyRequired = errors.New("anthropic api key is required")

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	anthropicBetaHeader     = "interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"
)

type AnthropicConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type AnthropicClient struct {
	client anthropicsdk.Client
}

func DefaultAnthropicBaseURL() string {
	return defaultAnthropicBaseURL
}

func NewAnthropicClient(config AnthropicConfig) (*AnthropicClient, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, ErrAnthropicAPIKeyRequired
	}
	options := []option.RequestOption{
		option.WithAPIKey(strings.TrimSpace(config.APIKey)),
		option.WithHeader("anthropic-beta", anthropicBetaHeader),
	}
	if strings.TrimSpace(config.BaseURL) != "" {
		options = append(options, option.WithBaseURL(strings.TrimSpace(config.BaseURL)))
	}
	if config.HTTPClient != nil {
		options = append(options, option.WithHTTPClient(config.HTTPClient))
	}
	return &AnthropicClient{
		client: anthropicsdk.NewClient(options...),
	}, nil
}

func (c *AnthropicClient) Stream(ctx context.Context, req Request) (Stream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	params, err := buildAnthropicParams(req)
	if err != nil {
		return nil, err
	}
	stream := c.client.Messages.NewStreaming(ctx, params)
	return withRequestTrace(newAnthropicStream(ctx, stream), RequestTrace{
		APIMode:           "messages",
		ParallelToolCalls: len(req.Tools) > 0,
	}), nil
}

func (c *AnthropicClient) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	if err := req.Validate(); err != nil {
		return 0, "", err
	}
	params, err := buildAnthropicParams(req)
	if err != nil {
		return 0, "", err
	}
	countParams := anthropicsdk.MessageCountTokensParams{
		Model:        params.Model,
		Messages:     params.Messages,
		CacheControl: params.CacheControl,
		OutputConfig: params.OutputConfig,
		Thinking:     params.Thinking,
		ToolChoice:   params.ToolChoice,
	}
	if len(params.System) > 0 {
		countParams.System = anthropicsdk.MessageCountTokensParamsSystemUnion{
			OfTextBlockArray: params.System,
		}
	}
	if len(params.Tools) > 0 {
		tools := make([]anthropicsdk.MessageCountTokensToolUnionParam, 0, len(params.Tools))
		for _, tool := range params.Tools {
			tools = append(tools, anthropicsdk.MessageCountTokensToolUnionParam{
				OfTool: tool.OfTool,
			})
		}
		countParams.Tools = tools
	}
	result, err := c.client.Messages.CountTokens(ctx, countParams)
	if err != nil {
		return 0, "", err
	}
	return int(result.InputTokens), TokenCountSourceExact, nil
}
