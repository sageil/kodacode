package events

import (
	"errors"
	"strings"
)

const (
	TypeTaskCreated         Type = "task_created"
	TypeTaskProgressUpdated Type = "task_progress_updated"
	TypeTaskBlocked         Type = "task_blocked"
	TypeTaskCompleted       Type = "task_completed"
	TypeTaskReviewed        Type = "task_reviewed"
)

type TaskCreatedPayload struct {
	TaskID          string `json:"task_id"`
	ParentTaskID    string `json:"parent_task_id"`
	WorkflowID      string `json:"workflow_id,omitempty"`
	WorkflowPhaseID string `json:"workflow_phase_id,omitempty"`
	Title           string `json:"title"`
	Kind            string `json:"kind"`
	Status          string `json:"status"`
	Notes           string `json:"notes"`
}

func (TaskCreatedPayload) eventType() Type { return TypeTaskCreated }

func (p TaskCreatedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.Title) == "":
		return errors.New("title is required")
	case strings.TrimSpace(p.Status) == "":
		return errors.New("status is required")
	case !isValidTaskMutableStatus(p.Status):
		return errors.New("status must be pending or in_progress")
	default:
		return nil
	}
}

type TaskProgressUpdatedPayload struct {
	TaskID          string `json:"task_id"`
	WorkflowID      string `json:"workflow_id,omitempty"`
	WorkflowPhaseID string `json:"workflow_phase_id,omitempty"`
	Status          string `json:"status"`
	Progress        string `json:"progress"`
	Notes           string `json:"notes"`
}

func (TaskProgressUpdatedPayload) eventType() Type { return TypeTaskProgressUpdated }

func (p TaskProgressUpdatedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.TaskID) == "":
		return errors.New("task_id is required")
	case strings.TrimSpace(p.Status) != "" && !isValidTaskMutableStatus(p.Status):
		return errors.New("status must be pending or in_progress when set")
	case strings.TrimSpace(p.Status) == "" && strings.TrimSpace(p.Progress) == "" && strings.TrimSpace(p.Notes) == "":
		return errors.New("status, progress, or notes is required")
	default:
		return nil
	}
}

type TaskBlockedPayload struct {
	TaskID          string `json:"task_id"`
	WorkflowID      string `json:"workflow_id,omitempty"`
	WorkflowPhaseID string `json:"workflow_phase_id,omitempty"`
	BlockReason     string `json:"block_reason"`
	Notes           string `json:"notes"`
}

func (TaskBlockedPayload) eventType() Type { return TypeTaskBlocked }

func (p TaskBlockedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.TaskID) == "":
		return errors.New("task_id is required")
	case strings.TrimSpace(p.BlockReason) == "":
		return errors.New("block_reason is required")
	default:
		return nil
	}
}

type TaskCompletedPayload struct {
	TaskID          string `json:"task_id"`
	WorkflowID      string `json:"workflow_id,omitempty"`
	WorkflowPhaseID string `json:"workflow_phase_id,omitempty"`
	Summary         string `json:"summary"`
}

func (TaskCompletedPayload) eventType() Type { return TypeTaskCompleted }

func (p TaskCompletedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.TaskID) == "":
		return errors.New("task_id is required")
	case strings.TrimSpace(p.Summary) == "":
		return errors.New("summary is required")
	}
	return nil
}

type TaskReviewedPayload struct {
	TaskID          string `json:"task_id"`
	WorkflowID      string `json:"workflow_id,omitempty"`
	WorkflowPhaseID string `json:"workflow_phase_id,omitempty"`
	ReviewStatus    string `json:"review_status"`
	ReviewSummary   string `json:"review_summary"`
}

func (TaskReviewedPayload) eventType() Type { return TypeTaskReviewed }

func (p TaskReviewedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.TaskID) == "":
		return errors.New("task_id is required")
	case strings.TrimSpace(p.ReviewStatus) == "":
		return errors.New("review_status is required")
	case !isValidTaskReviewStatus(p.ReviewStatus):
		return errors.New("review_status must be pass, concern, fail, or accepted")
	case strings.TrimSpace(p.ReviewSummary) == "":
		return errors.New("review_summary is required")
	default:
		return nil
	}
}
