package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestModelCatalogUsesModelsDevProviderAliasesAndKeepsProviderReturnedOpenAIModels(t *testing.T) {
	t.Parallel()

	cacheFile := filepath.Join(t.TempDir(), "models-cache.json")
	catalog := NewModelCatalog(ModelCatalogConfig{
		CacheFile:   cacheFile,
		ExpiryDays:  7,
		OpenAIOAuth: true,
		RemoteProviders: []RemoteModelCatalogProvider{
			{ID: "togetherai", Name: "Together AI", Kind: RemoteModelCatalogProviderModelsDev},
			{ID: "openai", Name: "OpenAI", Kind: RemoteModelCatalogProviderOpenAI},
		},
	})
	catalog.fetchEnrichment = func(context.Context) (map[string]catalogProvider, error) {
		return map[string]catalogProvider{
			"together": {
				ID:   "together",
				Name: "Together",
				Models: map[string]CatalogModel{
					"meta-llama/llama-3.3-70b-instruct": {
						ID:   "meta-llama/llama-3.3-70b-instruct",
						Name: "Llama 3.3 70B Instruct",
					},
				},
			},
		}, nil
	}
	catalog.fetchRemote = func(_ context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
		switch source.ID {
		case "openai":
			return &catalogProvider{
				ID:   "openai",
				Name: "OpenAI",
				Models: map[string]CatalogModel{
					"gpt-4.1": {ID: "gpt-4.1", Name: "GPT-4.1"},
					"gpt-5.4": {ID: "gpt-5.4", Name: "GPT-5.4"},
				},
			}, nil
		default:
			return nil, nil
		}
	}

	if err := catalog.EnsureFresh(context.Background()); err != nil {
		t.Fatalf("EnsureFresh() error = %v", err)
	}

	together := catalog.ModelsForProvider("togetherai")
	if len(together) != 1 || together[0].ID != "meta-llama/llama-3.3-70b-instruct" {
		t.Fatalf("together models = %#v", together)
	}
	if got := catalog.ProviderName("togetherai"); got != "Together AI" {
		t.Fatalf("ProviderName(togetherai) = %q", got)
	}

	openAI := catalog.ModelsForProvider("openai")
	if len(openAI) != 2 {
		t.Fatalf("openai models = %#v", openAI)
	}
}

func TestModelCatalogUsesOpenAIModelsDevEnrichmentForCodexProvider(t *testing.T) {
	t.Parallel()

	cacheFile := filepath.Join(t.TempDir(), "models-cache.json")
	catalog := NewModelCatalog(ModelCatalogConfig{
		CacheFile:  cacheFile,
		ExpiryDays: 7,
		RemoteProviders: []RemoteModelCatalogProvider{
			{ID: "openai-codex", Name: "OpenAI Codex", Kind: RemoteModelCatalogProviderOpenAI},
		},
	})
	catalog.fetchEnrichment = func(context.Context) (map[string]catalogProvider, error) {
		return map[string]catalogProvider{
			"openai": {
				ID:   "openai",
				Name: "OpenAI",
				Models: map[string]CatalogModel{
					"gpt-5.2": {
						ID:              "gpt-5.2",
						Name:            "GPT-5.2",
						ContextSize:     128000,
						MaxOutputTokens: 16000,
						ToolCalls:       true,
						ToolCallsKnown:  true,
						CostInput:       1.25,
						CostInputKnown:  true,
						CostOutput:      10,
						CostOutputKnown: true,
					},
				},
			},
		}, nil
	}
	catalog.fetchRemote = func(_ context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
		if source.ID != "openai-codex" {
			return nil, nil
		}
		return &catalogProvider{
			ID:   "openai-codex",
			Name: "OpenAI Codex",
			Models: map[string]CatalogModel{
				"gpt-5.2": {ID: "gpt-5.2", Name: "GPT-5.2 Codex"},
			},
		}, nil
	}

	if err := catalog.EnsureFresh(context.Background()); err != nil {
		t.Fatalf("EnsureFresh() error = %v", err)
	}

	models := catalog.ModelsForProvider("openai-codex")
	if len(models) != 1 {
		t.Fatalf("openai-codex models = %#v", models)
	}
	model := models[0]
	if model.ID != "gpt-5.2" || model.ContextSize != 128000 || model.MaxOutputTokens != 16000 || !model.ToolCalls || !model.ToolCallsKnown {
		t.Fatalf("enriched openai-codex model = %#v", model)
	}
	if model.CostInput != 1.25 || model.CostOutput != 10 || !model.CostInputKnown || !model.CostOutputKnown {
		t.Fatalf("enriched openai-codex costs = %#v", model)
	}
	if got := catalog.ProviderName("openai-codex"); got != "OpenAI Codex" {
		t.Fatalf("ProviderName(openai-codex) = %q, want OpenAI Codex", got)
	}
	if platformModels := catalog.ModelsForProvider("openai"); len(platformModels) != 0 {
		t.Fatalf("openai platform models = %#v, want no availability copied from codex", platformModels)
	}
}

