// Package openai implements the provider.Provider interface for OpenAI and
// any OpenAI-compatible API (Groq, Ollama, Together AI, vLLM, etc.).
package openai

import (
	"context"
	"log"
	"strings"
	"sync"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/sageil/kodacode/v1/internal/provider"
)

// compile-time interface check
var _ provider.Provider = (*Client)(nil)
var _ provider.AttachmentSupporter = (*Client)(nil)

// Client wraps the openai-go SDK and satisfies provider.Provider.
// When useResponsesAPI is true, Chat uses the Responses API (sdkClient.Responses)
// instead of Chat Completions. This is used for OpenAI's own endpoint (both
// API key and OAuth) while OpenAI-compatible providers use Chat Completions.
type Client struct {
	id              string
	name            string
	models          []provider.Model // static models from config
	sdkClient       openaisdk.Client
	skipStreamUsage bool // set after a provider rejects stream_options
	skipToolChoice  bool // set after a provider rejects tool_choice

	useResponsesAPI             bool   // true for OpenAI native (API key + OAuth), false for compatible providers
	skipResponseMaxOutputTokens bool   // true for codex/oauth, which rejects max_output_tokens
	reasoningSummary            string // Responses API only: "auto", "concise", or "detailed"
	apiModeMu                   sync.RWMutex
	modelAPICaps                map[string]modelAPICapabilities
}

type apiMode uint8

const (
	apiModeChatCompletions apiMode = iota
	apiModeResponses
)

type modelAPICapabilities struct {
	chatCompletionsUnsupported bool
	responsesUnsupported       bool
}

