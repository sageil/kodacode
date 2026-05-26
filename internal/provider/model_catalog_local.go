package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

func (c *ModelCatalog) fetchLocalModels(ctx context.Context, endpoint LocalModelCatalogProvider) ([]CatalogModel, error) {
	root := modelCatalogRoot(endpoint.BaseURL)
	if root == "" {
		return nil, fmt.Errorf("%s: local base_url is required", endpoint.ID)
	}

	ctx, cancel := context.WithTimeout(ctx, defaultModelCatalogTimeout)
	defer cancel()

	body, err := httpGetBody(ctx, root+"/models")
	if err != nil {
		return nil, fmt.Errorf("%s: fetch models: %w", endpoint.ID, err)
	}

	models, ok := parseOpenAIModelsResponse(body)
	if !ok {
		return nil, fmt.Errorf("%s: unsupported models response shape", endpoint.ID)
	}
	nativeRoot := strings.TrimSuffix(root, "/v1")
	lmStudioNative := false
	if nativeBody, err := httpGetBody(ctx, nativeRoot+"/api/v1/models"); err == nil {
		enrichFromLMStudioNative(models, nativeBody)
		lmStudioNative = true
	}
	if !lmStudioNative && len(models) > 0 {
		if tryFetchOllamaModelInfo(ctx, nativeRoot, &models[0]) {
			var wg sync.WaitGroup
			sem := make(chan struct{}, localCapabilityProbeParallel)
			for idx := 1; idx < len(models); idx++ {
				wg.Add(1)
				go func(model *CatalogModel) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					_ = tryFetchOllamaModelInfo(ctx, nativeRoot, model)
				}(&models[idx])
			}
			wg.Wait()
		}
	}
	return models, nil
}

func httpGetBody(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func parseOpenAIModelsResponse(body []byte) ([]CatalogModel, bool) {
	type modelEntry struct {
		ID               string   `json:"id"`
		Type             string   `json:"type"`
		DisplayName      string   `json:"display_name"`
		MaxContextLength int      `json:"max_context_length"`
		ContextLength    int      `json:"context_length"`
		Capabilities     []string `json:"capabilities"`
		Pricing          struct {
			Input  float64 `json:"input"`
			Output float64 `json:"output"`
		} `json:"pricing"`
	}
	buildModels := func(entries []modelEntry) []CatalogModel {
		models := make([]CatalogModel, 0, len(entries))
		for _, entry := range entries {
			model := CatalogModel{
				ID:         strings.TrimSpace(entry.ID),
				Name:       firstNonBlank(strings.TrimSpace(entry.DisplayName), strings.TrimSpace(entry.ID)),
				CostInput:  entry.Pricing.Input,
				CostOutput: entry.Pricing.Output,
			}
			if model.ID == "" {
				continue
			}
			model.ContextSize = max(entry.MaxContextLength, entry.ContextLength)
			switch strings.ToLower(strings.TrimSpace(entry.Type)) {
			case "vlm":
				model.Vision = true
				model.VisionKnown = true
				model.OutputModalities = []string{"text"}
			case "chat", "language", "code":
				model.OutputModalities = []string{"text"}
			case "image":
				model.OutputModalities = []string{"image"}
			case "embedding":
				model.OutputModalities = []string{"embedding"}
			case "rerank":
				model.OutputModalities = []string{"ranking"}
			case "moderation":
				model.OutputModalities = []string{"moderation"}
			}
			for _, capability := range entry.Capabilities {
				if capability == "tool_use" {
					model.ToolCalls = true
					model.ToolCallsKnown = true
				}
			}
			models = append(models, model)
		}
		return models
	}

	var objectPayload struct {
		Data []modelEntry `json:"data"`
	}
	if err := json.Unmarshal(body, &objectPayload); err == nil && objectPayload.Data != nil {
		return buildModels(objectPayload.Data), true
	}

	var arrayPayload []modelEntry
	if err := json.Unmarshal(body, &arrayPayload); err == nil && arrayPayload != nil {
		return buildModels(arrayPayload), true
	}

	return nil, false
}

func enrichFromLMStudioNative(models []CatalogModel, body []byte) {
	var payload struct {
		Models []json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}

	type nativeModel struct {
		Key              string          `json:"key"`
		MaxContextLength int             `json:"max_context_length"`
		Capabilities     json.RawMessage `json:"capabilities"`
	}

	nativeModels := make(map[string]nativeModel, len(payload.Models))
	for _, raw := range payload.Models {
		var model nativeModel
		if err := json.Unmarshal(raw, &model); err == nil && strings.TrimSpace(model.Key) != "" {
			nativeModels[model.Key] = model
		}
	}

	for idx := range models {
		nativeModel, ok := nativeModels[models[idx].ID]
		if !ok {
			continue
		}
		if nativeModel.MaxContextLength > 0 && models[idx].ContextSize == 0 {
			models[idx].ContextSize = nativeModel.MaxContextLength
		}
		if len(nativeModel.Capabilities) == 0 {
			continue
		}
		var capabilities struct {
			Vision          bool            `json:"vision"`
			TrainedForTools bool            `json:"trained_for_tool_use"`
			Reasoning       json.RawMessage `json:"reasoning"`
		}
		if err := json.Unmarshal(nativeModel.Capabilities, &capabilities); err != nil {
			continue
		}
		if capabilities.TrainedForTools {
			models[idx].ToolCalls = true
			models[idx].ToolCallsKnown = true
		}
		if capabilities.Vision {
			models[idx].Vision = true
			models[idx].VisionKnown = true
		}
		if len(capabilities.Reasoning) > 0 && string(capabilities.Reasoning) != "null" && string(capabilities.Reasoning) != "false" {
			models[idx].Reasoning = true
		}
	}
}

func tryFetchOllamaModelInfo(ctx context.Context, baseURL string, model *CatalogModel) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	body, _ := json.Marshal(map[string]string{"model": model.ID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/show", bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	var payload struct {
		ModelInfo    map[string]any `json:"model_info"`
		Capabilities []string       `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false
	}

	for key, value := range payload.ModelInfo {
		if !strings.Contains(strings.ToLower(key), "context_length") {
			continue
		}
		if parsed, ok := value.(float64); ok && parsed > 0 && model.ContextSize == 0 {
			model.ContextSize = int(parsed)
		}
	}
	for _, capability := range payload.Capabilities {
		switch capability {
		case "tools":
			model.ToolCalls = true
			model.ToolCallsKnown = true
		case "vision":
			model.Vision = true
			model.VisionKnown = true
		case "thinking":
			model.Reasoning = true
		}
	}
	return true
}