func TestModelCatalogDoesNotFilterOpenAIPlatformModelsWhenAPIKeyAlsoExists(t *testing.T) {
	t.Parallel()

	cacheFile := filepath.Join(t.TempDir(), "models-cache.json")
	catalog := NewModelCatalog(ModelCatalogConfig{
		CacheFile:    cacheFile,
		ExpiryDays:   7,
		OpenAIOAuth:  true,
		OpenAIAPIKey: true,
		RemoteProviders: []RemoteModelCatalogProvider{
			{ID: "openai", Name: "OpenAI", Kind: RemoteModelCatalogProviderOpenAI},
		},
	})
	catalog.fetchRemote = func(_ context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
		return &catalogProvider{
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]CatalogModel{
				"gpt-4.1": {ID: "gpt-4.1", Name: "GPT-4.1"},
				"gpt-5.4": {ID: "gpt-5.4", Name: "GPT-5.4"},
			},
		}, nil
	}

	if err := catalog.EnsureFresh(context.Background()); err != nil {
		t.Fatalf("EnsureFresh() error = %v", err)
	}

	models := catalog.ModelsForProvider("openai")
	if len(models) != 2 {
		t.Fatalf("models = %#v, want both oauth and api-key-visible models", models)
	}
}

func TestModelCatalogRefreshesLocalProvidersOnEveryStartup(t *testing.T) {
	t.Parallel()

	cacheFile := filepath.Join(t.TempDir(), "models-cache.json")
	first := NewModelCatalog(ModelCatalogConfig{
		CacheFile:  cacheFile,
		ExpiryDays: 365,
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:   "openai",
			Name: "OpenAI",
			Kind: RemoteModelCatalogProviderOpenAI,
		}},
		LocalProviders: []LocalModelCatalogProvider{{
			ID:      "ollama",
			Name:    "Ollama",
			BaseURL: "http://localhost:11434/v1",
		}},
	})
	first.fetchRemote = func(context.Context, RemoteModelCatalogProvider) (*catalogProvider, error) {
		return &catalogProvider{
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]CatalogModel{
				"gpt-5.4": {ID: "gpt-5.4", Name: "GPT-5.4"},
			},
		}, nil
	}
	first.fetchLocal = func(context.Context, LocalModelCatalogProvider) ([]CatalogModel, error) {
		return []CatalogModel{{ID: "llama3.2", Name: "Llama 3.2"}}, nil
	}

	if err := first.EnsureFresh(context.Background()); err != nil {
		t.Fatalf("first EnsureFresh() error = %v", err)
	}
	first.Init(context.Background())

	second := NewModelCatalog(ModelCatalogConfig{
		CacheFile:  cacheFile,
		ExpiryDays: 365,
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:   "openai",
			Name: "OpenAI",
			Kind: RemoteModelCatalogProviderOpenAI,
		}},
		LocalProviders: []LocalModelCatalogProvider{{
			ID:      "ollama",
			Name:    "Ollama",
			BaseURL: "http://localhost:11434/v1",
		}},
	})
	second.fetchRemote = func(context.Context, RemoteModelCatalogProvider) (*catalogProvider, error) {
		t.Fatal("fresh remote cache should not refresh on startup")
		return nil, nil
	}
	second.fetchLocal = func(context.Context, LocalModelCatalogProvider) ([]CatalogModel, error) {
		return []CatalogModel{{ID: "qwen3", Name: "Qwen 3"}}, nil
	}

	second.Init(context.Background())

	openAI := second.ModelsForProvider("openai")
	if len(openAI) != 1 || openAI[0].ID != "gpt-5.4" {
		t.Fatalf("openai models = %#v", openAI)
	}
	ollama := second.ModelsForProvider("ollama")
	if len(ollama) != 1 || ollama[0].ID != "qwen3" {
		t.Fatalf("ollama models = %#v", ollama)
	}
}

func TestModelCatalogReportsLocalRefreshFailuresThroughConfiguredReporter(t *testing.T) {
	t.Parallel()

	var (
		reportedMessage string
		reportedErr     error
	)
	catalog := NewModelCatalog(ModelCatalogConfig{
		LocalProviders: []LocalModelCatalogProvider{{
			ID:      "lmstudio",
			Name:    "LM Studio",
			BaseURL: "http://127.0.0.1:1234/v1",
		}},
		ReportError: func(message string, err error) {
			reportedMessage = message
			reportedErr = err
		},
	})
	catalog.fetchLocal = func(context.Context, LocalModelCatalogProvider) ([]CatalogModel, error) {
		return nil, errors.New("connection refused")
	}

	catalog.Init(context.Background())

	if reportedMessage != "model catalog: local refresh failed" {
		t.Fatalf("reported message = %q", reportedMessage)
	}
	if reportedErr == nil || reportedErr.Error() != "connection refused" {
		t.Fatalf("reported error = %v", reportedErr)
	}
}