// New creates an OpenAI-compatible provider client.
//
//   - id      unique provider identifier (e.g. "openai", "groq")
//   - name    human-readable display name (e.g. "OpenAI", "Groq")
//   - apiKey  bearer token; may be empty for local providers like Ollama
//   - baseURL full base URL (e.g. "https://api.openai.com/v1"); required
//   - models  list of models to advertise; callers should populate this from
//     config or a static list since we do not make live discovery calls
func New(id, name, apiKey, baseURL string, models []provider.Model) *Client {
	opts := []option.RequestOption{
		option.WithBaseURL(strings.TrimRight(baseURL, "/")),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	// OpenRouter: inject provider routing preferences so requests with tools
	// are only routed to providers that support tool calling.
	if strings.Contains(baseURL, "openrouter") {
		opts = append(opts, option.WithMiddleware(openRouterMiddleware))
	}
	return &Client{
		id:        id,
		name:      name,
		models:    models,
		sdkClient: openaisdk.NewClient(opts...),
	}
}

// NewOpenAI creates a Client for OpenAI's own API using the Responses API.
// This is used when authenticating with an API key against api.openai.com
// (not for OpenAI-compatible providers like Groq or Ollama).
func NewOpenAI(apiKey, baseURL string, models []provider.Model) *Client {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	opts := []option.RequestOption{
		option.WithBaseURL(strings.TrimRight(baseURL, "/")),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &Client{
		id:              "openai",
		name:            "OpenAI",
		models:          models,
		sdkClient:       openaisdk.NewClient(opts...),
		useResponsesAPI: true,
	}
}

// ID implements provider.Provider.
func (c *Client) ID() string { return c.id }

// Name implements provider.Provider.
func (c *Client) Name() string { return c.name }

func (c *Client) AttachmentCapabilities(model provider.Model) provider.AttachmentCapabilities {
	return (provider.AttachmentCapabilities{
		Images: true,
		PDFs:   true,
		Text:   true,
		Binary: true,
	}).ForModel(model)
}

// Models implements provider.Provider. Returns the static model list from config.
// Model discovery is handled by the ModelCache, not by individual providers.
func (c *Client) Models(_ context.Context) ([]provider.Model, error) {
	out := make([]provider.Model, len(c.models))
	copy(out, c.models)
	return out, nil
}

// StaticModels returns the locally configured model list without any provider
// discovery calls. This lets registry listing surface configured models even
// when the model cache is cold or refresh fails.
func (c *Client) StaticModels() []provider.Model {
	out := make([]provider.Model, len(c.models))
	copy(out, c.models)
	return out
}

// Chat implements provider.Provider. It opens a streaming request and returns
// a channel of StreamChunks. The channel is always closed by the background
// goroutine. ctx cancellation is propagated to the stream.
//
// For OpenAI native (useResponsesAPI=true), it uses the Responses API.
// For OpenAI-compatible providers, it uses Chat Completions.
func (c *Client) Chat(
	ctx context.Context,
	model string,
	messages []provider.Message,
	opts provider.ChatOptions,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 64)

	api := c.apiModeForModel(model)
	if api == apiModeResponses {
		allowMaxOutputTokens := c.responseMaxOutputTokensEnabled()
		tokenField := responseTokenField(opts.MaxTokens, allowMaxOutputTokens)
		log.Printf("openai: provider=%s api=responses model=%s token_field=%s token_limit=%d", c.id, model, tokenField, opts.MaxTokens)
		params := buildResponseParams(model, messages, opts, c.skipToolChoice, c.reasoningSummary, allowMaxOutputTokens)
		stream := c.sdkClient.Responses.NewStreaming(ctx, params)
		go consumeResponseStream(ctx, stream, ch)
		return ch, nil
	}

	tokenField := chatCompletionTokenField(model, opts.MaxTokens)
	log.Printf("openai: provider=%s api=chat_completions model=%s token_field=%s token_limit=%d", c.id, model, tokenField, opts.MaxTokens)
	params := buildParams(model, messages, opts, c.skipStreamUsage, c.skipToolChoice, chatToolChoiceMode(c.id))
	stream := c.sdkClient.Chat.Completions.NewStreaming(ctx, params)
	go consumeStream(ctx, stream, ch)
	return ch, nil
}

// MarkStreamOptionsUnsupported disables stream_options for future calls.
// Called by the retry loop when a 400 error suggests the provider rejects
// the stream_options parameter.
func (c *Client) MarkStreamOptionsUnsupported() {
	c.skipStreamUsage = true
}

// MarkToolChoiceUnsupported disables tool_choice for future calls.
func (c *Client) MarkToolChoiceUnsupported() {
	c.skipToolChoice = true
}

// MarkReasoningSummaryUnsupported switches the reasoning summary mode.
// Called when the API rejects the current summary value (e.g., "concise"
// not supported by a model that only accepts "detailed").
// Only applies when useResponsesAPI is true.
func (c *Client) MarkReasoningSummaryUnsupported(fallback string) {
	c.reasoningSummary = fallback
}

func responseTokenField(maxTokens int, allowMaxOutputTokens bool) string {
	if maxTokens <= 0 {
		return "none"
	}
	if allowMaxOutputTokens {
		return "max_output_tokens"
	}
	return "none"
}

func (c *Client) apiModeForModel(model string) apiMode {
	if c.useResponsesAPI {
		return apiModeResponses
	}

	defaultMode := defaultCompatibleAPIMode(c.id, model)
	caps := c.modelAPICapabilitiesFor(model)

	if defaultMode == apiModeResponses {
		if !caps.responsesUnsupported {
			return apiModeResponses
		}
		if !caps.chatCompletionsUnsupported {
			return apiModeChatCompletions
		}
		return apiModeResponses
	}

	if !caps.chatCompletionsUnsupported {
		return apiModeChatCompletions
	}
	if !caps.responsesUnsupported {
		return apiModeResponses
	}
	return apiModeChatCompletions
}

func defaultCompatibleAPIMode(providerID, model string) apiMode {
	lower := strings.ToLower(model)
	if providerID == "github-copilot" && strings.HasPrefix(lower, "gpt-5.4") {
		return apiModeResponses
	}
	return apiModeChatCompletions
}

func (c *Client) modelAPICapabilitiesFor(model string) modelAPICapabilities {
	key := strings.ToLower(model)
	c.apiModeMu.RLock()
	defer c.apiModeMu.RUnlock()
	if c.modelAPICaps == nil {
		return modelAPICapabilities{}
	}
	return c.modelAPICaps[key]
}

func (c *Client) markAPIModeUnsupported(model string, mode apiMode) {
	if model == "" {
		return
	}
	key := strings.ToLower(model)
	c.apiModeMu.Lock()
	defer c.apiModeMu.Unlock()
	if c.modelAPICaps == nil {
		c.modelAPICaps = make(map[string]modelAPICapabilities)
	}
	caps := c.modelAPICaps[key]
	switch mode {
	case apiModeResponses:
		caps.responsesUnsupported = true
	case apiModeChatCompletions:
		caps.chatCompletionsUnsupported = true
	}
	c.modelAPICaps[key] = caps
}

func (c *Client) MarkChatCompletionsUnsupported(model string) {
	c.markAPIModeUnsupported(model, apiModeChatCompletions)
}

func (c *Client) MarkResponsesUnsupported(model string) {
	c.markAPIModeUnsupported(model, apiModeResponses)
}

func (c *Client) MarkResponseMaxOutputTokensUnsupported() {
	c.apiModeMu.Lock()
	defer c.apiModeMu.Unlock()
	c.skipResponseMaxOutputTokens = true
}

func (c *Client) responseMaxOutputTokensEnabled() bool {
	c.apiModeMu.RLock()
	defer c.apiModeMu.RUnlock()
	return !c.skipResponseMaxOutputTokens
}
