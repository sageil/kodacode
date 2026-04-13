package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type CopilotModel struct {
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

type copilotModelsResponse struct {
	Data []copilotModelPayload `json:"data"`
}

type copilotModelPayload struct {
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

func FetchCopilotModels(ctx context.Context, token string) ([]CopilotModel, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("copilot models: missing token")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, copilotModelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("copilot models: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot models: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot models: HTTP %d", resp.StatusCode)
	}

	var payload copilotModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("copilot models: decode: %w", err)
	}

	merged := make(map[string]CopilotModel, len(payload.Data))
	for _, item := range payload.Data {
		model := CopilotModel{
			ID:                 item.ID,
			Name:               item.Name,
			Family:             item.Capabilities.Family,
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
		if existing, ok := merged[model.ID]; ok {
			merged[model.ID] = mergeCopilotModel(existing, model)
			continue
		}
		merged[model.ID] = model
	}

	out := make([]CopilotModel, 0, len(merged))
	for _, model := range merged {
		out = append(out, model)
	}
	return out, nil
}

func mergeCopilotModel(primary, fallback CopilotModel) CopilotModel {
	merged := primary
	if merged.Name == "" {
		merged.Name = fallback.Name
	}
	if merged.Family == "" {
		merged.Family = fallback.Family
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
	if !merged.ModelPickerEnabled {
		merged.ModelPickerEnabled = fallback.ModelPickerEnabled
	}
	if len(merged.SupportedEndpoints) == 0 {
		merged.SupportedEndpoints = cloneStrings(fallback.SupportedEndpoints)
		return merged
	}
	if len(fallback.SupportedEndpoints) == 0 {
		return merged
	}
	seen := make(map[string]bool, len(merged.SupportedEndpoints)+len(fallback.SupportedEndpoints))
	endpoints := make([]string, 0, len(merged.SupportedEndpoints)+len(fallback.SupportedEndpoints))
	for _, endpoint := range append(append([]string(nil), merged.SupportedEndpoints...), fallback.SupportedEndpoints...) {
		if endpoint == "" || seen[endpoint] {
			continue
		}
		seen[endpoint] = true
		endpoints = append(endpoints, endpoint)
	}
	merged.SupportedEndpoints = endpoints
	return merged
}
