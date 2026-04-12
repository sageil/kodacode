package service

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestPlannerBlockedReason_SameBatchExplorer(t *testing.T) {
	got := plannerBlockedReason(nil, "engineer", false, []provider.ToolCall{
		{Name: "subagent", Arguments: `{"agent_id":"explorer","task":"inspect"}`},
		{Name: "subagent", Arguments: `{"agent_id":"planner","task":"plan"}`},
	})
	if !strings.Contains(got, "same response as explorer") {
		t.Fatalf("expected same-batch rejection, got %q", got)
	}
}
