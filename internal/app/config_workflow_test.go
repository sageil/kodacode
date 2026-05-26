package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestConfigValidateRejectsInvalidWorkflowReviewMode(t *testing.T) {
	config := Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: OpenAIProviderConfig{APIKey: "test-key"},
		Workflow: WorkflowConfig{
			ReviewMode: WorkflowReviewMode("sometimes"),
		},
	}

	if err := config.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid workflow review mode")
	}
}

func TestConfigValidateAllowsWorkflowReviewModelRoute(t *testing.T) {
	config := Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: OpenAIProviderConfig{APIKey: "test-key"},
		Workflow: WorkflowConfig{
			ReviewMode: WorkflowReviewAuto,
			ReviewModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
			},
		},
	}

	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
