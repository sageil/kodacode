package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestWrapModelCatalogOverridesAppliesOverrideAndInjectsModel(t *testing.T) {
	base := &fakeModelCatalog{
		providerNames: map[string]string{
			"openai": "OpenAI",
		},
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {
				{
					ID:          "gpt-5",
					Name:        "GPT-5",
					ContextSize: 128000,
					CostInput:   1.25,
					CostOutput:  10,
					ToolCalls:   true,
					Vision:      true,
					VisionKnown: true,
				},
			},
		},
	}
	catalog := wrapModelCatalogOverrides(base, []ModelOverrideConfig{
		{
			Ref:         provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			ContextSize: intPtr(256000),
			ToolCalls:   boolPtr(false),
			Vision:      boolPtr(false),
			CostInput:   floatPtr(0.75),
		},
		{
			Ref:         provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"},
			Name:        "GPT OSS 20B",
			ContextSize: intPtr(131072),
			Reasoning:   boolPtr(true),
			ToolCalls:   boolPtr(true),
		},
	})

	openAI := catalog.ModelsForProvider("openai")
	if len(openAI) != 1 {
		t.Fatalf("openai models = %#v", openAI)
	}
	if openAI[0].ContextSize != 256000 {
		t.Fatalf("openai context size = %d", openAI[0].ContextSize)
	}
	if openAI[0].ToolCalls || !openAI[0].ToolCallsKnown {
		t.Fatalf("openai tool calls = %#v", openAI[0])
	}
	if openAI[0].Vision || !openAI[0].VisionKnown {
		t.Fatalf("openai vision = %#v", openAI[0])
	}
	if openAI[0].CostInput != 0.75 || openAI[0].CostOutput != 10 {
		t.Fatalf("openai costs = %#v", openAI[0])
	}

	nvidia := catalog.ModelsForProvider("nvidia")
	if len(nvidia) != 1 {
		t.Fatalf("nvidia models = %#v", nvidia)
	}
	if nvidia[0].ID != "openai/gpt-oss-20b" || nvidia[0].Name != "GPT OSS 20B" {
		t.Fatalf("nvidia model = %#v", nvidia[0])
	}
	if nvidia[0].ContextSize != 131072 || !nvidia[0].Reasoning || !nvidia[0].ToolCalls {
		t.Fatalf("nvidia model capabilities = %#v", nvidia[0])
	}
}

func TestWrapModelCatalogOverridesDelegatesRefresh(t *testing.T) {
	base := &fakeModelCatalog{}
	catalog := wrapModelCatalogOverrides(base, []ModelOverrideConfig{{
		Ref:         provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		ContextSize: intPtr(128000),
	}})

	if err := catalog.EnsureFresh(context.Background()); err != nil {
		t.Fatalf("EnsureFresh() error = %v", err)
	}
	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if base.ensureCalls != 1 || base.refreshCalls != 1 {
		t.Fatalf("delegate calls = ensure:%d refresh:%d", base.ensureCalls, base.refreshCalls)
	}
}

func TestResolveModelInputBudgetForRouteUsesOverrideInjectedModel(t *testing.T) {
	catalog := wrapModelCatalogOverrides(&fakeModelCatalog{}, []ModelOverrideConfig{{
		Ref:            provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"},
		MaxInputTokens: intPtr(128000),
	}})

	budget, ok := resolveModelInputBudgetForRoute(provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"},
	}, catalog)
	if !ok {
		t.Fatal("ok = false, want override-injected model limit")
	}
	if budget.InputLimitTokens != 128000 {
		t.Fatalf("limit = %d, want 128000", budget.InputLimitTokens)
	}
	if budget.Source != currentTurnInputLimitSourceCatalog {
		t.Fatalf("source = %q, want %q", budget.Source, currentTurnInputLimitSourceCatalog)
	}
}
