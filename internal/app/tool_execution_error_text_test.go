package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutionErrorTextTaskWorkflowCreateCompletedReturnsCorrection(t *testing.T) {
	err := tool.InvalidArguments(tool.TaskWorkflowToolName, tool.ErrTaskCompleteActionOnly)
	got := toolExecutionErrorText(tool.TaskWorkflowToolName, err)

	for _, want := range []string{
		`task_workflow:`,
		`create cannot use status "complete"`,
		`complete with task_id and summary`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("toolExecutionErrorText() text = %q, missing %q", got, want)
		}
	}
}

func TestToolExecutionErrorTextTaskWorkflowMissingSummaryReturnsCorrection(t *testing.T) {
	err := tool.InvalidArguments(tool.TaskWorkflowToolName, tool.ErrTaskSummaryRequired)
	got := toolExecutionErrorText(tool.TaskWorkflowToolName, err)

	for _, want := range []string{
		`task_workflow:`,
		`complete needs task_id and summary`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("toolExecutionErrorText() text = %q, missing %q", got, want)
		}
	}
}

func TestToolExecutionErrorTextTaskWorkflowInvalidActionRoutesReviewIntent(t *testing.T) {
	err := tool.InvalidArguments(tool.TaskWorkflowToolName, tool.ErrTaskWorkflowActionInvalid)
	got := toolExecutionErrorText(tool.TaskWorkflowToolName, err)

	for _, want := range []string{
		`task_workflow:`,
		`action must be list, create, update, block, or complete`,
		`Use task_review for reviews`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("toolExecutionErrorText() text = %q, missing %q", got, want)
		}
	}
}

func TestToolExecutionErrorTextTaskWorkflowUnsupportedFieldsShowsActionPayload(t *testing.T) {
	err := tool.InvalidArguments(tool.TaskWorkflowToolName, fmt.Errorf("%w: action=create fields=summary, progress", tool.ErrTaskWorkflowFieldUnsupported))
	got := toolExecutionErrorText(tool.TaskWorkflowToolName, err)

	for _, want := range []string{
		`task_workflow:`,
		`create cannot use summary, progress`,
		`Use create for title/parent/kind/status/notes`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("toolExecutionErrorText() text = %q, missing %q", got, want)
		}
	}
}

func TestToolExecutionErrorTextTaskReviewUnsupportedFieldsShowsActionPayload(t *testing.T) {
	err := tool.InvalidArguments(tool.TaskReviewToolName, fmt.Errorf("%w: action=review fields=title, status", tool.ErrTaskReviewFieldUnsupported))
	got := toolExecutionErrorText(tool.TaskReviewToolName, err)

	for _, want := range []string{
		`task_review failed`,
		`review accepts only action, task_id, review_status, and review_summary`,
		`Remove title, status`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("toolExecutionErrorText() text = %q, missing %q", got, want)
		}
	}
}

func TestToolExecutionErrorTextTaskReviewMissingTaskIDReturnsListRecovery(t *testing.T) {
	err := tool.InvalidArguments(tool.TaskReviewToolName, tool.ErrTaskInputIDRequired)
	got := toolExecutionErrorText(tool.TaskReviewToolName, err)

	for _, want := range []string{
		`task_review failed`,
		`review requires task_id`,
		`returned task_id`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("toolExecutionErrorText() text = %q, missing %q", got, want)
		}
	}
}

func TestToolExecutionErrorTextTaskReviewNotFoundReturnsListRecovery(t *testing.T) {
	got := toolExecutionErrorText(tool.TaskReviewToolName, fmt.Errorf("%w: task-33", events.ErrTaskNotFound))

	for _, want := range []string{
		`task_review failed`,
		`task_id task-33 was not found`,
		`returned task_id`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("toolExecutionErrorText() text = %q, missing %q", got, want)
		}
	}
}

func TestToolExecutionErrorTextCommonRecoveriesStayConcise(t *testing.T) {
	cases := []struct {
		name string
		tool string
		err  error
	}{
		{name: "task create complete", tool: tool.TaskWorkflowToolName, err: tool.InvalidArguments(tool.TaskWorkflowToolName, tool.ErrTaskCompleteActionOnly)},
		{name: "task unsupported", tool: tool.TaskWorkflowToolName, err: tool.InvalidArguments(tool.TaskWorkflowToolName, fmt.Errorf("%w: action=create fields=summary, progress", tool.ErrTaskWorkflowFieldUnsupported))},
		{name: "review unsupported", tool: tool.TaskReviewToolName, err: tool.InvalidArguments(tool.TaskReviewToolName, fmt.Errorf("%w: action=review fields=title, status", tool.ErrTaskReviewFieldUnsupported))},
		{name: "planner invalid", tool: tool.QuestionToolName, err: ErrPlannerSavePlanQuestionInvalid},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := toolExecutionErrorText(tt.tool, tt.err)
			if len(got) > 180 {
				t.Fatalf("toolExecutionErrorText() length = %d, want <= 180: %q", len(got), got)
			}
			if strings.Contains(got, `{"`) {
				t.Fatalf("toolExecutionErrorText() embeds JSON example: %q", got)
			}
		})
	}
}

func TestToolExecutionErrorTextTaskWorkflowParentNotFoundReturnsCleanRecovery(t *testing.T) {
	got := toolExecutionErrorText(tool.TaskWorkflowToolName, ErrTaskParentNotFound)

	for _, want := range []string{
		`task_workflow: parent task not found`,
		`Use list`,
		`existing parent_task_id`,
		`omit parent_task_id`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("toolExecutionErrorText() text = %q, missing %q", got, want)
		}
	}
}

func TestToolExecutionErrorTextTaskWorkflowUpdateUnsupportedFieldsReturnsCleanRecovery(t *testing.T) {
	err := tool.InvalidArguments(tool.TaskWorkflowToolName, fmt.Errorf("%w: action=update fields=parent_task_id, title", tool.ErrTaskWorkflowFieldUnsupported))
	got := toolExecutionErrorText(tool.TaskWorkflowToolName, err)

	for _, want := range []string{
		`task_workflow: update cannot use parent_task_id, title`,
		`Use update for task_id/status/progress/notes`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("toolExecutionErrorText() text = %q, missing %q", got, want)
		}
	}
}
