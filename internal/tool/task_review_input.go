package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrTaskReviewActionInvalid = errors.New("action must be list or review")

type taskReviewInput struct {
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

func parseTaskReviewInput(args json.RawMessage) (taskReviewInput, error) {
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
	if err := DecodeArgs(TaskReviewToolName, args, &raw); err != nil {
		return taskReviewInput{}, err
	}

	input := taskReviewInput{
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
		return taskReviewInput{}, InvalidArguments(TaskReviewToolName, ErrTaskActionRequired)
	}
	switch input.Action {
	case taskActionList:
		if fields := input.nonEmptyFields("task_id", "parent_task_id", "title", "kind", "status", "notes", "progress", "block_reason", "summary", "review_status", "review_summary"); len(fields) > 0 {
			return taskReviewInput{}, invalidTaskReviewFields(input.Action, fields...)
		}
		return input, nil
	case taskActionReview:
		if fields := input.nonEmptyFields("parent_task_id", "title", "kind", "status", "notes", "progress", "block_reason", "summary"); len(fields) > 0 {
			return taskReviewInput{}, invalidTaskReviewFields(input.Action, fields...)
		}
		if input.TaskID == "" {
			return taskReviewInput{}, InvalidArguments(TaskReviewToolName, ErrTaskInputIDRequired)
		}
		switch input.ReviewStatus {
		case "pass", "concern", "fail", "accepted":
		default:
			return taskReviewInput{}, InvalidArguments(TaskReviewToolName, ErrTaskReviewStatusInvalid)
		}
		if input.ReviewSummary == "" {
			return taskReviewInput{}, InvalidArguments(TaskReviewToolName, ErrTaskReviewSummaryNeeded)
		}
		return input, nil
	default:
		return taskReviewInput{}, InvalidArguments(TaskReviewToolName, ErrTaskReviewActionInvalid)
	}
}

func (input taskReviewInput) nonEmptyFields(fields ...string) []string {
	var out []string
	for _, field := range fields {
		if input.fieldValue(field) != "" {
			out = append(out, field)
		}
	}
	return out
}

func (input taskReviewInput) fieldValue(field string) string {
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

func invalidTaskReviewFields(action string, fields ...string) error {
	detail := strings.Join(fields, ", ")
	if action = strings.TrimSpace(action); action != "" {
		detail = "action=" + action + " fields=" + detail
	}
	return InvalidArguments(TaskReviewToolName, fmt.Errorf("%w: %s", ErrTaskReviewFieldUnsupported, detail))
}
