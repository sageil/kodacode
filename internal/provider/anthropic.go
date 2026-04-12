package provider

import (
	"context"
	"fmt"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// compile-time interface checks
var _ Provider = (*AnthropicProvider)(nil)
var _ TokenCounter = (*AnthropicProvider)(nil)
var _ AttachmentSupporter = (*AnthropicProvider)(nil)
var _ StaticModelProvider = (*AnthropicProvider)(nil)

// AnthropicProvider implements the provider.Provider interface using the
// official Anthropic SDK.
type AnthropicProvider struct {
	client         anthropicsdk.Client
	skipToolChoice bool
	models         []Model
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	client := anthropicsdk.NewClient(option.WithAPIKey(apiKey))
	return &AnthropicProvider{
		client: client,
	}
}

func (p *AnthropicProvider) SetConfiguredModels(models []Model) {
	p.models = cloneModels(models)
}

// MarkToolChoiceUnsupported disables tool_choice for future calls.
func (p *AnthropicProvider) MarkToolChoiceUnsupported() {
	p.skipToolChoice = true
}

// ID implements provider.Provider.
func (p *AnthropicProvider) ID() string { return "anthropic" }

// Name implements provider.Provider.
func (p *AnthropicProvider) Name() string { return "Anthropic" }

func (p *AnthropicProvider) AttachmentCapabilities(model Model) AttachmentCapabilities {
	return (AttachmentCapabilities{
		Images: true,
		PDFs:   true,
		Text:   true,
	}).ForModel(model)
}

func (p *AnthropicProvider) StaticModels() []Model {
	return cloneModels(p.models)
}

// Models implements provider.Provider by querying the Anthropic Models API.
// Context sizes are not returned by the API — they are enriched downstream
// by the model cache (models.dev).
func (p *AnthropicProvider) Models(ctx context.Context) ([]Model, error) {
	page, err := p.client.Models.List(ctx, anthropicsdk.ModelListParams{
		Limit: anthropicsdk.Int(1000),
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic models API: %w", err)
	}

	var models []Model
	for _, m := range page.Data {
		models = append(models, Model{
			ID:            m.ID,
			Name:          m.DisplayName,
			ToolCall:      true,
			ToolCallKnown: true,
		})
	}
	return mergeConfiguredStaticModels(models, p.models), nil
}

// CountTokens implements provider.Provider using the Anthropic count tokens API.
func (p *AnthropicProvider) CountTokens(
	ctx context.Context,
	model string,
	messages []Message,
	opts ChatOptions,
) (int, error) {
	params, err := buildAnthropicParams(model, messages, opts, p.skipToolChoice)
	if err != nil {
		return 0, fmt.Errorf("anthropic: build count params: %w", err)
	}

	countParams := anthropicsdk.MessageCountTokensParams{
		Model:    params.Model,
		Messages: params.Messages,
	}
	if len(params.System) > 0 {
		countParams.System = anthropicsdk.MessageCountTokensParamsSystemUnion{
			OfTextBlockArray: params.System,
		}
	}
	if len(params.Tools) > 0 {
		tools := make([]anthropicsdk.MessageCountTokensToolUnionParam, 0, len(params.Tools))
		for _, t := range params.Tools {
			tools = append(tools, anthropicsdk.MessageCountTokensToolUnionParam{
				OfTool: t.OfTool,
			})
		}
		countParams.Tools = tools
	}

	result, err := p.client.Messages.CountTokens(ctx, countParams)
	if err != nil {
		return 0, fmt.Errorf("anthropic: count tokens: %w", err)
	}
	return int(result.InputTokens), nil
}

// Chat implements provider.Provider.
func (p *AnthropicProvider) Chat(
	ctx context.Context,
	model string,
	messages []Message,
	opts ChatOptions,
) (<-chan StreamChunk, error) {
	params, err := buildAnthropicParams(model, messages, opts, p.skipToolChoice)
	if err != nil {
		return nil, fmt.Errorf("anthropic: build params: %w", err)
	}

	stream := p.client.Messages.NewStreaming(ctx, params)

	ch := make(chan StreamChunk, 64)
	go consumeAnthropicStream(ctx, stream, ch)
	return ch, nil
}
