package app

import (
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

const (
	toolErrorRecoveryReadThenRetry       = "read_then_retry"
	toolErrorRecoveryRetryWithValidInput = "retry_with_valid_input"
	toolErrorRecoveryListThenRetry       = "list_then_retry"
	toolErrorRecoveryUseAvailableTool    = "use_available_tool"
)

func toolExecutionErrorDetail(input ExecuteToolInput, cause error, errorText string) *events.ToolErrorDetail {
	message := strings.TrimSpace(errorText)
	if message == "" && cause != nil {
		message = strings.TrimSpace(toolExecutionErrorText(input.ToolName, cause))
	}
	if message == "" {
		return nil
	}

	code, retryable, recovery := toolErrorCode(input.ToolName, cause)
	if code == "" {
		code = "tool_execution_failed"
	}
	return &events.ToolErrorDetail{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		Recovery:  recovery,
		Fields:    toolErrorFields(input, cause),
	}
}

func toolErrorCode(toolName string, cause error) (string, bool, string) {
	name := strings.TrimSpace(toolName)
	switch {
	case cause == nil:
		return "tool_returned_error", true, toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrInvalidArguments):
		if code, recovery := invalidArgumentToolErrorCode(name, cause); code != "" {
			return code, true, recovery
		}
		return "tool_invalid_arguments", true, toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, ErrToolNotFound):
		return "tool_not_found", false, toolErrorRecoveryUseAvailableTool
	case errors.Is(cause, ErrToolNotAllowed):
		return "tool_not_allowed", false, ""
	case errors.Is(cause, ErrPermissionPolicyDenied):
		return "permission_denied", false, ""
	case errors.Is(cause, ErrToolPendingQuestionConflict):
		return "tool_pending_question_conflict", false, ""
	case errors.Is(cause, ErrWriteRequiresRead):
		return "write_requires_read", true, toolErrorRecoveryReadThenRetry
	case errors.Is(cause, ErrWriteRequiresCompleteRead):
		return "write_requires_complete_read", true, toolErrorRecoveryReadThenRetry
	case errors.Is(cause, ErrWriteRequiresFreshRead):
		return "write_requires_fresh_read", true, toolErrorRecoveryReadThenRetry
	case errors.Is(cause, ErrPlannerSavePlanQuestionRequiresVisiblePlan):
		return "plan_question_requires_visible_plan", true, toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, ErrPlannerSavePlanQuestionInvalid):
		return "plan_question_invalid", true, toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, ErrPlannerPlanApprovalDisabledByWorkflow):
		return "plan_question_disabled_by_workflow", true, toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, ErrPlannerPlanApprovalDisabled):
		return "plan_question_disabled", true, toolErrorRecoveryRetryWithValidInput
	}

	if name == tool.TaskWorkflowToolName {
		if code, recovery := taskWorkflowToolErrorCode(cause); code != "" {
			return code, true, recovery
		}
	}
	if name == tool.TaskReviewToolName {
		if code, recovery := taskReviewToolErrorCode(cause); code != "" {
			return code, true, recovery
		}
	}

	return "tool_execution_failed", false, ""
}

func invalidArgumentToolErrorCode(toolName string, cause error) (string, string) {
	switch toolName {
	case tool.ReadToolName:
		return readToolErrorCode(cause)
	case tool.TaskWorkflowToolName:
		return taskWorkflowToolErrorCode(cause)
	case tool.TaskReviewToolName:
		return taskReviewToolErrorCode(cause)
	case tool.QuestionToolName:
		return questionToolErrorCode(cause)
	default:
		return "", ""
	}
}

func readToolErrorCode(cause error) (string, string) {
	switch {
	case errors.Is(cause, tool.ErrReadPathsRequired):
		return "read_paths_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrReadEmptyPath):
		return "read_empty_path", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrReadOffsetInvalid):
		return "read_offset_invalid", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrReadLimitInvalid):
		return "read_limit_invalid", toolErrorRecoveryRetryWithValidInput
	default:
		return "", ""
	}
}

func taskWorkflowToolErrorCode(cause error) (string, string) {
	switch {
	case errors.Is(cause, tool.ErrTaskActionRequired):
		return "task_action_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskCompleteActionOnly):
		return "task_complete_action_only", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskStatusInvalid):
		return "task_status_invalid", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskInputTitleRequired):
		return "task_title_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskParentInputInvalid):
		return "task_parent_input_invalid", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskInputIDRequired):
		return "task_id_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskInputUpdateRequired):
		return "task_update_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskBlockReasonRequired):
		return "task_block_reason_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskSummaryRequired), errors.Is(cause, ErrTaskCompletionSummaryRequired):
		return "task_summary_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskWorkflowActionInvalid):
		return "task_workflow_action_invalid", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskWorkflowFieldUnsupported):
		return "task_workflow_field_unsupported", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, events.ErrTaskNotFound):
		return "task_not_found", toolErrorRecoveryListThenRetry
	case errors.Is(cause, ErrTaskChildTasksIncomplete):
		return "task_child_tasks_incomplete", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, ErrTaskParentCompleted):
		return "task_parent_completed", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, ErrTaskParentNotFound):
		return "task_parent_not_found", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, ErrTaskParentSelfReference):
		return "task_parent_self_reference", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, ErrTaskAnotherInProgress):
		return "task_another_in_progress", toolErrorRecoveryRetryWithValidInput
	default:
		return "", ""
	}
}

func taskReviewToolErrorCode(cause error) (string, string) {
	switch {
	case errors.Is(cause, tool.ErrTaskActionRequired):
		return "task_review_action_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskInputIDRequired):
		return "task_review_task_id_required", toolErrorRecoveryListThenRetry
	case errors.Is(cause, tool.ErrTaskReviewStatusInvalid):
		return "task_review_status_invalid", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskReviewSummaryNeeded):
		return "task_review_summary_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskReviewActionInvalid):
		return "task_review_action_invalid", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrTaskReviewFieldUnsupported):
		return "task_review_field_unsupported", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, events.ErrTaskNotFound):
		return "task_review_task_not_found", toolErrorRecoveryListThenRetry
	default:
		return "", ""
	}
}

func questionToolErrorCode(cause error) (string, string) {
	switch {
	case errors.Is(cause, tool.ErrQuestionRequired):
		return "question_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrQuestionOptionsEmpty):
		return "question_options_required", toolErrorRecoveryRetryWithValidInput
	case errors.Is(cause, tool.ErrQuestionOptionInvalid):
		return "question_option_invalid", toolErrorRecoveryRetryWithValidInput
	default:
		return "", ""
	}
}

func toolErrorFields(input ExecuteToolInput, cause error) map[string]string {
	fields := make(map[string]string)
	if toolName := strings.TrimSpace(input.ToolName); toolName != "" {
		fields["tool_name"] = toolName
	}
	var invalid *tool.InvalidArgumentsError
	if errors.As(cause, &invalid) && strings.TrimSpace(invalid.ToolName) != "" {
		fields["argument_tool_name"] = strings.TrimSpace(invalid.ToolName)
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func cloneToolErrorDetail(detail *events.ToolErrorDetail) *events.ToolErrorDetail {
	if detail == nil {
		return nil
	}
	out := *detail
	if len(detail.Fields) > 0 {
		out.Fields = make(map[string]string, len(detail.Fields))
		for key, value := range detail.Fields {
			out.Fields[key] = value
		}
	}
	return &out
}
