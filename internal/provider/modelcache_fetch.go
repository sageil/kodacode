package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const localCapabilityProbeConcurrency = 4

func (mc *ModelCache) fetchLocalModels(ctx context.Context, ep LocalProviderEndpoint) []modelsDevModel {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	body, err := httpGetBody(ctx, ep.BaseURL+"/models")
	if err != nil {
		log.Printf("modelcache: %s: fetch failed (is it running?): %v", ep.ID, err)
		return nil
	}

	models := parseOpenAIModelsResponse(body)

	// LM Studio native API (/api/v1/models) returns richer capability data.
	nativeBase := strings.TrimSuffix(ep.BaseURL, "/v1")
	lmStudioNative := false
	if nativeBody, err := httpGetBody(ctx, nativeBase+"/api/v1/models"); err == nil {
		enrichFromLMStudioNative(models, nativeBody)
		lmStudioNative = true
	}

	if !lmStudioNative && len(models) > 0 {
		ollamaBase := strings.TrimSuffix(ep.BaseURL, "/v1")
		if tryFetchOllamaModelInfo(ctx, ollamaBase, &models[0]) {
			var wg sync.WaitGroup
			sem := make(chan struct{}, localCapabilityProbeConcurrency)
			for i := 1; i < len(models); i++ {
				wg.Add(1)
				go func(m *modelsDevModel) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					_ = tryFetchOllamaModelInfo(ctx, ollamaBase, m)
				}(&models[i])
			}
			wg.Wait()
		}
	}

	log.Printf("modelcache: %s: discovered %d models from %s", ep.ID, len(models), ep.BaseURL)
	return models
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
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func parseOpenAIModelsResponse(body []byte) []modelsDevModel {
	var result struct {
		Data []struct {
			ID               string   `json:"id"`
			Type             string   `json:"type"`
			MaxContextLength int      `json:"max_context_length"`
			Capabilities     []string `json:"capabilities"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) != nil {
		return nil
	}

	models := make([]modelsDevModel, 0, len(result.Data))
	for _, m := range result.Data {
		mdl := modelsDevModel{ID: m.ID, Name: m.ID}
		if m.MaxContextLength > 0 {
			mdl.Limit.Context = m.MaxContextLength
		}
		if m.Type == "vlm" {
			mdl.Attachment = true
			mdl.AttachmentKnown = true
			mdl.VisionKnown = true
		}
		for _, cap := range m.Capabilities {
			switch cap {
			case "tool_use":
				mdl.ToolCall = true
				mdl.ToolCallKnown = true
			}
		}
		models = append(models, mdl)
	}
	return models
}

func enrichFromLMStudioNative(models []modelsDevModel, body []byte) {
	var result struct {
		Models []json.RawMessage `json:"models"`
	}
	if json.Unmarshal(body, &result) != nil {
		return
	}

	type nativeModel struct {
		Key              string          `json:"key"`
		MaxContextLength int             `json:"max_context_length"`
		Capabilities     json.RawMessage `json:"capabilities"`
	}

	native := make(map[string]nativeModel, len(result.Models))
	for _, raw := range result.Models {
		var m nativeModel
		if json.Unmarshal(raw, &m) == nil && m.Key != "" {
			native[m.Key] = m
		}
	}

	for i := range models {
		n, ok := native[models[i].ID]
		if !ok {
			continue
		}
		if n.MaxContextLength > 0 && models[i].Limit.Context == 0 {
			models[i].Limit.Context = n.MaxContextLength
		}
		if len(n.Capabilities) == 0 {
			continue
		}
		var caps struct {
			Vision          bool            `json:"vision"`
			TrainedForTools bool            `json:"trained_for_tool_use"`
			Reasoning       json.RawMessage `json:"reasoning"`
		}
		if json.Unmarshal(n.Capabilities, &caps) != nil {
			continue
		}
		if caps.TrainedForTools {
			models[i].ToolCall = true
			models[i].ToolCallKnown = true
		}
		if caps.Vision {
			models[i].Attachment = true
			models[i].AttachmentKnown = true
			models[i].VisionKnown = true
		}
		if len(caps.Reasoning) > 0 && string(caps.Reasoning) != "null" && string(caps.Reasoning) != "false" {
			models[i].Reasoning = true
		}
	}
}

func tryFetchOllamaModelInfo(ctx context.Context, baseURL string, mdl *modelsDevModel) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	endpoint := baseURL + "/api/show"
	body, _ := json.Marshal(map[string]string{"model": mdl.ID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		log.Printf("modelcache: ollama %s: request error: %v", mdl.ID, err)
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

	var result struct {
		ModelInfo    map[string]any `json:"model_info"`
		Capabilities []string       `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("modelcache: ollama %s: decode error: %v", mdl.ID, err)
		return false
	}

	for k, v := range result.ModelInfo {
		if strings.Contains(strings.ToLower(k), "context_length") {
			if f, ok := v.(float64); ok && f > 0 && mdl.Limit.Context == 0 {
				mdl.Limit.Context = int(f)
			}
		}
	}

	if len(result.Capabilities) == 0 {
		log.Printf("modelcache: ollama %s: no capabilities reported (upgrade to ollama 0.5.0+)", mdl.ID)
	}
	for _, cap := range result.Capabilities {
		switch cap {
		case "tools":
			mdl.ToolCall = true
			mdl.ToolCallKnown = true
		case "vision":
			mdl.Attachment = true
			mdl.AttachmentKnown = true
			mdl.VisionKnown = true
		case "thinking":
			mdl.Reasoning = true
		}
	}
	log.Printf("modelcache: ollama %s: capabilities=%v context=%d", mdl.ID, result.Capabilities, mdl.Limit.Context)
	return true
}

func (mc *ModelCache) fetchModelsDevData(ctx context.Context) map[string]modelsDevProvider {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsDevURL, nil)
	if err != nil {
		log.Printf("modelcache: failed to create request: %v", err)
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("modelcache: fetch failed: %v", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		log.Printf("modelcache: fetch returned %d", resp.StatusCode)
		return nil
	}

	rbody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("modelcache: failed to read response: %v", err)
		return nil
	}

	var providers map[string]modelsDevProvider
	if err := json.Unmarshal(rbody, &providers); err != nil {
		log.Printf("modelcache: failed to parse response: %v", err)
		return nil
	}

	log.Printf("modelcache: fetched %d providers from %s", len(providers), modelsDevURL)
	return providers
}
