package app

import (
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func toolExecutionErrorText(toolName string, cause error) string {
	if errors.Is(cause, ErrPlannerSavePlanQuestionRequiresVisiblePlan) {
		return plannerSavePlanQuestionRequiresVisiblePlanText
	}
	if errors.Is(cause, ErrPlannerSavePlanQuestionInvalid) {
		return plannerSavePlanQuestionInvalidText
	}
	if errors.Is(cause, ErrPlannerPlanApprovalDisabledByWorkflow) {
		return plannerPlanApprovalDisabledByWorkflowText
	}
	if errors.Is(cause, ErrPlannerPlanApprovalDisabled) {
		return plannerPlanApprovalDisabledText
	}
	name := strings.TrimSpace(toolName)
	if name == tool.TaskWorkflowToolName {
		if text, ok := taskWorkflowActionText(cause); ok {
			return text
		}
	}
	if name == tool.TaskReviewToolName {
		if text, ok := taskReviewActionText(cause); ok {
			return text
		}
	}
	defaultText := tool.DefaultErrorText(name, cause)
	return defaultText
}

func taskWorkflowActionText(err error) (string, bool) {
	switch {
	case errors.Is(err, tool.ErrTaskCompleteActionOnly):
		text := `task_workflow: create cannot use status "complete". Create it first, then complete with task_id and summary.`
		return text, true
	case errors.Is(err, tool.ErrTaskStatusInvalid):
		text := `task_workflow: status must be pending or in_progress. Use complete to finish or block to block.`
		return text, true
	case errors.Is(err, tool.ErrTaskInputTitleRequired):
		text := `task_workflow: create needs title.`
		return text, true
	case errors.Is(err, tool.ErrTaskInputIDRequired):
		text := `task_workflow: this action needs task_id.`
		return text, true
	case errors.Is(err, tool.ErrTaskInputUpdateRequired):
		text := `task_workflow: update needs status, progress, or notes.`
		return text, true
	case errors.Is(err, tool.ErrTaskBlockReasonRequired):
		text := `task_workflow: block needs block_reason.`
		return text, true
	case errors.Is(err, tool.ErrTaskSummaryRequired):
		text := `task_workflow: complete needs task_id and summary.`
		return text, true
	case errors.Is(err, tool.ErrTaskWorkflowActionInvalid):
		text := `task_workflow: action must be list, create, update, block, or complete. Use task_review for reviews.`
		return text, true
	case errors.Is(err, tool.ErrTaskWorkflowFieldUnsupported):
		if fields := taskErrorSuffix(err, tool.ErrTaskWorkflowFieldUnsupported); fields != "" {
			return taskWorkflowUnsupportedFieldsText(fields), true
		}
		return "task_workflow: remove fields this action does not use.", true
	case errors.Is(err, events.ErrTaskNotFound):
		if taskID := taskErrorSuffix(err, events.ErrTaskNotFound); taskID != "" {
			return "task_workflow: task " + taskID + " not found. Use list, then retry with an existing task_id.", true
		}
		return "task_workflow: task not found. Use list, then retry with an existing task_id.", true
	case errors.Is(err, ErrTaskChildTasksIncomplete):
		return "task_workflow: complete child tasks before completing the parent.", true
	case errors.Is(err, ErrTaskCompletionSummaryRequired):
		return "task_workflow: complete needs summary.", true
	case errors.Is(err, ErrTaskParentCompleted):
		return "task_workflow: parent task is complete. Use a pending parent_task_id or omit parent_task_id.", true
	case errors.Is(err, ErrTaskParentNotFound):
		return "task_workflow: parent task not found. Use list, then create with an existing parent_task_id or omit parent_task_id.", true
	case errors.Is(err, ErrTaskParentSelfReference):
		return "task_workflow: parent_task_id cannot equal task_id.", true
	case errors.Is(err, ErrTaskAnotherInProgress):
		if activeTaskID := taskWorkflowReferencedTaskID(err, ErrTaskAnotherInProgress); activeTaskID != "" {
			text := "task_workflow: " + activeTaskID + " is already in progress. Update, block, or complete it first."
			return text, true
		}
		return "task_workflow: another task is in progress. Update, block, or complete it first.", true
	default:
		return "", false
	}
}

func taskReviewActionText(err error) (string, bool) {
	switch {
	case errors.Is(err, tool.ErrTaskActionRequired):
		return `task_review failed: action must be "list" or "review".`, true
	case errors.Is(err, tool.ErrTaskInputIDRequired):
		return `task_review failed: review requires task_id. List review tasks and retry with a returned task_id.`, true
	case errors.Is(err, tool.ErrTaskReviewStatusInvalid):
		return `task_review failed: review_status must be pass, concern, fail, or accepted.`, true
	case errors.Is(err, tool.ErrTaskReviewSummaryNeeded):
		return `task_review failed: review requires review_summary.`, true
	case errors.Is(err, tool.ErrTaskReviewActionInvalid):
		return `task_review failed: action must be list or review. Use task_workflow for workflow updates.`, true
	case errors.Is(err, tool.ErrTaskReviewFieldUnsupported):
		if fields := taskErrorSuffix(err, tool.ErrTaskReviewFieldUnsupported); fields != "" {
			return taskReviewUnsupportedFieldsText(fields), true
		}
		return "task_review failed: remove fields not supported by this action.", true
	case errors.Is(err, events.ErrTaskNotFound):
		if taskID := taskErrorSuffix(err, events.ErrTaskNotFound); taskID != "" {
			return "task_review failed: task_id " + taskID + " was not found. List review tasks and retry with a returned task_id.", true
		}
		return "task_review failed: task was not found. List review tasks and retry with a returned task_id.", true
	default:
		return "", false
	}
}

func taskWorkflowReferencedTaskID(err, base error) string {
	return taskErrorSuffix(err, base)
}

func taskWorkflowUnsupportedFieldsText(detail string) string {
	action, fields := parseTaskUnsupportedFieldDetail(detail)
	fieldsText := fields
	if fieldsText == "" {
		fieldsText = strings.TrimSpace(detail)
	}
	switch action {
	case "list":
		return `task_workflow: list only uses action. Remove ` + fieldsText + `.`
	case "create":
		return `task_workflow: create cannot use ` + fieldsText + `. Use create for title/parent/kind/status/notes.`
	case "update":
		return `task_workflow: update cannot use ` + fieldsText + `. Use update for task_id/status/progress/notes.`
	case "block":
		return `task_workflow: block cannot use ` + fieldsText + `. Use block with task_id and block_reason.`
	case "complete":
		return `task_workflow: complete cannot use ` + fieldsText + `. Use complete with task_id and summary.`
	default:
		return "task_workflow: remove fields this action does not use: " + fieldsText + "."
	}
}

func taskReviewUnsupportedFieldsText(detail string) string {
	action, fields := parseTaskUnsupportedFieldDetail(detail)
	fieldsText := fields
	if fieldsText == "" {
		fieldsText = strings.TrimSpace(detail)
	}
	switch action {
	case "list":
		return `task_review failed: list accepts only action. Remove ` + fieldsText + `.`
	case "review":
		return `task_review failed: review accepts only action, task_id, review_status, and review_summary. Remove ` + fieldsText + `.`
	default:
		return "task_review failed: remove unsupported field(s): " + fieldsText + "."
	}
}

func parseTaskUnsupportedFieldDetail(detail string) (action string, fields string) {
	detail = strings.TrimSpace(detail)
	const actionPrefix = "action="
	if !strings.HasPrefix(detail, actionPrefix) {
		return "", detail
	}
	rest := strings.TrimPrefix(detail, actionPrefix)
	action, fields, ok := strings.Cut(rest, " fields=")
	if !ok {
		return strings.TrimSpace(rest), ""
	}
	return strings.TrimSpace(action), strings.TrimSpace(fields)
}

func taskErrorSuffix(err, base error) string {
	if err == nil || base == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	var invalid *tool.InvalidArgumentsError
	if errors.As(err, &invalid) && invalid.Cause != nil {
		message = strings.TrimSpace(invalid.Cause.Error())
	}
	prefix := strings.TrimSpace(base.Error())
	if message == "" || prefix == "" {
		return ""
	}
	suffix, ok := strings.CutPrefix(message, prefix+": ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(suffix)
}