func TestModelCatalogRefreshForcesRemoteRefresh(t *testing.T) {
	t.Parallel()

	cacheFile := filepath.Join(t.TempDir(), "models-cache.json")
	catalog := NewModelCatalog(ModelCatalogConfig{
		CacheFile:  cacheFile,
		ExpiryDays: 365,
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:   "openai",
			Name: "OpenAI",
			Kind: RemoteModelCatalogProviderOpenAI,
		}},
	})

	calls := 0
	catalog.fetchRemote = func(_ context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
		calls++
		modelID := "gpt-5.4"
		if calls > 1 {
			modelID = "gpt-5.5"
		}
		return &catalogProvider{
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]CatalogModel{
				modelID: {ID: modelID, Name: modelID},
			},
		}, nil
	}

	if err := catalog.EnsureFresh(context.Background()); err != nil {
		t.Fatalf("EnsureFresh() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("initial remote calls = %d, want 1", calls)
	}

	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("remote calls after Refresh = %d, want 2", calls)
	}
	models := catalog.ModelsForProvider("openai")
	if len(models) != 1 || models[0].ID != "gpt-5.5" {
		t.Fatalf("models = %#v", models)
	}
}

func TestMergeCatalogModelsLetsModelsDevOverrideKnownPricing(t *testing.T) {
	t.Parallel()

	merged := mergeCatalogModels(
		CatalogModel{
			ID:              "gpt-5",
			Name:            "GPT-5",
			CostInput:       1.25,
			CostInputKnown:  true,
			CostOutput:      10,
			CostOutputKnown: true,
		},
		CatalogModel{
			ID:                  "gpt-5",
			CostInput:           2,
			CostInputKnown:      true,
			CostOutput:          8,
			CostOutputKnown:     true,
			CostCacheRead:       0.125,
			CostCacheReadKnown:  true,
			CostCacheWrite:      2.5,
			CostCacheWriteKnown: true,
		},
	)

	if merged.CostInput != 2 || merged.CostOutput != 8 {
		t.Fatalf("merged base cost = %#v", merged)
	}
	if merged.CostCacheRead != 0.125 {
		t.Fatalf("CostCacheRead = %f, want 0.125", merged.CostCacheRead)
	}
	if merged.CostCacheWrite != 2.5 {
		t.Fatalf("CostCacheWrite = %f, want 2.5", merged.CostCacheWrite)
	}
}

func TestModelCatalogUsesProviderSourceAndOnlyEnrichesReturnedModels(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(ModelCatalogConfig{
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:   "nvidia",
			Name: "NVIDIA",
			Kind: RemoteModelCatalogProviderOpenAICompatible,
		}},
	})
	catalog.fetchRemote = func(_ context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
		return &catalogProvider{
			ID:   "nvidia",
			Name: "NVIDIA",
			Models: map[string]CatalogModel{
				"meta/llama-3.3-70b-instruct": {
					ID:             "meta/llama-3.3-70b-instruct",
					Name:           "Llama 3.3 70B Instruct",
					MaxInputTokens: 1_000_000,
				},
			},
		}, nil
	}
	catalog.fetchEnrichment = func(context.Context) (map[string]catalogProvider, error) {
		return map[string]catalogProvider{
			"nvidia": {
				ID:   "nvidia",
				Name: "NVIDIA",
				Models: map[string]CatalogModel{
					"meta/llama-3.3-70b-instruct": {
						ID:             "meta/llama-3.3-70b-instruct",
						CostInput:      1.25,
						CostOutput:     10,
						ToolCalls:      true,
						ToolCallsKnown: true,
					},
					"meta/llama-stale": {
						ID:   "meta/llama-stale",
						Name: "Stale Llama",
					},
				},
			},
		}, nil
	}

	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	models := catalog.ModelsForProvider("nvidia")
	if len(models) != 1 {
		t.Fatalf("models = %#v, want only provider-listed model", models)
	}
	if models[0].ID != "meta/llama-3.3-70b-instruct" {
		t.Fatalf("models[0] = %#v", models[0])
	}
	if models[0].CostInput != 1.25 || models[0].CostOutput != 10 {
		t.Fatalf("models[0] cost = %#v", models[0])
	}
	if !models[0].ToolCalls {
		t.Fatalf("models[0] tool calls = %#v", models[0])
	}
}

