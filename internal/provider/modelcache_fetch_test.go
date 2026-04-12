package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLocalModels_LMStudioV0Capabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			if err := json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"id":                 "qwen2.5-coder-7b",
						"type":               "llm",
						"max_context_length": 32768,
						"capabilities":       []string{"tool_use"},
					},
					{
						"id":                 "llava-1.6-7b",
						"type":               "vlm",
						"max_context_length": 4096,
					},
					{
						"id":   "basic-model",
						"type": "llm",
					},
				},
			}); err != nil {
				t.Fatalf("encode /v1/models response: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	mc := NewModelCache(7)
	models := mc.fetchLocalModels(context.Background(), LocalProviderEndpoint{
		ID:      "lmstudio",
		BaseURL: srv.URL + "/v1",
	})

	if len(models) != 3 {
		t.Fatalf("got %d models, want 3", len(models))
	}

	coder := models[0]
	if !coder.ToolCall || !coder.ToolCallKnown {
		t.Error("qwen2.5-coder-7b should have ToolCall from v0 capabilities")
	}
	if coder.Limit.Context != 32768 {
		t.Errorf("context = %d, want 32768", coder.Limit.Context)
	}

	llava := models[1]
	if !llava.Attachment || !llava.AttachmentKnown {
		t.Error("llava should have Attachment from vlm type")
	}

	basic := models[2]
	if basic.ToolCall || basic.Attachment {
		t.Error("basic-model should have no capabilities")
	}
}

func TestFetchLocalModels_LMStudioNativeAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "qwen2.5-coder-7b", "type": "llm"},
					{"id": "llava-1.6-7b", "type": "llm"},
				},
			}); err != nil {
				t.Fatalf("encode /v1/models response: %v", err)
			}
		case "/api/v1/models":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{
						"key":                "qwen2.5-coder-7b",
						"type":               "llm",
						"max_context_length": 32768,
						"capabilities": map[string]any{
							"vision":               false,
							"trained_for_tool_use": true,
							"reasoning":            map[string]any{"allowed_options": []string{"off", "on"}, "default": "on"},
						},
					},
					{
						"key":                "llava-1.6-7b",
						"type":               "llm",
						"max_context_length": 4096,
						"capabilities":       map[string]any{"vision": true, "trained_for_tool_use": false},
					},
				},
			}); err != nil {
				t.Fatalf("encode /api/v1/models response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	mc := NewModelCache(7)
	models := mc.fetchLocalModels(context.Background(), LocalProviderEndpoint{
		ID:      "lmstudio",
		BaseURL: srv.URL + "/v1",
	})

	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}

	coder := models[0]
	if !coder.ToolCall || !coder.ToolCallKnown {
		t.Error("qwen2.5-coder-7b should have ToolCall from native API")
	}
	if coder.Attachment {
		t.Error("qwen2.5-coder-7b should not have vision")
	}
	if !coder.Reasoning {
		t.Error("qwen2.5-coder-7b should have Reasoning from native API")
	}
	if coder.Limit.Context != 32768 {
		t.Errorf("context = %d, want 32768", coder.Limit.Context)
	}

	llava := models[1]
	if !llava.Attachment || !llava.AttachmentKnown || !llava.VisionKnown {
		t.Error("llava should have vision from native API")
	}
	if llava.ToolCall {
		t.Error("llava should not have ToolCall")
	}
}

func TestFetchOllamaModelInfo_Capabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			if err := json.NewEncoder(w).Encode(map[string]any{
				"model_info": map[string]any{
					"general.context_length": 8192,
				},
				"capabilities": []string{"completion", "tools", "vision"},
			}); err != nil {
				t.Fatalf("encode /api/show response: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	mdl := &modelsDevModel{ID: "test-model"}
	if !tryFetchOllamaModelInfo(context.Background(), srv.URL, mdl) {
		t.Fatal("tryFetchOllamaModelInfo() = false, want true")
	}

	if mdl.Limit.Context != 8192 {
		t.Errorf("context = %d, want 8192", mdl.Limit.Context)
	}
	if !mdl.ToolCall || !mdl.ToolCallKnown {
		t.Error("should have ToolCall from 'tools' capability")
	}
	if !mdl.Attachment || !mdl.AttachmentKnown || !mdl.VisionKnown {
		t.Error("should have vision from 'vision' capability")
	}
}

func TestFetchOllamaModelInfo_Thinking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"capabilities": []string{"completion", "thinking"},
		}); err != nil {
			t.Fatalf("encode /api/show response: %v", err)
		}
	}))
	defer srv.Close()

	mdl := &modelsDevModel{ID: "deepseek-r1"}
	if !tryFetchOllamaModelInfo(context.Background(), srv.URL, mdl) {
		t.Fatal("tryFetchOllamaModelInfo() = false, want true")
	}

	if !mdl.Reasoning {
		t.Error("should have Reasoning from 'thinking' capability")
	}
	if mdl.ToolCall {
		t.Error("should not have ToolCall")
	}
}

func TestFetchOllamaModelInfo_Unavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	mdl := &modelsDevModel{ID: "test-model"}
	if tryFetchOllamaModelInfo(context.Background(), srv.URL, mdl) {
		t.Fatal("tryFetchOllamaModelInfo() = true, want false")
	}

	if mdl.ToolCall || mdl.Attachment || mdl.Reasoning {
		t.Error("should have no capabilities when endpoint unavailable")
	}
}
