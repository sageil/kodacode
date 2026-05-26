package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestOrderedUtilityTextCandidatesPrefersUtilityModelThenPrimary(t *testing.T) {
	candidates := orderedUtilityTextCandidates(
		provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
	)

	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	if got := candidates[0].Ref.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("candidate[0] = %q, want explicit utility model", got)
	}
	if got := candidates[1].Ref.String(); got != "openai/gpt-5" {
		t.Fatalf("candidate[1] = %q, want primary fallback", got)
	}
}

func TestOrderedUtilityTextCandidatesUsesActiveModelWhenUtilityModelUnset(t *testing.T) {
	candidates := orderedUtilityTextCandidates(
		provider.ModelRef{},
		provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
	)

	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	if got := candidates[0].Ref.String(); got != "openai/gpt-5" {
		t.Fatalf("candidate = %q, want active model", got)
	}
}

func TestOrderedUtilityTextCandidatesDeduplicatesPrimaryAndUtilityModel(t *testing.T) {
	candidates := orderedUtilityTextCandidates(
		provider.ModelRef{ProviderID: "OpenAI", ModelID: "gpt-5"},
		provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
	)

	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	if got := candidates[0].Ref.String(); got != "OpenAI/gpt-5" {
		t.Fatalf("candidate = %q, want the configured utility model preserved", got)
	}
}

func TestAvailableUtilityTextCandidatesSkipsUnavailableConfiguredProviderAndFallsBackToPrimary(t *testing.T) {
	candidates := availableUtilityTextCandidates(
		provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
		provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		func(providerID string) bool {
			return providerID == "openai"
		},
	)

	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	if got := candidates[0].Ref.String(); got != "openai/gpt-5" {
		t.Fatalf("candidate = %q, want primary fallback", got)
	}
}

func TestAvailableUtilityTextCandidatesSkipsUnavailablePrimaryProvider(t *testing.T) {
	candidates := availableUtilityTextCandidates(
		provider.ModelRef{},
		provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		func(string) bool { return false },
	)

	if len(candidates) != 0 {
		t.Fatalf("candidate count = %d, want 0", len(candidates))
	}
}