func TestModelCatalogUsesModelsDevAsAuthorityForExactProviderModelPair(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(ModelCatalogConfig{
		RemoteProviders: []RemoteModelCatalogProvider{
			{
				ID:   "github-copilot",
				Name: "GitHub Copilot",
				Kind: RemoteModelCatalogProviderGitHubCopilot,
			},
			{
				ID:   "openai",
				Name: "OpenAI",
				Kind: RemoteModelCatalogProviderOpenAI,
			},
		},
	})
	catalog.fetchRemote = func(_ context.Context, source RemoteModelCatalogProvider) (*catalogProvider, error) {
		switch source.ID {
		case "github-copilot":
			return &catalogProvider{
				ID:   "github-copilot",
				Name: "GitHub Copilot",
				Models: map[string]CatalogModel{
					"gpt-4.1": {
						ID:              "gpt-4.1",
						Name:            "Provider GPT-4.1",
						ContextSize:     128000,
						MaxInputTokens:  128000,
						MaxOutputTokens: 16384,
						ToolCalls:       true,
						ToolCallsKnown:  true,
						Vision:          false,
						VisionKnown:     true,
					},
				},
			}, nil
		case "openai":
			return &catalogProvider{
				ID:   "openai",
				Name: "OpenAI",
				Models: map[string]CatalogModel{
					"gpt-4.1": {
						ID:              "gpt-4.1",
						Name:            "Provider OpenAI GPT-4.1",
						ContextSize:     128000,
						MaxOutputTokens: 16384,
						ToolCalls:       true,
						ToolCallsKnown:  true,
						Vision:          true,
						VisionKnown:     true,
					},
				},
			}, nil
		default:
			return nil, nil
		}
	}
	catalog.fetchEnrichment = func(context.Context) (map[string]catalogProvider, error) {
		return map[string]catalogProvider{
			"github-copilot": {
				ID:   "github-copilot",
				Name: "GitHub Copilot",
				Models: map[string]CatalogModel{
					"gpt-4.1": {
						ID:                 "gpt-4.1",
						Name:               "Models.dev GPT-4.1",
						ContextSize:        200000,
						MaxInputTokens:     64000,
						MaxOutputTokens:    32768,
						Reasoning:          false,
						ReasoningKnown:     true,
						ToolCalls:          false,
						ToolCallsKnown:     true,
						Vision:             true,
						VisionKnown:        true,
						SupportedEndpoints: []string{"responses"},
					},
				},
			},
			"openai": {
				ID:   "openai",
				Name: "OpenAI",
				Models: map[string]CatalogModel{
					"gpt-4.1": {
						ID:              "gpt-4.1",
						Name:            "Models.dev OpenAI GPT-4.1",
						ContextSize:     1047576,
						MaxOutputTokens: 32768,
						Reasoning:       false,
						ReasoningKnown:  true,
						ToolCalls:       true,
						ToolCallsKnown:  true,
						Vision:          true,
						VisionKnown:     true,
					},
				},
			},
		}, nil
	}

	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	copilotModels := catalog.ModelsForProvider("github-copilot")
	if len(copilotModels) != 1 {
		t.Fatalf("copilot models = %#v, want one provider-returned model", copilotModels)
	}
	copilot := copilotModels[0]
	if copilot.Name != "Models.dev GPT-4.1" {
		t.Fatalf("copilot.Name = %q, want models.dev copilot name", copilot.Name)
	}
	if copilot.ContextSize != 200000 || copilot.MaxInputTokens != 64000 || copilot.MaxOutputTokens != 32768 {
		t.Fatalf("copilot limits = %#v", copilot)
	}
	if copilot.ToolCalls || !copilot.Vision {
		t.Fatalf("copilot capabilities = %#v", copilot)
	}
	if len(copilot.SupportedEndpoints) != 1 || copilot.SupportedEndpoints[0] != "responses" {
		t.Fatalf("copilot endpoints = %#v", copilot.SupportedEndpoints)
	}

	openAIModels := catalog.ModelsForProvider("openai")
	if len(openAIModels) != 1 {
		t.Fatalf("openai models = %#v, want one provider-returned model", openAIModels)
	}
	openAI := openAIModels[0]
	if openAI.Name != "Models.dev OpenAI GPT-4.1" {
		t.Fatalf("openai.Name = %q, want openai models.dev name", openAI.Name)
	}
	if openAI.ContextSize != 1047576 || openAI.MaxInputTokens != 1047576 || openAI.MaxOutputTokens != 32768 {
		t.Fatalf("openai limits = %#v", openAI)
	}
}

func TestModelCatalogUsesProviderModelAliasForNVIDIAGLM5Enrichment(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(ModelCatalogConfig{
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:   "nvidia",
			Name: "NVIDIA",
			Kind: RemoteModelCatalogProviderOpenAICompatible,
		}},
	})
	catalog.fetchRemote = func(context.Context, RemoteModelCatalogProvider) (*catalogProvider, error) {
		return &catalogProvider{
			ID:   "nvidia",
			Name: "NVIDIA",
			Models: map[string]CatalogModel{
				"z-ai/glm5": {
					ID:   "z-ai/glm5",
					Name: "z-ai/glm5",
				},
			},
		}, nil
	}
	catalog.fetchEnrichment = func(context.Context) (map[string]catalogProvider, error) {
		return map[string]catalogProvider{
			"nvidia": {
				ID:   "nvidia",
				Name: "NVIDIA",
				Models: map[string]CatalogModel{
					"z-ai/glm-5.1": {
						ID:              "z-ai/glm-5.1",
						Name:            "GLM-5.1",
						Family:          "glm",
						ContextSize:     202752,
						MaxOutputTokens: 131072,
						Reasoning:       true,
						ToolCalls:       true,
						ToolCallsKnown:  true,
					},
				},
			},
		}, nil
	}

	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	models := catalog.ModelsForProvider("nvidia")
	if len(models) != 1 {
		t.Fatalf("models = %#v, want aliased model", models)
	}
	if models[0].ID != "z-ai/glm5" {
		t.Fatalf("models[0].ID = %q, want z-ai/glm5", models[0].ID)
	}
	if models[0].ContextSize != 202752 || models[0].MaxOutputTokens != 131072 {
		t.Fatalf("models[0] limits = %#v", models[0])
	}
	if !models[0].Reasoning || !models[0].ToolCalls {
		t.Fatalf("models[0] capabilities = %#v", models[0])
	}
}

