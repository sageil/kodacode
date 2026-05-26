package app

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestConfigValidateRejectsNegativeOutputBudget(t *testing.T) {
	config := Config{
		OutputBudgets: OutputBudgetsConfig{AgentTurn: -1},
	}

	err := config.Validate()
	if err == nil || !strings.Contains(err.Error(), "output_budgets.agent_turn") {
		t.Fatalf("Validate() error = %v, want output budget validation error", err)
	}
}

func TestConfigValidateRejectsNonPositiveModelDefaultOutputTokens(t *testing.T) {
	config := Config{
		ModelOverrides: []ModelOverrideConfig{{
			Ref:                 provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			DefaultOutputTokens: intPtr(0),
		}},
	}

	err := config.Validate()
	if err == nil || !strings.Contains(err.Error(), "default_output_tokens") {
		t.Fatalf("Validate() error = %v, want default output tokens validation error", err)
	}
}
