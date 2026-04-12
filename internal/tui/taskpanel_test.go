package tui

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestCountCriteria_InlineNumberedAcceptanceCriteria(t *testing.T) {
	notes := "Implement standardized tracing. Acceptance criteria: 1. A trace ID is attached to each request. 2. Middleware propagates the trace ID."
	if got := countCriteria(notes); got != 2 {
		t.Fatalf("countCriteria() = %d, want 2", got)
	}
}

func TestCountCriteria_MultilineAcceptanceCriteria(t *testing.T) {
	notes := "Document rate limiting improvements.\n\nAcceptance criteria\n- Redis integration is described.\n- Logging changes are documented.\n- Testing approach is listed."
	if got := countCriteria(notes); got != 3 {
		t.Fatalf("countCriteria() = %d, want 3", got)
	}
}

func TestTaskPanelExpandedShowsReviewConcernDetails(t *testing.T) {
	panel := NewTaskPanel()
	panel.SetSize(120)
	panel.Toggle()
	panel.SetTasks([]*tool.Task{{
		ID:                "task 6",
		Title:             "Add role-based graduated rate limits",
		Status:            "in_progress",
		Notes:             "Acceptance criteria:\n- role overrides are strongly typed",
		ReviewStatus:      tool.TaskReviewConcern,
		LastReviewSummary: "Expected Partial<Record<UserRole, number>>; missing production-path override test.",
	}})

	view := panel.View()
	if !strings.Contains(view, "review concern") {
		t.Fatalf("expected review concern badge, got:\n%s", view)
	}
	if !strings.Contains(view, "Expected Partial<Record<UserRole, number>>") {
		t.Fatalf("expected review summary detail, got:\n%s", view)
	}
}

func TestTaskPanelExpandedShowsBlockedReason(t *testing.T) {
	panel := NewTaskPanel()
	panel.SetSize(120)
	panel.Toggle()
	panel.SetTasks([]*tool.Task{{
		ID:                "task 5",
		Title:             "Add integration tests",
		Status:            "blocked",
		BlockReason:       tool.TaskBlockReasonExecutionStall,
		LastReviewSummary: "Tests pass, but execution stalled before the task state changed.",
	}})

	view := panel.View()
	if !strings.Contains(view, "blocked: execution stalled") {
		t.Fatalf("expected blocked reason, got:\n%s", view)
	}
	if !strings.Contains(view, "Tests pass, but execution stalled") {
		t.Fatalf("expected blocked detail summary, got:\n%s", view)
	}
}