func TestModelCatalogUsesGoogleProviderSourceAndOnlyEnrichesReturnedGeminiModels(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(ModelCatalogConfig{
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:   "google",
			Name: "Google",
			Kind: RemoteModelCatalogProviderGoogle,
		}},
	})
	catalog.fetchRemote = func(context.Context, RemoteModelCatalogProvider) (*catalogProvider, error) {
		return &catalogProvider{
			ID:   "google",
			Name: "Google",
			Models: map[string]CatalogModel{
				"gemini-2.5-pro": {
					ID:             "gemini-2.5-pro",
					Name:           "Gemini 2.5 Pro",
					MaxInputTokens: 1048576,
				},
				"gemini-2.5-flash": {
					ID:             "gemini-2.5-flash",
					Name:           "Gemini 2.5 Flash",
					MaxInputTokens: 1048576,
				},
			},
		}, nil
	}
	catalog.fetchEnrichment = func(context.Context) (map[string]catalogProvider, error) {
		return map[string]catalogProvider{
			"google": {
				ID:   "google",
				Name: "Google",
				Models: map[string]CatalogModel{
					"gemini-2.5-pro": {
						ID:             "gemini-2.5-pro",
						Name:           "Gemini 2.5 Pro",
						CostInput:      1.25,
						CostOutput:     10,
						Reasoning:      true,
						ToolCalls:      true,
						ToolCallsKnown: true,
					},
					"gemini-2.0-flash-001": {ID: "gemini-2.0-flash-001", Name: "Gemini 2.0 Flash 001"},
					"gemma-3-27b-it":       {ID: "gemma-3-27b-it", Name: "Gemma 3 27B"},
				},
			},
		}, nil
	}

	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	models := catalog.ModelsForProvider("google")
	if len(models) != 2 {
		t.Fatalf("models = %#v, want only provider-returned google models", models)
	}
	if models[0].ID != "gemini-2.5-flash" || models[1].ID != "gemini-2.5-pro" {
		t.Fatalf("models = %#v, want only provider-returned Gemini IDs", models)
	}
	var pro CatalogModel
	for _, model := range models {
		if model.ID == "gemini-2.5-pro" {
			pro = model
			break
		}
	}
	if pro.CostInput != 1.25 || pro.CostOutput != 10 {
		t.Fatalf("gemini-2.5-pro pricing = %#v", pro)
	}
	if !pro.Reasoning || !pro.ToolCalls || !pro.ToolCallsKnown {
		t.Fatalf("gemini-2.5-pro capabilities = %#v", pro)
	}
}

func TestCatalogRemoteSourceFromProviderIgnoresBaseURLForModelsDevProviders(t *testing.T) {
	t.Parallel()

	left := catalogRemoteSourceFromProvider(RemoteModelCatalogProvider{
		ID:      "togetherai",
		Kind:    RemoteModelCatalogProviderModelsDev,
		BaseURL: "https://api.together.xyz/v1",
	})
	right := catalogRemoteSourceFromProvider(RemoteModelCatalogProvider{
		ID:      "togetherai",
		Kind:    RemoteModelCatalogProviderModelsDev,
		BaseURL: "https://proxy.invalid/v1",
	})

	if left != right {
		t.Fatalf("models.dev source mismatch = %#v %#v, want provider-id-only source identity", left, right)
	}
}

func TestCatalogRemoteSourceFromProviderTracksBaseURLForGoogleProviders(t *testing.T) {
	t.Parallel()

	left := catalogRemoteSourceFromProvider(RemoteModelCatalogProvider{
		ID:      "google",
		Kind:    RemoteModelCatalogProviderGoogle,
		BaseURL: "https://generativelanguage.googleapis.com",
	})
	right := catalogRemoteSourceFromProvider(RemoteModelCatalogProvider{
		ID:      "google",
		Kind:    RemoteModelCatalogProviderGoogle,
		BaseURL: "https://proxy.invalid",
	})

	if left == right {
		t.Fatalf("google source = %#v %#v, want base-url-sensitive source identity", left, right)
	}
}

func TestCatalogModelFromGoogleFiltersToGenerateContentGeminiModels(t *testing.T) {
	t.Parallel()

	valid, ok := catalogModelFromGoogle(genai.Model{
		Name:             "models/gemini-2.5-pro",
		DisplayName:      "Gemini 2.5 Pro",
		InputTokenLimit:  1048576,
		OutputTokenLimit: 65536,
		SupportedActions: []string{"generateContent", "countTokens"},
		Thinking:         true,
	})
	if !ok {
		t.Fatal("ok = false, want Gemini generateContent model")
	}
	if valid.ID != "gemini-2.5-pro" || valid.Name != "Gemini 2.5 Pro" {
		t.Fatalf("valid = %#v", valid)
	}
	if !valid.Reasoning {
		t.Fatalf("valid reasoning = %#v", valid)
	}

	if _, ok := catalogModelFromGoogle(genai.Model{
		Name:             "models/gemma-3-27b-it",
		DisplayName:      "Gemma 3 27B",
		SupportedActions: []string{"generateContent"},
	}); ok {
		t.Fatal("ok = true for gemma model, want filtered out")
	}

	if _, ok := catalogModelFromGoogle(genai.Model{
		Name:             "models/gemini-embedding",
		DisplayName:      "Gemini Embedding",
		SupportedActions: []string{"embedContent"},
	}); ok {
		t.Fatal("ok = true for non-generateContent model, want filtered out")
	}
}

