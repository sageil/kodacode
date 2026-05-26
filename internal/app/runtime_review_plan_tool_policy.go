package app

import (
	"slices"
	"strings"

	toolpkg "github.com/sageil/kodacode/internal/tool"
)

func reviewPlanHarnessParentTools(agentID, userText string, tools []string) []string {
	if !isReviewPlanHarnessParentTurn(agentID, userText) {
		return tools
	}
	out := slices.Clone(tools)
	return slices.DeleteFunc(out, func(name string) bool {
		return strings.TrimSpace(name) == toolpkg.TaskWorkflowToolName
	})
}

func isReviewPlanHarnessParentTurn(agentID, userText string) bool {
	if strings.TrimSpace(agentID) != "engineer" {
		return false
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(userText), " "))
	if normalized == "" {
		return false
	}
	return containsReviewHarnessIntent(normalized) && containsPlanHarnessIntent(normalized)
}

func containsReviewHarnessIntent(text string) bool {
	for _, marker := range []string{
		"review",
		"audit",
		"regression check",
		"issue hunt",
		"bottleneck",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func containsPlanHarnessIntent(text string) bool {
	for _, marker := range []string{
		" plan",
		"plan ",
		"action plan",
		"execution plan",
		"implementation sequence",
		"what needs to change",
		"roadmap",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return strings.HasSuffix(text, "plan")
}
