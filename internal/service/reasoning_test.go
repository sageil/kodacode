package service

import (
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestResolveChatReasoningPrecedence(t *testing.T) {
	modelBudget := 4000
	agentBudget := 9000
	req := &pipeline.TurnRequest{
		Model: provider.Model{
			ID:             "m",
			ThinkingBudget: &modelBudget,
			Reasoning:      true,
		},
		Agent: config.AgentConfig{
			Reasoning: config.ReasoningConfig{
				Budget: &agentBudget,
				Effort: "medium",
			},
		},
		Variant: "low",
	}

	got := resolveChatReasoning(req)
	if got.ReasoningBudget == nil || *got.ReasoningBudget != Variants["low"].Budget {
		t.Fatalf("ReasoningBudget = %v, want %d", got.ReasoningBudget, Variants["low"].Budget)
	}
	if got.ReasoningEffort != Variants["low"].Effort {
		t.Fatalf("ReasoningEffort = %q, want %q", got.ReasoningEffort, Variants["low"].Effort)
	}
	if !got.ReasoningSupported {
		t.Fatal("ReasoningSupported = false, want true")
	}
}

func TestResolveChatReasoningFallsBackToAgentThenModel(t *testing.T) {
	modelBudget := 6000
	agentBudget := 7000

	req := &pipeline.TurnRequest{
		Model: provider.Model{ThinkingBudget: &modelBudget},
		Agent: config.AgentConfig{Reasoning: config.ReasoningConfig{Budget: &agentBudget}},
	}
	got := resolveChatReasoning(req)
	if got.ReasoningBudget == nil || *got.ReasoningBudget != agentBudget {
		t.Fatalf("ReasoningBudget = %v, want %d", got.ReasoningBudget, agentBudget)
	}

	req.Agent.Reasoning.Budget = nil
	got = resolveChatReasoning(req)
	if got.ReasoningBudget == nil || *got.ReasoningBudget != modelBudget {
		t.Fatalf("ReasoningBudget = %v, want %d", got.ReasoningBudget, modelBudget)
	}
}

func TestResolveChatReasoningReducesBudgetForLaterStepsAndHighContext(t *testing.T) {
	agentBudget := 12000
	req := &pipeline.TurnRequest{
		Step:         2,
		ContextUsage: 0.8,
		Agent: config.AgentConfig{
			Reasoning: config.ReasoningConfig{Budget: &agentBudget},
		},
	}

	got := resolveChatReasoning(req)
	if got.ReasoningBudget == nil {
		t.Fatal("ReasoningBudget = nil, want reduced budget")
	}
	if *got.ReasoningBudget != Variants["low"].Budget {
		t.Fatalf("ReasoningBudget = %d, want %d", *got.ReasoningBudget, Variants["low"].Budget)
	}
}