func TestModelCatalogRetainsCachedRemoteProviderOnFetchFailure(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(ModelCatalogConfig{
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:      "anthropic",
			Name:    "Anthropic",
			Kind:    RemoteModelCatalogProviderAnthropic,
			BaseURL: "https://api.anthropic.com",
		}},
	})
	catalog.providers = map[string]catalogProvider{
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			Models: map[string]CatalogModel{
				"claude-sonnet-4-5": {ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5"},
			},
		},
	}
	catalog.remoteSources = map[string]catalogRemoteSource{
		"anthropic": catalogRemoteSourceFromProvider(RemoteModelCatalogProvider{
			ID:      "anthropic",
			Name:    "Anthropic",
			Kind:    RemoteModelCatalogProviderAnthropic,
			BaseURL: "https://api.anthropic.com",
		}),
	}
	catalog.fetchRemote = func(context.Context, RemoteModelCatalogProvider) (*catalogProvider, error) {
		return nil, errors.New("boom")
	}

	err := catalog.Refresh(context.Background())
	if err == nil {
		t.Fatal("Refresh() error = nil, want failure")
	}

	models := catalog.ModelsForProvider("anthropic")
	if len(models) != 1 || models[0].ID != "claude-sonnet-4-5" {
		t.Fatalf("models = %#v, want cached provider models after fetch failure", models)
	}
}

func TestModelCatalogDoesNotReuseCachedRemoteProviderAfterSourceChange(t *testing.T) {
	t.Parallel()

	cacheFile := filepath.Join(t.TempDir(), "models-cache.json")
	first := NewModelCatalog(ModelCatalogConfig{
		CacheFile:  cacheFile,
		ExpiryDays: 365,
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:      "openai",
			Name:    "OpenAI",
			Kind:    RemoteModelCatalogProviderOpenAI,
			BaseURL: "https://api.openai.com/v1/responses",
		}},
	})
	first.fetchRemote = func(context.Context, RemoteModelCatalogProvider) (*catalogProvider, error) {
		return &catalogProvider{
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]CatalogModel{
				"gpt-stale": {ID: "gpt-stale", Name: "GPT Stale"},
			},
		}, nil
	}
	if err := first.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh() error = %v", err)
	}

	second := NewModelCatalog(ModelCatalogConfig{
		CacheFile:  cacheFile,
		ExpiryDays: 365,
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:      "openai",
			Name:    "OpenAI",
			Kind:    RemoteModelCatalogProviderOpenAI,
			BaseURL: "https://chatgpt.com/backend-api/codex/responses",
		}},
	})
	second.fetchRemote = func(context.Context, RemoteModelCatalogProvider) (*catalogProvider, error) {
		return nil, errors.New("boom")
	}
	second.Init(context.Background())

	if err := second.EnsureFresh(context.Background()); err == nil {
		t.Fatal("EnsureFresh() error = nil, want failure")
	}
	if models := second.ModelsForProvider("openai"); len(models) != 0 {
		t.Fatalf("models = %#v, want stale cache dropped after source change", models)
	}
}

func TestModelCatalogInitDropsCachedRemoteProviderAfterSourceChange(t *testing.T) {
	t.Parallel()

	cacheFile := filepath.Join(t.TempDir(), "models-cache.json")
	first := NewModelCatalog(ModelCatalogConfig{
		CacheFile:  cacheFile,
		ExpiryDays: 365,
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:      "openai",
			Name:    "OpenAI",
			Kind:    RemoteModelCatalogProviderOpenAI,
			BaseURL: "https://api.openai.com/v1/responses",
		}},
	})
	first.fetchRemote = func(context.Context, RemoteModelCatalogProvider) (*catalogProvider, error) {
		return &catalogProvider{
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]CatalogModel{
				"gpt-stale": {ID: "gpt-stale", Name: "GPT Stale"},
			},
		}, nil
	}
	if err := first.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh() error = %v", err)
	}

	second := NewModelCatalog(ModelCatalogConfig{
		CacheFile:  cacheFile,
		ExpiryDays: 365,
		RemoteProviders: []RemoteModelCatalogProvider{{
			ID:      "openai",
			Name:    "OpenAI",
			Kind:    RemoteModelCatalogProviderOpenAI,
			BaseURL: "https://chatgpt.com/backend-api/codex/responses",
		}},
	})
	second.Init(context.Background())

	if models := second.ModelsForProvider("openai"); len(models) != 0 {
		t.Fatalf("models = %#v, want mismatched-source cache dropped during init", models)
	}
}

