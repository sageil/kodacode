package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestActiveWorkflowTaskSelectsDeepestActiveTask(t *testing.T) {
	state := events.SessionState{
		TaskOrder: []string{"task-parent", "task-child"},
		Tasks: map[string]*events.TaskState{
			"task-parent": {TaskID: "task-parent", Status: events.TaskStatusInProgress},
			"task-child":  {TaskID: "task-child", ParentTaskID: "task-parent", Status: events.TaskStatusInProgress},
		},
	}

	task := activeWorkflowTask(state)
	if task == nil || task.TaskID != "task-child" {
		t.Fatalf("activeWorkflowTask() = %#v, want task-child", task)
	}
}

func TestActiveWorkflowTaskReturnsParentWhenNoActiveDescendantExists(t *testing.T) {
	state := events.SessionState{
		TaskOrder: []string{"task-parent", "task-child"},
		Tasks: map[string]*events.TaskState{
			"task-parent": {TaskID: "task-parent", Status: events.TaskStatusInProgress},
			"task-child":  {TaskID: "task-child", ParentTaskID: "task-parent", Status: events.TaskStatusPending},
		},
	}

	task := activeWorkflowTask(state)
	if task == nil || task.TaskID != "task-parent" {
		t.Fatalf("activeWorkflowTask() = %#v, want task-parent", task)
	}
}

func TestActiveWorkflowTaskReturnsNilForAmbiguousBranches(t *testing.T) {
	state := events.SessionState{
		TaskOrder: []string{"task-a", "task-b"},
		Tasks: map[string]*events.TaskState{
			"task-a": {TaskID: "task-a", Status: events.TaskStatusInProgress},
			"task-b": {TaskID: "task-b", Status: events.TaskStatusInProgress},
		},
	}

	if task := activeWorkflowTask(state); task != nil {
		t.Fatalf("activeWorkflowTask() = %#v, want nil", task)
	}
}
