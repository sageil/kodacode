package service

import (
	"context"
	"slices"
	"testing"

	"github.com/sageil/kodacode/v1/internal/config"
)

func TestSubagentAgentConfig_StructuredPlannerRemovesTask(t *testing.T) {
	cfg := config.AgentConfig{
		Tools:     []string{"read", "task", "read_files"},
		DenyTools: []string{"bash"},
	}

	got := subagentAgentConfig(withStructuredPlanner(context.Background()), "planner", cfg)

	if slices.Contains(got.Tools, "task") {
		t.Fatalf("planner tools still contain task: %v", got.Tools)
	}
	if !slices.Contains(got.DenyTools, "task") {
		t.Fatalf("planner deny_tools missing task: %v", got.DenyTools)
	}
	if !slices.Contains(got.DenyTools, "bash") {
		t.Fatalf("planner deny_tools lost existing entries: %v", got.DenyTools)
	}
}

func TestShouldForwardTaskSession_StructuredPlannerDisablesForwarding(t *testing.T) {
	ctx := withStructuredPlanner(context.Background())

	if shouldForwardTaskSession(ctx, "planner") {
		t.Fatal("structured planner should not receive parent task session")
	}
	if !shouldForwardTaskSession(ctx, "explorer") {
		t.Fatal("non-planner subagents should still receive parent task session")
	}
	if !shouldForwardTaskSession(context.Background(), "planner") {
		t.Fatal("planner without structured mode should still receive parent task session")
	}
}