func TestMergeCatalogProviderModelsUnionsDistinctModels(t *testing.T) {
	t.Parallel()

	merged := mergeCatalogProviderModels(
		catalogProvider{
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]CatalogModel{
				"gpt-5.4": {ID: "gpt-5.4", Name: "GPT-5.4"},
			},
		},
		catalogProvider{
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]CatalogModel{
				"gpt-5.5":      {ID: "gpt-5.5", Name: "GPT-5.5"},
				"gpt-5.4-mini": {ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini"},
			},
		},
	)

	if len(merged.Models) != 3 {
		t.Fatalf("merged models = %#v", merged.Models)
	}
	if _, ok := merged.Models["gpt-5.4"]; !ok {
		t.Fatalf("merged models missing gpt-5.4: %#v", merged.Models)
	}
	if _, ok := merged.Models["gpt-5.5"]; !ok {
		t.Fatalf("merged models missing gpt-5.5: %#v", merged.Models)
	}
}

func TestParseOpenAICodexModelsResponse(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"models": [
			{
				"slug": "gpt-5.5",
				"display_name": "GPT-5.5",
				"supported_in_api": true,
				"visibility": "list",
				"supported_reasoning_levels": [{"effort":"low"},{"effort":"xhigh"}]
			},
			{
				"slug": "hidden-model",
				"display_name": "Hidden",
				"supported_in_api": true,
				"visibility": "hidden"
			},
			{
				"slug": "unsupported",
				"display_name": "Unsupported",
				"supported_in_api": false,
				"visibility": "list"
			}
		]
	}`)

	models := parseOpenAICodexModelsResponse(body)
	if len(models) != 1 {
		t.Fatalf("models = %#v, want 1 visible supported model", models)
	}
	if models[0].ID != "gpt-5.5" {
		t.Fatalf("models[0] = %#v", models[0])
	}
	if !models[0].Reasoning {
		t.Fatalf("models[0] reasoning = false, want true")
	}
}

func TestParseOpenAIModelsResponseSupportsObjectEnvelope(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"data": [
			{
				"id": "gpt-5.4",
				"object": "model",
				"max_context_length": 262144,
				"capabilities": ["tool_use"]
			}
		]
	}`)

	models, ok := parseOpenAIModelsResponse(body)
	if !ok {
		t.Fatal("ok = false, want parsed object envelope")
	}
	if len(models) != 1 {
		t.Fatalf("models = %#v, want 1", models)
	}
	if models[0].ID != "gpt-5.4" || models[0].ContextSize != 262144 {
		t.Fatalf("models[0] = %#v", models[0])
	}
	if !models[0].ToolCalls || !models[0].ToolCallsKnown {
		t.Fatalf("models[0] tool calls = %#v", models[0])
	}
}

func TestParseOpenAIModelsResponseSupportsTogetherArray(t *testing.T) {
	t.Parallel()

	body := []byte(`[
		{
			"id": "moonshotai/Kimi-K2-Instruct",
			"type": "chat",
			"display_name": "Kimi K2 Instruct",
			"context_length": 131072,
			"pricing": {
				"input": 0.6,
				"output": 2.5
			}
		},
		{
			"id": "BAAI/bge-large-en-v1.5",
			"type": "embedding",
			"display_name": "BGE Large"
		}
	]`)

	models, ok := parseOpenAIModelsResponse(body)
	if !ok {
		t.Fatal("ok = false, want parsed array response")
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v, want 2", models)
	}
	if models[0].ID != "moonshotai/Kimi-K2-Instruct" {
		t.Fatalf("models[0] = %#v", models[0])
	}
	if models[0].Name != "Kimi K2 Instruct" || models[0].ContextSize != 131072 {
		t.Fatalf("models[0] metadata = %#v", models[0])
	}
	if models[0].CostInput != 0.6 || models[0].CostOutput != 2.5 {
		t.Fatalf("models[0] pricing = %#v", models[0])
	}
	if len(models[0].OutputModalities) != 1 || models[0].OutputModalities[0] != "text" {
		t.Fatalf("models[0] modalities = %#v", models[0].OutputModalities)
	}
	if len(models[1].OutputModalities) != 1 || models[1].OutputModalities[0] != "embedding" {
		t.Fatalf("models[1] modalities = %#v", models[1].OutputModalities)
	}
}

