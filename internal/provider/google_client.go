package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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

func buildGoogleGenerateContentConfig(req Request) (*genai.GenerateContentConfig, error) {
	if err := rejectCustomTools("google", req.Tools); err != nil {
		return nil, err
	}
	req = NormalizePromptRequest(req)
	config := &genai.GenerateContentConfig{}
	if maxOutputTokens := googleMaxOutputTokens(req); maxOutputTokens > 0 {
		config.MaxOutputTokens = maxOutputTokens
	}
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(instructions)},
		}
	}
	if len(req.Tools) > 0 {
		decls := make([]*genai.FunctionDeclaration, 0, len(req.Tools))
		for _, tool := range req.Tools {
			schema, err := googleToolSchema(tool)
			if err != nil {
				return nil, err
			}
			decls = append(decls, &genai.FunctionDeclaration{
				Name:                 tool.Name,
				Description:          tool.Description,
				ParametersJsonSchema: schema,
			})
		}
		config.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
		config.ToolConfig = &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAuto,
			},
		}
	}

	if googleThinkingModel(req.Model.ModelID) {
		thinking := &genai.ThinkingConfig{
			IncludeThoughts: req.ThinkingEnabled,
		}
		if variant := strings.TrimSpace(strings.ToLower(req.ThinkingMode)); variant != "" {
			effective := effectiveGoogleReasoningVariant(req.Model, variant)
			if effective == "" {
				return nil, errUnsupportedReasoningVariant(req.Model, variant)
			}
			modelID := strings.ToLower(strings.TrimSpace(req.Model.ModelID))
			switch {
			case googleGemini3ProModel(modelID), googleGemini3FlashModel(modelID):
				thinking.ThinkingLevel = genai.ThinkingLevel(strings.ToUpper(effective))
			default:
				budget, err := strconv.ParseInt(effective, 10, 32)
				if err != nil {
					return nil, errUnsupportedReasoningVariant(req.Model, variant)
				}
				thinking.ThinkingBudget = genai.Ptr(int32(budget))
			}
		}
		if thinking.IncludeThoughts || thinking.ThinkingLevel != "" || thinking.ThinkingBudget != nil {
			config.ThinkingConfig = thinking
		}
	}

	return config, nil
}

func googleMaxOutputTokens(req Request) int32 {
	maxOutputTokens := EffectiveMaxOutputTokens(req)
	if maxOutputTokens <= 0 {
		return 0
	}
	if maxOutputTokens > 1<<31-1 {
		return 1<<31 - 1
	}
	return int32(maxOutputTokens)
}

func googleToolSchema(tool Tool) (any, error) {
	if strings.TrimSpace(tool.InputSchema) == "" {
		return nil, nil
	}
	var schema any
	if err := json.Unmarshal([]byte(tool.InputSchema), &schema); err != nil {
		return nil, fmt.Errorf("google: unmarshal tool schema %q: %w", tool.Name, err)
	}
	return relaxGoogleToolSchema(schema), nil
}

func relaxGoogleToolSchema(schema any) any {
	object, ok := schema.(map[string]any)
	if !ok {
		return schema
	}
	properties, ok := object["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return schema
	}
	required, ok := object["required"].([]any)
	if !ok || len(required) == 0 {
		return schema
	}
	filtered := make([]any, 0, len(required))
	changed := false
	for _, raw := range required {
		name, ok := raw.(string)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		if propertyAllowsNull(properties[name]) {
			changed = true
			continue
		}
		filtered = append(filtered, name)
	}
	if changed {
		object["required"] = filtered
	}
	return object
}

func propertyAllowsNull(schema any) bool {
	property, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	switch typed := property["type"].(type) {
	case string:
		return typed == "null"
	case []any:
		for _, item := range typed {
			name, ok := item.(string)
			if ok && name == "null" {
				return true
			}
		}
	}
	return false
}

func buildGoogleConversation(conversation []nativeMessage) ([]*genai.Content, error) {
	callNames := make(map[string]string)
	for _, message := range conversation {
		for _, part := range message.parts {
			call, ok := part.(nativeToolCallPart)
			if !ok {
				continue
			}
			callNames[call.id] = call.name
		}
	}

	contents := make([]*genai.Content, 0, len(conversation))
	for _, message := range conversation {
		role := "user"
		if message.role == "assistant" {
			role = "model"
		}
		parts := make([]*genai.Part, 0, len(message.parts))
		for _, raw := range message.parts {
			switch part := raw.(type) {
			case nativeTextPart:
				if part.text != "" {
					parts = append(parts, genai.NewPartFromText(part.text))
				}
			case nativeAnthropicThinkingPart:
				continue
			case nativeImagePart:
				data, err := attachmentBytes(part.attachment)
				if err != nil {
					return nil, err
				}
				parts = append(parts, genai.NewPartFromBytes(data, part.attachment.MIMEType))
			case nativeToolCallPart:
				var args map[string]any
				if trimmed := strings.TrimSpace(part.arguments); trimmed != "" {
					if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
						return nil, fmt.Errorf("google: unmarshal tool arguments for %q: %w", part.name, err)
					}
				}
				item := genai.NewPartFromFunctionCall(part.name, args)
				item.FunctionCall.ID = part.id
				item.ThoughtSignature = append([]byte(nil), part.googleThoughtSignature...)
				parts = append(parts, item)
			case nativeToolResultPart:
				name := strings.TrimSpace(callNames[part.toolCallID])
				if name == "" {
					name = part.toolCallID
				}
				parts = append(parts, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						ID:       part.toolCallID,
						Name:     name,
						Response: buildNativeToolResultObject(part),
					},
				})
			default:
				return nil, fmt.Errorf("google: unsupported conversation part %T", raw)
			}
		}
		if len(parts) == 0 {
			continue
		}
		if n := len(contents); n > 0 && contents[n-1].Role == role {
			contents[n-1].Parts = append(contents[n-1].Parts, parts...)
			continue
		}
		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}
	return contents, nil
}
