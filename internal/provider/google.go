package provider

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"google.golang.org/genai"
)

// compile-time interface checks
var _ Provider = (*GoogleProvider)(nil)
var _ TokenCounter = (*GoogleProvider)(nil)
var _ AttachmentSupporter = (*GoogleProvider)(nil)
var _ StaticModelProvider = (*GoogleProvider)(nil)

// googleCallIDCounter generates unique tool call IDs since Google's API
// does not provide them (unlike OpenAI/Anthropic).
var googleCallIDCounter atomic.Int64

// GoogleProvider implements the provider.Provider interface using the
// official Google Gemini genai SDK.
type GoogleProvider struct {
	client         *genai.Client
	isOAuth        bool     // true when using Cloud Code Assist (no context caching support)
	oauthModels    []string // model IDs confirmed available via quota endpoint
	skipToolChoice bool
	configured     []Model

	extraMods []string           // extra model IDs from config, merged on first use

	// mu protects cacheByModel.
	mu           sync.Mutex
	cacheByModel map[string]*googleCacheEntry
}

// NewGoogleProvider creates a new Google Gemini provider.
// The genai client is initialised once and reused across all Chat calls.
func NewGoogleProvider(ctx context.Context, apiKey string) (*GoogleProvider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("google: create client: %w", err)
	}
	return &GoogleProvider{
		client:       client,
		cacheByModel: make(map[string]*googleCacheEntry),
	}, nil
}

// MarkToolChoiceUnsupported disables FunctionCallingConfig for future calls.
func (p *GoogleProvider) MarkToolChoiceUnsupported() {
	p.skipToolChoice = true
}

func (p *GoogleProvider) SetConfiguredModels(models []Model) {
	p.configured = cloneModels(models)
}

func (p *GoogleProvider) ID() string { return "google" }

func (p *GoogleProvider) Name() string { return "Google" }

func (p *GoogleProvider) AttachmentCapabilities(model Model) AttachmentCapabilities {
	return (AttachmentCapabilities{
		Images: true,
		PDFs:   true,
		Text:   true,
		Binary: true,
	}).ForModel(model)
}

func (p *GoogleProvider) StaticModels() []Model {
	var base []Model
	if !p.isOAuth {
		base = googleStaticModels()
		return mergeConfiguredStaticModels(base, p.configured)
	}

	ids := append([]string(nil), p.oauthModels...)
	if len(ids) == 0 {
		ids = append(ids, cloudCodeAssistModels...)
	}
	ids = append(ids, p.extraMods...)

	seen := make(map[string]bool, len(ids))
	models := make([]Model, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, Model{
			ID:            id,
			Name:          id,
			ContextSize:   googleDefaultContextSize,
			Reasoning:     isThinkingModel(id),
			ToolCall:      true,
			ToolCallKnown: true,
		})
	}
	return mergeConfiguredStaticModels(models, p.configured)
}

func (p *GoogleProvider) Models(ctx context.Context) ([]Model, error) {

	if p.isOAuth {
		ids := p.oauthModels
		if len(ids) == 0 {
			ids = cloudCodeAssistModels // fallback when quota fetch failed
		}
		models := make([]Model, 0, len(ids))
		for _, id := range ids {
			models = append(models, Model{
				ID:            id,
				Name:          id,
				ContextSize:   googleDefaultContextSize,
				Reasoning:     isThinkingModel(id),
				ToolCall:      true,
				ToolCallKnown: true,
			})
		}
		return mergeConfiguredStaticModels(models, p.configured), nil
	}
	models, err := p.listModels(ctx)
	if err != nil {
		log.Printf("google: list models failed, using static fallback: %v", err)
		return p.StaticModels(), nil
	}
	return mergeConfiguredStaticModels(models, p.configured), nil
}

// CountTokens implements provider.Provider using the Gemini count tokens API.
func (p *GoogleProvider) CountTokens(
	ctx context.Context,
	model string,
	messages []Message,
	opts ChatOptions,
) (int, error) {

	thinking := isThinkingModel(model)
	contents, err := convertGoogleMessages(messages, thinking)
	if err != nil {
		return 0, fmt.Errorf("google: convert messages: %w", err)
	}
	config, err := buildGoogleConfig(opts, p.skipToolChoice, thinking)
	if err != nil {
		return 0, fmt.Errorf("google: build config: %w", err)
	}

	countCfg := &genai.CountTokensConfig{
		SystemInstruction: config.SystemInstruction,
		Tools:             config.Tools,
	}
	result, err := p.client.Models.CountTokens(ctx, model, contents, countCfg)
	if err != nil {
		return 0, fmt.Errorf("google: count tokens: %w", err)
	}
	return int(result.TotalTokens), nil
}

