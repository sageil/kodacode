package events

import (
	"fmt"
	"strings"
)

const (
	TaskStatusPending    = "pending"
	TaskStatusInProgress = "in_progress"
	TaskStatusBlocked    = "blocked"
	TaskStatusCompleted  = "completed"
)

const (
	TaskReviewStatusPass     = "pass"
	TaskReviewStatusConcern  = "concern"
	TaskReviewStatusFail     = "fail"
	TaskReviewStatusAccepted = "accepted"
)

type TaskState struct {
	TaskID          string
	ParentTaskID    string
	WorkflowID      string
	WorkflowPhaseID string
	Title           string
	Kind            string
	Status          string
	Notes           string
	Progress        string
	BlockReason     string
	ReviewStatus    string
	ReviewSummary   string
	CreatedAtSeq    int64
	UpdatedAtSeq    int64
	CompletedAtSeq  int64
}

func cloneTaskState(state *TaskState) *TaskState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}

func cloneTaskStates(tasks map[string]*TaskState) map[string]*TaskState {
	if len(tasks) == 0 {
		return nil
	}
	out := make(map[string]*TaskState, len(tasks))
	for id, task := range tasks {
		out[id] = cloneTaskState(task)
	}
	return out
}

func taskIDFromSequence(sequence int64) string {
	return fmt.Sprintf("task-%d", sequence)
}

func resolveTaskID(taskID string, sequence int64) string {
	taskID = strings.TrimSpace(taskID)
	if taskID != "" {
		return taskID
	}
	return taskIDFromSequence(sequence)
}

func isValidTaskMutableStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "", TaskStatusPending, TaskStatusInProgress:
		return true
	default:
		return false
	}
}

func isValidTaskReviewStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case TaskReviewStatusPass, TaskReviewStatusConcern, TaskReviewStatusFail, TaskReviewStatusAccepted:
		return true
	default:
		return false
	}
}
