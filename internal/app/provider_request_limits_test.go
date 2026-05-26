package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestRequestMaxOutputTokensUsesAgentBudgetInsteadOfProviderCeiling(t *testing.T) {
	models := &fakeModelCatalog{modelsByID: map[string][]provider.CatalogModel{
		"anthropic": {
			{ID: "claude-sonnet-4-6", MaxOutputTokens: 64000},
		},
	}}
	ref := provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"}

	got := requestMaxOutputTokensForModel(models, nil, OutputBudgetsConfig{}, ref, outputBudgetAgentTurn, false)
	if got != defaultOutputBudgetAgentTurn {
		t.Fatalf("request max output tokens = %d, want %d", got, defaultOutputBudgetAgentTurn)
	}
}

func TestRequestMaxOutputTokensUsesThinkingBudget(t *testing.T) {
	models := &fakeModelCatalog{modelsByID: map[string][]provider.CatalogModel{
		"openai": {
			{ID: "gpt-5", MaxOutputTokens: 32000},
		},
	}}
	ref := provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"}

	got := requestMaxOutputTokensForModel(models, nil, OutputBudgetsConfig{}, ref, outputBudgetAgentTurn, true)
	if got != defaultOutputBudgetAgentTurnThinking {
		t.Fatalf("request max output tokens = %d, want %d", got, defaultOutputBudgetAgentTurnThinking)
	}
}

func TestRequestMaxOutputTokensClampsBudgetToProviderCeiling(t *testing.T) {
	models := &fakeModelCatalog{modelsByID: map[string][]provider.CatalogModel{
		"local": {
			{ID: "small", MaxOutputTokens: 1024},
		},
	}}
	ref := provider.ModelRef{ProviderID: "local", ModelID: "small"}

	got := requestMaxOutputTokensForModel(models, nil, OutputBudgetsConfig{}, ref, outputBudgetAgentTurn, false)
	if got != 1024 {
		t.Fatalf("request max output tokens = %d, want 1024", got)
	}
}

func TestRequestMaxOutputTokensUsesPerModelDefaultOverride(t *testing.T) {
	models := &fakeModelCatalog{modelsByID: map[string][]provider.CatalogModel{
		"anthropic": {
			{ID: "claude-sonnet-4-6", MaxOutputTokens: 64000},
		},
	}}
	ref := provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"}
	overrides := []ModelOverrideConfig{{
		Ref:                 ref,
		DefaultOutputTokens: intPtr(12000),
	}}

	got := requestMaxOutputTokensForModel(models, overrides, OutputBudgetsConfig{}, ref, outputBudgetAgentTurn, false)
	if got != 12000 {
		t.Fatalf("request max output tokens = %d, want 12000", got)
	}
}

func TestModelMaxOutputTokenCeilingUsesProviderLimit(t *testing.T) {
	models := &fakeModelCatalog{modelsByID: map[string][]provider.CatalogModel{
		"anthropic": {
			{ID: "claude-sonnet-4-6", MaxOutputTokens: 64000},
		},
	}}
	ref := provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"}

	got := modelMaxOutputTokenCeilingForModel(models, ref)
	if got != 64000 {
		t.Fatalf("model max output token ceiling = %d, want 64000", got)
	}
}
