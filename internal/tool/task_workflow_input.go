package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrTaskWorkflowActionInvalid = errors.New("action must be list, create, update, block, or complete")

type taskWorkflowInput struct {
	Action        string
	TaskID        string
	ParentTaskID  string
	Title         string
	Kind          string
	Status        string
	Notes         string
	Progress      string
	BlockReason   string
	Summary       string
	ReviewStatus  string
	ReviewSummary string
}

func parseTaskWorkflowInput(args json.RawMessage) (taskWorkflowInput, error) {
	var raw struct {
		Action        *string `json:"action"`
		TaskID        *string `json:"task_id"`
		ParentTaskID  *string `json:"parent_task_id"`
		Title         *string `json:"title"`
		Kind          *string `json:"kind"`
		Status        *string `json:"status"`
		Notes         *string `json:"notes"`
		Progress      *string `json:"progress"`
		BlockReason   *string `json:"block_reason"`
		Summary       *string `json:"summary"`
		ReviewStatus  *string `json:"review_status"`
		ReviewSummary *string `json:"review_summary"`
	}
	if err := DecodeArgs(TaskWorkflowToolName, args, &raw); err != nil {
		return taskWorkflowInput{}, err
	}

	input := taskWorkflowInput{
		Action:        strings.TrimSpace(stringValue(raw.Action)),
		TaskID:        strings.TrimSpace(stringValue(raw.TaskID)),
		ParentTaskID:  strings.TrimSpace(stringValue(raw.ParentTaskID)),
		Title:         strings.TrimSpace(stringValue(raw.Title)),
		Kind:          strings.TrimSpace(stringValue(raw.Kind)),
		Status:        strings.TrimSpace(stringValue(raw.Status)),
		Notes:         strings.TrimSpace(stringValue(raw.Notes)),
		Progress:      strings.TrimSpace(stringValue(raw.Progress)),
		BlockReason:   strings.TrimSpace(stringValue(raw.BlockReason)),
		Summary:       strings.TrimSpace(stringValue(raw.Summary)),
		ReviewStatus:  strings.TrimSpace(stringValue(raw.ReviewStatus)),
		ReviewSummary: strings.TrimSpace(stringValue(raw.ReviewSummary)),
	}

	if input.Action == "" {
		return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskActionRequired)
	}
	switch input.Action {
	case taskActionList:
		if fields := input.nonEmptyFields("task_id", "parent_task_id", "title", "kind", "status", "notes", "progress", "block_reason", "summary", "review_status", "review_summary"); len(fields) > 0 {
			return taskWorkflowInput{}, invalidTaskWorkflowFields(input.Action, fields...)
		}
		return input, nil
	case taskActionCreate:
		if input.Title == "" {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskInputTitleRequired)
		}
		if input.ParentTaskID != "" && input.ParentTaskID == input.TaskID {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskParentInputInvalid)
		}
		if input.Status == "" {
			input.Status = "pending"
		}
		if isCompleteTaskStatus(input.Status) {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskCompleteActionOnly)
		}
		if !isMutableTaskStatus(input.Status) {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskStatusInvalid)
		}
		if fields := input.nonEmptyFields("progress", "block_reason", "summary", "review_status", "review_summary"); len(fields) > 0 {
			return taskWorkflowInput{}, invalidTaskWorkflowFields(input.Action, fields...)
		}
		return input, nil
	case taskActionUpdate:
		if input.TaskID == "" {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskInputIDRequired)
		}
		if fields := input.nonEmptyFields("parent_task_id", "title", "kind", "review_status", "review_summary"); len(fields) > 0 {
			return taskWorkflowInput{}, invalidTaskWorkflowFields(input.Action, fields...)
		}
		if input.Status == "" && input.Progress == "" && input.Notes == "" {
			if fields := input.nonEmptyFields("block_reason", "summary"); len(fields) > 0 {
				return taskWorkflowInput{}, invalidTaskWorkflowFields(input.Action, fields...)
			}
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskInputUpdateRequired)
		}
		if isBlockedTaskStatus(input.Status) {
			input.Action = taskActionBlock
			input.Status = ""
			if input.BlockReason == "" {
				switch {
				case input.Progress != "":
					input.BlockReason = input.Progress
					input.Progress = ""
				case input.Notes != "":
					input.BlockReason = input.Notes
					input.Notes = ""
				}
			}
			if input.BlockReason == "" {
				return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskBlockReasonRequired)
			}
			return input, nil
		}
		if isCompleteTaskStatus(input.Status) {
			input.Action = taskActionComplete
			input.Status = ""
			if input.Summary == "" {
				switch {
				case input.Progress != "":
					input.Summary = input.Progress
				case input.Notes != "":
					input.Summary = input.Notes
				}
			}
			input.Progress = ""
			input.Notes = ""
			if input.Summary == "" {
				return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskSummaryRequired)
			}
			return input, nil
		}
		if input.Status != "" && !isMutableTaskStatus(input.Status) {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskStatusInvalid)
		}
		if fields := input.nonEmptyFields("block_reason", "summary", "review_status", "review_summary"); len(fields) > 0 {
			return taskWorkflowInput{}, invalidTaskWorkflowFields(input.Action, fields...)
		}
		return input, nil
	case taskActionBlock:
		if input.TaskID == "" {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskInputIDRequired)
		}
		if fields := input.nonEmptyFields("parent_task_id", "title", "kind", "status", "progress", "summary", "review_status", "review_summary"); len(fields) > 0 {
			return taskWorkflowInput{}, invalidTaskWorkflowFields(input.Action, fields...)
		}
		if input.BlockReason == "" {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskBlockReasonRequired)
		}
		return input, nil
	case taskActionComplete:
		if input.TaskID == "" {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskInputIDRequired)
		}
		if fields := input.nonEmptyFields("parent_task_id", "title", "kind", "status", "notes", "progress", "block_reason", "review_status", "review_summary"); len(fields) > 0 {
			return taskWorkflowInput{}, invalidTaskWorkflowFields(input.Action, fields...)
		}
		if input.Summary == "" {
			return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskSummaryRequired)
		}
		return input, nil
	default:
		return taskWorkflowInput{}, InvalidArguments(TaskWorkflowToolName, ErrTaskWorkflowActionInvalid)
	}
}

func (input taskWorkflowInput) nonEmptyFields(fields ...string) []string {
	var out []string
	for _, field := range fields {
		if input.fieldValue(field) != "" {
			out = append(out, field)
		}
	}
	return out
}

func (input taskWorkflowInput) fieldValue(field string) string {
	switch field {
	case "task_id":
		return input.TaskID
	case "parent_task_id":
		return input.ParentTaskID
	case "title":
		return input.Title
	case "kind":
		return input.Kind
	case "status":
		return input.Status
	case "notes":
		return input.Notes
	case "progress":
		return input.Progress
	case "block_reason":
		return input.BlockReason
	case "summary":
		return input.Summary
	case "review_status":
		return input.ReviewStatus
	case "review_summary":
		return input.ReviewSummary
	default:
		return ""
	}
}

func invalidTaskWorkflowFields(action string, fields ...string) error {
	detail := strings.Join(fields, ", ")
	if action = strings.TrimSpace(action); action != "" {
		detail = "action=" + action + " fields=" + detail
	}
	return InvalidArguments(TaskWorkflowToolName, fmt.Errorf("%w: %s", ErrTaskWorkflowFieldUnsupported, detail))
}
