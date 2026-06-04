package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestLoadRuntimeConfigWithSourcesAppliesStoredWorkflowReviewSettings(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Model: StoredModelConfig{
				Primary: "openai/gpt-5",
			},
			Workflow: StoredWorkflowConfig{
				ReviewMode:      "auto",
				PlannerApproval: boolPtr(true),
				ReviewModel: StoredModelConfig{
					Primary: "openai/gpt-5-mini",
				},
			},
		}},
		fakeAuthLookup{entries: map[string]provider.AuthEntry{
			"openai": {Type: provider.AuthTypeAPI, Access: "test-key"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}

	if config.Workflow.ReviewMode != WorkflowReviewAuto {
		t.Fatalf("review mode = %q", config.Workflow.ReviewMode)
	}
	if got := config.Workflow.ReviewModelRoute.Primary.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("review model = %q", got)
	}
	if !config.Workflow.PlannerApproval {
		t.Fatal("planner approval = false, want true")
	}
}