func TestFetchOpenAIOAuthModelCatalogProviderReturnsLiveErrorWithoutFallback(t *testing.T) {
	t.Setenv("KODACODE_OPENAI_CLIENT_VERSION", "")

	store := NewAuthStoreAt(filepath.Join(t.TempDir(), "auth.yaml"))
	entry := AuthEntry{
		Type:          AuthTypeOAuth,
		Access:        "oauth-access",
		Refresh:       "oauth-refresh",
		Expires:       time.Now().Add(time.Hour).UnixMilli(),
		ClientVersion: "1.2.3",
	}
	if err := store.Set("openai", entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "chatgpt.com" {
			t.Fatalf("host = %q, want chatgpt.com", req.URL.Host)
		}
		if req.URL.Path != "/backend-api/codex/models" {
			t.Fatalf("path = %q, want /backend-api/codex/models", req.URL.Path)
		}
		if got := req.URL.Query().Get("client_version"); got != "1.2.3" {
			t.Fatalf("client_version = %q, want 1.2.3", got)
		}
		return httpJSONResponse(http.StatusInternalServerError, `{"error":{"message":"codex unavailable"}}`), nil
	})}

	provider, err := fetchOpenAIOAuthModelCatalogProvider(context.Background(), RemoteModelCatalogProvider{
		ID:   "openai",
		Name: "OpenAI",
		Kind: RemoteModelCatalogProviderOpenAI,
		OAuth: &OpenAIOAuthConfig{
			Entry: entry,
			Store: store,
		},
		HTTPClient: httpClient,
	})
	if provider != nil {
		t.Fatalf("provider = %#v, want nil on oauth discovery failure", provider)
	}
	if err == nil {
		t.Fatal("error = nil, want live oauth discovery failure")
	}
}

func TestFetchOpenAIModelCatalogProviderReturnsPartialModelsAndErrorWhenAPIKeyDiscoveryFails(t *testing.T) {
	t.Setenv("KODACODE_OPENAI_CLIENT_VERSION", "")

	store := NewAuthStoreAt(filepath.Join(t.TempDir(), "auth.yaml"))
	entry := AuthEntry{
		Type:          AuthTypeOAuth,
		Access:        "oauth-access",
		Refresh:       "oauth-refresh",
		Expires:       time.Now().Add(time.Hour).UnixMilli(),
		ClientVersion: "1.2.3",
	}
	if err := store.Set("openai", entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.openai.com" && req.URL.Path == "/v1/models":
			return httpJSONResponse(http.StatusInternalServerError, `{"error":{"message":"platform unavailable"}}`), nil
		case req.URL.Host == "chatgpt.com" && req.URL.Path == "/backend-api/codex/models":
			if got := req.URL.Query().Get("client_version"); got != "1.2.3" {
				t.Fatalf("client_version = %q, want 1.2.3", got)
			}
			return httpJSONResponse(http.StatusOK, `{"models":[{"slug":"gpt-5.5","display_name":"GPT-5.5","supported_in_api":true,"visibility":"list"}]}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})}

	provider, err := fetchOpenAIModelCatalogProvider(context.Background(), RemoteModelCatalogProvider{
		ID:     "openai",
		Name:   "OpenAI",
		Kind:   RemoteModelCatalogProviderOpenAI,
		APIKey: "sk-test",
		OAuth: &OpenAIOAuthConfig{
			Entry: entry,
			Store: store,
		},
		HTTPClient: httpClient,
	})
	if provider == nil {
		t.Fatal("provider = nil, want partial oauth model catalog")
	}
	if err == nil {
		t.Fatal("error = nil, want surfaced api-key discovery failure")
	}
	models := provider.Models
	if len(models) != 1 || models["gpt-5.5"].ID != "gpt-5.5" {
		t.Fatalf("provider models = %#v", models)
	}
}

func TestFetchOpenAIModelCatalogProviderReturnsAPIKeyModelsAndErrorWhenOAuthDiscoveryFails(t *testing.T) {
	t.Setenv("KODACODE_OPENAI_CLIENT_VERSION", "")

	store := NewAuthStoreAt(filepath.Join(t.TempDir(), "auth.yaml"))
	entry := AuthEntry{
		Type:          AuthTypeOAuth,
		Access:        "oauth-access",
		Refresh:       "oauth-refresh",
		Expires:       time.Now().Add(time.Hour).UnixMilli(),
		ClientVersion: "1.2.3",
	}
	if err := store.Set("openai", entry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.openai.com" && req.URL.Path == "/v1/models":
			return httpJSONResponse(http.StatusOK, `{"data":[{"id":"gpt-5.4","object":"model"}]}`), nil
		case req.URL.Host == "chatgpt.com" && req.URL.Path == "/backend-api/codex/models":
			if got := req.URL.Query().Get("client_version"); got != "1.2.3" {
				t.Fatalf("client_version = %q, want 1.2.3", got)
			}
			return httpJSONResponse(http.StatusInternalServerError, `{"error":{"message":"codex unavailable"}}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})}

	provider, err := fetchOpenAIModelCatalogProvider(context.Background(), RemoteModelCatalogProvider{
		ID:         "openai",
		Name:       "OpenAI",
		Kind:       RemoteModelCatalogProviderOpenAI,
		APIKey:     "sk-test",
		OAuth:      &OpenAIOAuthConfig{Entry: entry, Store: store},
		HTTPClient: httpClient,
	})
	if provider == nil {
		t.Fatal("provider = nil, want api-key model catalog")
	}
	if err == nil {
		t.Fatal("error = nil, want surfaced oauth discovery failure")
	}
	if model := provider.Models["gpt-5.4"]; model.ID != "gpt-5.4" {
		t.Fatalf("provider models = %#v", provider.Models)
	}
	if !strings.Contains(err.Error(), "codex unavailable") {
		t.Fatalf("error = %v, want oauth discovery failure", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func httpJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
