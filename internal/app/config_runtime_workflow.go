package app

import "strings"

func applyStoredWorkflowConfig(config *WorkflowConfig, stored StoredWorkflowConfig) {
	if config == nil {
		return
	}
	if reviewMode := strings.TrimSpace(stored.ReviewMode); reviewMode != "" {
		config.ReviewMode = WorkflowReviewMode(reviewMode)
	}
	if route, ok := storedModelRoute(stored.ReviewModel); ok {
		config.ReviewModelRoute = route
	}
	if stored.PlannerApproval != nil {
		config.PlannerApproval = *stored.PlannerApproval
	}
}
