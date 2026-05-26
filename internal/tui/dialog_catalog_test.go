package tui

import (
	"testing"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
)

func TestBuildModelItemsPrefersRuntimeAvailableModelsForGitHubCopilot(t *testing.T) {
	items := buildModelItems(app.DialogState{
		ConnectedProviders: []app.ConnectedProvider{
			{ProviderID: "github-copilot"},
		},
		AvailableModels: []app.AvailableModel{
			{
				Ref:          provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.4"},
				ProviderName: "GitHub Copilot",
				ModelName:    "GPT-5.4",
				Capacity:     provider.NormalizeModelCapacity(200000, 100000, 0),
				CostInput:    1.25,
				CostOutput:   10,
				Reasoning:    true,
				ToolCalls:    true,
				Vision:       true,
			},
			{
				Ref:          provider.ModelRef{ProviderID: "github-copilot", ModelID: "o3-mini"},
				ProviderName: "GitHub Copilot",
				ModelName:    "o3-mini",
			},
		},
	}, provider.ModelRef{})

	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].Ref.String() != "github-copilot/gpt-5.4" {
		t.Fatalf("items[0] = %#v", items[0])
	}
	if !items[0].Reasoning || !items[0].ToolCalls || !items[0].Vision {
		t.Fatalf("items[0] capabilities = %#v", items[0])
	}
	if items[0].Capacity.WindowTokens != 200000 || items[0].Capacity.InputTokens != 100000 || items[0].CostInput != 1.25 || items[0].CostOutput != 10 {
		t.Fatalf("items[0] metadata = %#v", items[0])
	}
	if items[1].Ref.String() != "github-copilot/o3-mini" {
		t.Fatalf("items[1] = %#v", items[1])
	}
}

func TestBuildModelItemsDoesNotInventConnectedProviderModelsWithoutRuntimeCatalog(t *testing.T) {
	items := buildModelItems(app.DialogState{
		ConnectedProviders: []app.ConnectedProvider{
			{ProviderID: "nvidia"},
		},
	}, provider.ModelRef{})

	if len(items) != 0 {
		t.Fatalf("items = %#v, want no synthetic provider models", items)
	}
}

func TestBuildModelItemsAddsExactSelectedModelWhenMissingFromCatalog(t *testing.T) {
	items := buildModelItems(app.DialogState{
		ConnectedProviders: []app.ConnectedProvider{
			{ProviderID: "github-copilot"},
		},
		AvailableModels: []app.AvailableModel{{
			Ref:          provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.4"},
			ProviderName: "GitHub Copilot",
			ModelName:    "GPT-5.4",
		}},
	}, provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"})

	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].Ref.String() != "github-copilot/gpt-4.1" || !items[0].Exact {
		t.Fatalf("items[0] = %#v, want exact selected model row", items[0])
	}
}