// Chat implements provider.Provider. It opens a streaming Gemini chat
// completion and returns a channel of StreamChunks. The channel is always
// closed by the background goroutine. ctx cancellation is propagated.
func (p *GoogleProvider) Chat(
	ctx context.Context,
	model string,
	messages []Message,
	opts ChatOptions,
) (<-chan StreamChunk, error) {

	// Gemini 2.5+ models support thinking and require thoughtSignature on
	// function call parts. Use the model name to detect thinking support.
	thinking := isThinkingModel(model)
	contents, err := convertGoogleMessages(messages, thinking)
	if err != nil {
		return nil, fmt.Errorf("google: convert messages: %w", err)
	}
	config, err := buildGoogleConfig(opts, p.skipToolChoice, thinking)
	if err != nil {
		return nil, fmt.Errorf("google: build config: %w", err)
	}

	// Try to use or create a context cache for the system instructions + tools.
	// Skip for OAuth sessions — Cloud Code Assist doesn't support context caching.
	var cachedName string
	if !p.isOAuth {
		cachedName = p.ensureCache(ctx, model, opts)
	}
	if cachedName != "" {
		config.CachedContent = cachedName
		// Clear system instruction and tools — they're in the cache.
		config.SystemInstruction = nil
		config.Tools = nil
	}

	stream := p.client.Models.GenerateContentStream(ctx, model, contents, config)

	ch := make(chan StreamChunk, 64)
	go consumeGoogleStream(ctx, stream, ch)
	return ch, nil
}

// googleDefaultContextSize is the context window size shared by all current Gemini models.
const googleDefaultContextSize = 1048576

// cloudCodeAssistModels is the set of models available on Cloud Code Assist (OAuth).
// Matches the Gemini CLI's VALID_GEMINI_MODELS. Used as fallback when the quota
// endpoint is unreachable.
var cloudCodeAssistModels = []string{
	"gemini-3.1-pro-preview",
	"gemini-3-pro-preview",
	"gemini-3-flash-preview",
	"gemini-3.1-flash-lite-preview",
	"gemini-2.5-pro",
	"gemini-2.5-flash",
	"gemini-2.5-flash-lite",
}

// listModels queries the Gemini API for available models and returns those
// suitable for coding assistance (Gemini chat models with tool support).
func (p *GoogleProvider) listModels(ctx context.Context) ([]Model, error) {
	var models []Model
	for m, err := range p.client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("iterate models: %w", err)
		}
		if !slices.Contains(m.SupportedActions, "generateContent") {
			continue
		}
		id := strings.TrimPrefix(m.Name, "models/")
		if !isGeminiChatModel(id) {
			continue
		}
		name := m.DisplayName
		if name == "" {
			name = id
		}
		models = append(models, Model{
			ID:            id,
			Name:          name,
			ContextSize:   int(m.InputTokenLimit),
			Reasoning:     m.Thinking,
			ToolCall:      true,
			ToolCallKnown: true,
		})
	}
	if len(models) == 0 {
		return googleStaticModels(), nil
	}
	return models, nil
}

// isThinkingModel returns true for Gemini models that support thinking and
// require thoughtSignature on function call parts. All Gemini models from
// 2.5 onward support thinking; older models (1.x, 2.0) do not.
func isThinkingModel(model string) bool {
	if !strings.HasPrefix(model, "gemini-") {
		return false
	}
	for _, legacy := range []string{"gemini-1.", "gemini-1-", "gemini-2.0", "gemini-2-0"} {
		if strings.HasPrefix(model, legacy) {
			return false
		}
	}
	return true
}

// isGeminiChatModel returns true for Gemini models suitable for coding assistance.
// Excludes non-Gemini models (gemma, lyria), specialized variants (TTS, image
// generation, computer-use, robotics, deep-research), versioned duplicates
// (e.g. gemini-2.0-flash-001), and embedding/live models.
// Used by both the List API path (google.go) and the models.dev cache (modelcache.go).
func isGeminiChatModel(id string) bool {
	if !strings.HasPrefix(id, "gemini-") {
		return false
	}
	// Gemini 1.x and 2.0 models are deprecated and return 404 from the API.
	if strings.HasPrefix(id, "gemini-1.") || strings.HasPrefix(id, "gemini-1-") ||
		strings.HasPrefix(id, "gemini-2.0") {
		return false
	}
	for _, suffix := range []string{"-tts", "-image", "-computer-use", "-robotics", "-customtools", "-embedding", "-live"} {
		if strings.Contains(id, suffix) {
			return false
		}
	}
	if strings.HasPrefix(id, "deep-research") {
		return false
	}
	// Skip date-stamped versions (e.g. "gemini-2.5-flash-001", "preview-09-2025")
	// but keep named previews like "gemini-3-pro-preview".
	parts := strings.Split(id, "-")
	last := parts[len(parts)-1]
	if len(last) >= 2 && last[0] >= '0' && last[0] <= '9' {
		return false
	}
	// Also skip aliases like "gemini-flash-latest" — they duplicate real model IDs.
	if strings.HasSuffix(id, "-latest") {
		return false
	}
	return true
}

// googleStaticModels is the hardcoded fallback when the List API is unavailable.
func googleStaticModels() []Model {
	return []Model{
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", ContextSize: googleDefaultContextSize, ToolCall: true, ToolCallKnown: true},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", ContextSize: googleDefaultContextSize, ToolCall: true, ToolCallKnown: true},
		{ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash Lite", ContextSize: googleDefaultContextSize, ToolCall: true, ToolCallKnown: true},
	}
}
