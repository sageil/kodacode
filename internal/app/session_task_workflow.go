package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

var ErrTaskAnotherInProgress = errors.New("another task is in_progress")
var ErrTaskParentNotFound = errors.New("parent_task_id not found")
var ErrTaskParentSelfReference = errors.New("parent_task_id must reference another task")
var ErrTaskParentCompleted = errors.New("parent task is completed")
var ErrTaskCompletionSummaryRequired = errors.New("summary is required")
var ErrTaskChildTasksIncomplete = errors.New("child tasks not completed")

func validateTaskInProgressTransition(state events.SessionState, targetTaskID, targetParentTaskID, nextStatus string) error {
	if strings.TrimSpace(nextStatus) != events.TaskStatusInProgress {
		return nil
	}
	for _, taskID := range state.TaskOrder {
		if taskID == targetTaskID {
			continue
		}
		task := state.Tasks[taskID]
		if task == nil {
			continue
		}
		if strings.TrimSpace(task.Status) == events.TaskStatusInProgress &&
			!taskSharesActivePath(state, targetTaskID, targetParentTaskID, taskID) {
			return fmt.Errorf("%w: %s", ErrTaskAnotherInProgress, taskID)
		}
	}
	return nil
}

func taskSharesActivePath(state events.SessionState, targetTaskID, targetParentTaskID, otherTaskID string) bool {
	targetTaskID = strings.TrimSpace(targetTaskID)
	targetParentTaskID = strings.TrimSpace(targetParentTaskID)
	otherTaskID = strings.TrimSpace(otherTaskID)
	if otherTaskID == "" {
		return false
	}
	if targetTaskID != "" {
		return otherTaskID == targetTaskID ||
			taskIsAncestorOf(state, otherTaskID, targetTaskID) ||
			taskIsAncestorOf(state, targetTaskID, otherTaskID)
	}
	return taskIsAncestorOf(state, otherTaskID, targetParentTaskID)
}

func taskIsAncestorOf(state events.SessionState, ancestorTaskID, taskID string) bool {
	ancestorTaskID = strings.TrimSpace(ancestorTaskID)
	taskID = strings.TrimSpace(taskID)
	if ancestorTaskID == "" || taskID == "" {
		return false
	}
	visited := map[string]struct{}{}
	for currentID := taskID; currentID != ""; {
		if currentID == ancestorTaskID {
			return true
		}
		if _, seen := visited[currentID]; seen {
			return false
		}
		visited[currentID] = struct{}{}
		task := state.Tasks[currentID]
		if task == nil {
			return false
		}
		currentID = strings.TrimSpace(task.ParentTaskID)
	}
	return false
}

func validateTaskParent(state events.SessionState, taskID, parentTaskID string) error {
	parentTaskID = strings.TrimSpace(parentTaskID)
	if parentTaskID == "" {
		return nil
	}
	if taskID != "" && taskID == parentTaskID {
		return ErrTaskParentSelfReference
	}
	parentTask := state.Tasks[parentTaskID]
	if parentTask == nil {
		return ErrTaskParentNotFound
	}
	if strings.TrimSpace(parentTask.Status) == events.TaskStatusCompleted {
		return ErrTaskParentCompleted
	}
	return nil
}

func validateTaskCompletion(state events.SessionState, task *events.TaskState, summary string) error {
	if strings.TrimSpace(summary) == "" {
		return ErrTaskCompletionSummaryRequired
	}
	if task == nil {
		return events.ErrTaskNotFound
	}
	hasChildren, allChildrenCompleted := taskChildrenCompletionState(state, task.TaskID)
	if hasChildren {
		if !allChildrenCompleted {
			return ErrTaskChildTasksIncomplete
		}
	}
	return nil
}

func taskChildrenCompletionState(state events.SessionState, taskID string) (bool, bool) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false, false
	}
	hasChildren := false
	allCompleted := true
	for _, candidateID := range state.TaskOrder {
		task := state.Tasks[candidateID]
		if task == nil {
			continue
		}
		if strings.TrimSpace(task.ParentTaskID) == taskID {
			hasChildren = true
			if strings.TrimSpace(task.Status) != events.TaskStatusCompleted {
				allCompleted = false
			}
		}
	}
	return hasChildren, allCompleted
}
