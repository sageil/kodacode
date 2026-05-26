package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type GitHubCopilotModel struct {
	ID                 string
	Name               string
	Family             string
	ModelPickerEnabled bool
	SupportedEndpoints []string
	ContextSize        int
	MaxInputTokens     int
	MaxOutputTokens    int
	ToolCalls          bool
	Vision             bool
	Reasoning          bool
}

type gitHubCopilotModelsResponse struct {
	Data []gitHubCopilotModelPayload `json:"data"`
}

type gitHubCopilotModelPayload struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	ModelPickerEnabled bool     `json:"model_picker_enabled"`
	SupportedEndpoints []string `json:"supported_endpoints"`
	Capabilities       struct {
		Family string `json:"family"`
		Limits struct {
			MaxContextWindowTokens int `json:"max_context_window_tokens"`
			MaxPromptTokens        int `json:"max_prompt_tokens"`
			MaxOutputTokens        int `json:"max_output_tokens"`
		} `json:"limits"`
		Supports struct {
			AdaptiveThinking  bool     `json:"adaptive_thinking"`
			MaxThinkingBudget int      `json:"max_thinking_budget"`
			MinThinkingBudget int      `json:"min_thinking_budget"`
			ReasoningEffort   []string `json:"reasoning_effort"`
			ToolCalls         bool     `json:"tool_calls"`
			Vision            bool     `json:"vision"`
		} `json:"supports"`
	} `json:"capabilities"`
}

func FetchGitHubCopilotModels(ctx context.Context, config GitHubCopilotConfig) ([]GitHubCopilotModel, error) {
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	authorizer, err := newGitHubCopilotAuthorizer(config, client)
	if err != nil {
		return nil, fmt.Errorf("copilot models: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := doOpenAIAuthorizedRequest(ctx, client, authorizer, "copilot models", func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, gitHubCopilotModelsURL(config.BaseURL), nil)
		if err != nil {
			return nil, fmt.Errorf("copilot models: create request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var payload gitHubCopilotModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("copilot models: decode: %w", err)
	}

	merged := make(map[string]GitHubCopilotModel, len(payload.Data))
	for _, item := range payload.Data {
		modelID := strings.TrimSpace(item.ID)
		if modelID == "" {
			continue
		}
		model := GitHubCopilotModel{
			ID:                 modelID,
			Name:               strings.TrimSpace(item.Name),
			Family:             strings.TrimSpace(item.Capabilities.Family),
			ModelPickerEnabled: item.ModelPickerEnabled,
			SupportedEndpoints: cloneStrings(item.SupportedEndpoints),
			ContextSize:        item.Capabilities.Limits.MaxContextWindowTokens,
			MaxInputTokens:     item.Capabilities.Limits.MaxPromptTokens,
			MaxOutputTokens:    item.Capabilities.Limits.MaxOutputTokens,
			ToolCalls:          item.Capabilities.Supports.ToolCalls,
			Vision:             item.Capabilities.Supports.Vision,
			Reasoning: item.Capabilities.Supports.AdaptiveThinking ||
				item.Capabilities.Supports.MaxThinkingBudget > 0 ||
				item.Capabilities.Supports.MinThinkingBudget > 0 ||
				len(item.Capabilities.Supports.ReasoningEffort) > 0,
		}
		if existing, ok := merged[modelID]; ok {
			merged[modelID] = mergeGitHubCopilotModel(existing, model)
			continue
		}
		merged[modelID] = model
	}

	out := make([]GitHubCopilotModel, 0, len(merged))
	for _, model := range merged {
		if !model.ModelPickerEnabled {
			continue
		}
		if model.Name == "" {
			model.Name = model.ID
		}
		out = append(out, model)
	}
	return out, nil
}

func mergeGitHubCopilotModel(primary, fallback GitHubCopilotModel) GitHubCopilotModel {
	merged := primary
	if merged.Name == "" {
		merged.Name = fallback.Name
	}
	if merged.Family == "" {
		merged.Family = fallback.Family
	}
	if !merged.ModelPickerEnabled {
		merged.ModelPickerEnabled = fallback.ModelPickerEnabled
	}
	if merged.ContextSize == 0 {
		merged.ContextSize = fallback.ContextSize
	}
	if merged.MaxInputTokens == 0 {
		merged.MaxInputTokens = fallback.MaxInputTokens
	}
	if merged.MaxOutputTokens == 0 {
		merged.MaxOutputTokens = fallback.MaxOutputTokens
	}
	if !merged.ToolCalls {
		merged.ToolCalls = fallback.ToolCalls
	}
	if !merged.Vision {
		merged.Vision = fallback.Vision
	}
	if !merged.Reasoning {
		merged.Reasoning = fallback.Reasoning
	}
	if len(merged.SupportedEndpoints) == 0 {
		merged.SupportedEndpoints = cloneStrings(fallback.SupportedEndpoints)
		return merged
	}
	if len(fallback.SupportedEndpoints) == 0 {
		return merged
	}
	seen := make(map[string]struct{}, len(merged.SupportedEndpoints)+len(fallback.SupportedEndpoints))
	var endpoints []string
	for _, endpoint := range append(append([]string(nil), merged.SupportedEndpoints...), fallback.SupportedEndpoints...) {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		if _, ok := seen[endpoint]; ok {
			continue
		}
		seen[endpoint] = struct{}{}
		endpoints = append(endpoints, endpoint)
	}
	merged.SupportedEndpoints = endpoints
	return merged
}

func gitHubCopilotModelsURL(baseURL string) string {
	return gitHubCopilotRoot(baseURL) + "/models"
}
