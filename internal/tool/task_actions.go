package tool

import "errors"

const (
	taskActionList     = "list"
	taskActionCreate   = "create"
	taskActionUpdate   = "update"
	taskActionBlock    = "block"
	taskActionComplete = "complete"
	taskActionReview   = "review"
)

var (
	ErrTaskActionRequired           = errors.New("action is required")
	ErrTaskInputIDRequired          = errors.New("task_id is required")
	ErrTaskParentInputInvalid       = errors.New("parent_task_id must reference another task")
	ErrTaskInputTitleRequired       = errors.New("title is required")
	ErrTaskInputUpdateRequired      = errors.New("status, progress, or notes is required")
	ErrTaskStatusInvalid            = errors.New("status must be pending or in_progress when set")
	ErrTaskCompleteActionOnly       = errors.New("use action complete to mark a task done; status only accepts pending or in_progress")
	ErrTaskSummaryRequired          = errors.New("summary is required")
	ErrTaskBlockReasonRequired      = errors.New("block_reason is required")
	ErrTaskReviewStatusInvalid      = errors.New("review_status must be pass, concern, fail, or accepted")
	ErrTaskReviewSummaryNeeded      = errors.New("review_summary is required")
	ErrTaskWorkflowFieldUnsupported = errors.New("field is not supported for this task_workflow action")
	ErrTaskReviewFieldUnsupported   = errors.New("field is not supported for this task_review action")
)

func isMutableTaskStatus(status string) bool {
	switch status {
	case "pending", "in_progress":
		return true
	default:
		return false
	}
}

func isCompleteTaskStatus(status string) bool {
	switch status {
	case "complete", "completed":
		return true
	default:
		return false
	}
}

func isBlockedTaskStatus(status string) bool {
	return status == "blocked"
}
