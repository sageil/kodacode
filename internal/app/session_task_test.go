package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestSessionServiceTaskLifecycleUpdatesSnapshot(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	created, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Title:     "Implement task workflow",
		Kind:      "implementation",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if created.TaskID == "" || created.Status != events.TaskStatusPending {
		t.Fatalf("created = %#v", created)
	}

	if _, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    created.TaskID,
		Status:    events.TaskStatusInProgress,
		Progress:  "wired projector state",
	}); err != nil {
		t.Fatalf("UpdateTaskProgress() error = %v", err)
	}
	if _, err := service.BlockTask(context.Background(), BlockTaskInput{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		TaskID:      created.TaskID,
		BlockReason: "needs UI review",
	}); err != nil {
		t.Fatalf("BlockTask() error = %v", err)
	}
	if _, err := service.ReviewTask(context.Background(), ReviewTaskInput{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		TaskID:        created.TaskID,
		ReviewStatus:  events.TaskReviewStatusPass,
		ReviewSummary: "task state is durable",
	}); err != nil {
		t.Fatalf("ReviewTask() error = %v", err)
	}
	if _, err := service.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    created.TaskID,
		Summary:   "feature landed",
	}); err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}

	tasks, err := service.ListTasks(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v", tasks)
	}
	if got := tasks[0]; got.Status != events.TaskStatusCompleted || got.ReviewStatus != events.TaskReviewStatusPass || got.Progress != "feature landed" {
		t.Fatalf("task = %#v", got)
	}
}

func TestSessionServiceCreateTaskRejectsDuplicateCustomID(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    "investigate-read",
		Title:     "Investigate read tool",
	}); err != nil {
		t.Fatalf("CreateTask(first) error = %v", err)
	}
	if _, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    "investigate-read",
		Title:     "Duplicate",
	}); err == nil || !strings.Contains(err.Error(), "task already exists") {
		t.Fatalf("CreateTask(duplicate) error = %v, want duplicate failure", err)
	}
}

func TestSessionServiceTaskWorkflowRejectsSecondInProgressTask(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Title:     "Inspect middleware layout",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask(first) error = %v", err)
	}
	second, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Title:     "Refine auth middleware",
	})
	if err != nil {
		t.Fatalf("CreateTask(second) error = %v", err)
	}

	_, err = service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    second.TaskID,
		Status:    events.TaskStatusInProgress,
	})
	if err == nil || !strings.Contains(err.Error(), ErrTaskAnotherInProgress.Error()) {
		t.Fatalf("UpdateTaskProgress() error = %v, want ErrTaskAnotherInProgress", err)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Tasks[first.TaskID].Status; got != events.TaskStatusInProgress {
		t.Fatalf("first task status = %q", got)
	}
	if got := state.Tasks[second.TaskID].Status; got != events.TaskStatusPending {
		t.Fatalf("second task status = %q", got)
	}
}

func TestSessionServiceTaskWorkflowLeavesSolePendingTaskPendingAfterComplete(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Title:     "Audit middleware contracts",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask(first) error = %v", err)
	}
	second, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Title:     "Consolidate request validation",
	})
	if err != nil {
		t.Fatalf("CreateTask(second) error = %v", err)
	}

	if _, err := service.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    first.TaskID,
		Summary:   "audit done",
	}); err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Tasks[first.TaskID].Status; got != events.TaskStatusCompleted {
		t.Fatalf("first task status = %q", got)
	}
	if got := state.Tasks[second.TaskID].Status; got != events.TaskStatusPending {
		t.Fatalf("second task status = %q, want pending", got)
	}
}

func TestSessionServiceTaskWorkflowAllowsStatusOnlyUpdate(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	task, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		Title:     "Inspect runtime behavior",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	appendConfiguredTurnForTaskWorkflowTest(t, service, "session-1", "turn-1")

	updated, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    task.TaskID,
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("UpdateTaskProgress(status only) error = %v", err)
	}
	if updated.Status != events.TaskStatusInProgress {
		t.Fatalf("updated status = %q, want in_progress", updated.Status)
	}
}

func TestSessionServiceTaskWorkflowAllowsKickoffProgress(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	task, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		Title:     "Inspect runtime behavior",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	appendConfiguredTurnForTaskWorkflowTest(t, service, "session-1", "turn-1")

	updated, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    task.TaskID,
		Status:    events.TaskStatusInProgress,
		Progress:  "starting the first implementation pass",
	})
	if err != nil {
		t.Fatalf("UpdateTaskProgress(kickoff progress) error = %v", err)
	}
	if updated.Status != events.TaskStatusInProgress {
		t.Fatalf("updated status = %q, want in_progress", updated.Status)
	}
	if updated.Progress != "starting the first implementation pass" {
		t.Fatalf("updated progress = %q", updated.Progress)
	}
}

func TestSessionServiceTaskWorkflowAllowsProgressWithoutEvidence(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	task, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		Title:     "Inspect runtime behavior",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	appendConfiguredTurnForTaskWorkflowTest(t, service, "session-1", "turn-1")

	updated, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    task.TaskID,
		Progress:  "verified behavior",
	})
	if err != nil {
		t.Fatalf("UpdateTaskProgress() error = %v", err)
	}
	if updated.Progress != "verified behavior" {
		t.Fatalf("updated progress = %q", updated.Progress)
	}
}

func TestSessionServiceTaskWorkflowAllowsCompletionWithSummaryOnly(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	task, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		Title:     "Inspect runtime behavior",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	appendConfiguredTurnForTaskWorkflowTest(t, service, "session-1", "turn-1")

	completed, err := service.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    task.TaskID,
		Summary:   "all good",
	})
	if err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}
	if completed.Status != events.TaskStatusCompleted || completed.Progress != "all good" {
		t.Fatalf("completed task = %#v", completed)
	}
}

func TestTaskWorkflowErrorsAreConcise(t *testing.T) {
	for name, err := range map[string]error{
		"active_leaf":      ErrTaskAnotherInProgress,
		"parent":           ErrTaskParentNotFound,
		"parent_completed": ErrTaskParentCompleted,
		"complete_summary": ErrTaskCompletionSummaryRequired,
		"child_incomplete": ErrTaskChildTasksIncomplete,
	} {
		t.Run(name, func(t *testing.T) {
			message := err.Error()
			if strings.Count(message, " ") > 8 {
				t.Fatalf("error %q too verbose: %q", name, message)
			}
		})
	}
}

func TestSessionServiceTaskWorkflowRejectsCompletionWithoutSummary(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	task, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		Title:     "Inspect runtime behavior",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    task.TaskID,
		Progress:  "inspected the relevant file",
	}); err != nil {
		t.Fatalf("UpdateTaskProgress() error = %v", err)
	}

	_, err = service.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    task.TaskID,
	})
	if err != ErrTaskCompletionSummaryRequired {
		t.Fatalf("CompleteTask() error = %v, want ErrTaskCompletionSummaryRequired", err)
	}
}

func TestSessionServiceTaskWorkflowAllowsProgressAndCompletion(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	task, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		Title:     "Inspect runtime behavior",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	updated, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    task.TaskID,
		Status:    events.TaskStatusInProgress,
		Progress:  "inspected the relevant file",
	})
	if err != nil {
		t.Fatalf("UpdateTaskProgress() error = %v", err)
	}
	if updated.Progress != "inspected the relevant file" {
		t.Fatalf("updated progress = %q", updated.Progress)
	}

	completed, err := service.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    task.TaskID,
		Summary:   "inspection complete",
	})
	if err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}
	if completed.Status != events.TaskStatusCompleted || completed.Progress != "inspection complete" {
		t.Fatalf("completed task = %#v", completed)
	}
}

func TestSessionServiceTaskWorkflowKeepsActiveParentWhenCreatingChild(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	parent, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    "task-9",
		Title:     "Project review",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	child, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID:    "session-1",
		TurnID:       "turn-setup",
		TaskID:       "task-103",
		ParentTaskID: parent.TaskID,
		Title:        "Apply backend optimizations",
	})
	if err != nil {
		t.Fatalf("CreateTask(child) error = %v", err)
	}
	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Tasks[parent.TaskID].Status; got != events.TaskStatusInProgress {
		t.Fatalf("parent status = %q, want in_progress", got)
	}
	if child.ParentTaskID != parent.TaskID {
		t.Fatalf("child parent = %q, want %q", child.ParentTaskID, parent.TaskID)
	}

	updated, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    child.TaskID,
		Status:    events.TaskStatusInProgress,
		Progress:  "starting backend optimizations",
	})
	if err != nil {
		t.Fatalf("UpdateTaskProgress(child) error = %v", err)
	}
	if updated.Status != events.TaskStatusInProgress || updated.Progress != "starting backend optimizations" {
		t.Fatalf("updated child = %#v", updated)
	}
}

func TestSessionServiceTaskWorkflowRejectsChildUnderCompletedParent(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	parent, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    "task-parent",
		Title:     "Performance epic",
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	if _, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    parent.TaskID,
		Progress:  "analysis done",
	}); err != nil {
		t.Fatalf("UpdateTaskProgress(parent) error = %v", err)
	}
	if _, err := service.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    parent.TaskID,
		Summary:   "epic complete",
	}); err != nil {
		t.Fatalf("CompleteTask(parent) error = %v", err)
	}

	if _, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		TaskID:       "task-child",
		ParentTaskID: parent.TaskID,
		Title:        "Optimize backend queries",
	}); err != ErrTaskParentCompleted {
		t.Fatalf("CreateTask(child) error = %v, want ErrTaskParentCompleted", err)
	}
}

func TestSessionServiceTaskWorkflowRequiresChildCompletionBeforeParentComplete(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	parent, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    "task-parent",
		Title:     "Performance epic",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	child, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID:    "session-1",
		TurnID:       "turn-setup",
		TaskID:       "task-child",
		ParentTaskID: parent.TaskID,
		Title:        "Optimize backend queries",
	})
	if err != nil {
		t.Fatalf("CreateTask(child) error = %v", err)
	}

	if _, err := service.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    parent.TaskID,
		Summary:   "epic complete",
	}); err != ErrTaskChildTasksIncomplete {
		t.Fatalf("CompleteTask(parent) error = %v, want ErrTaskChildTasksIncomplete", err)
	}

	if _, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    child.TaskID,
		Status:    events.TaskStatusInProgress,
		Progress:  "query work started",
	}); err != nil {
		t.Fatalf("UpdateTaskProgress(child) error = %v", err)
	}
	if _, err := service.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    child.TaskID,
		Summary:   "backend queries optimized",
	}); err != nil {
		t.Fatalf("CompleteTask(child) error = %v", err)
	}

	completed, err := service.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    parent.TaskID,
		Summary:   "epic complete",
	})
	if err != nil {
		t.Fatalf("CompleteTask(parent) error = %v", err)
	}
	if completed.Status != events.TaskStatusCompleted || completed.Progress != "epic complete" {
		t.Fatalf("completed parent = %#v", completed)
	}
}

func TestSessionServiceTaskWorkflowAllowsStartingParentTaskWithChildren(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	parent, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    "task-parent",
		Title:     "Performance epic",
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	if _, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		TaskID:       "task-child",
		ParentTaskID: parent.TaskID,
		Title:        "Optimize backend queries",
	}); err != nil {
		t.Fatalf("CreateTask(child) error = %v", err)
	}

	updated, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-2",
		TaskID:    parent.TaskID,
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("UpdateTaskProgress() error = %v", err)
	}
	if updated.Status != events.TaskStatusInProgress {
		t.Fatalf("updated task = %#v", updated)
	}
}

func TestSessionServiceTaskWorkflowKeepsActiveParentInProgressWhenCreatingChild(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	parent, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    "task-parent",
		Title:     "Performance epic",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	child, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		TaskID:       "task-child",
		ParentTaskID: parent.TaskID,
		Title:        "Optimize backend queries",
	})
	if err != nil {
		t.Fatalf("CreateTask(child) error = %v", err)
	}
	if child.ParentTaskID != parent.TaskID {
		t.Fatalf("child parent = %q, want %q", child.ParentTaskID, parent.TaskID)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Tasks[parent.TaskID].Status; got != events.TaskStatusInProgress {
		t.Fatalf("parent status = %q, want in_progress", got)
	}
	if got := state.Tasks[child.TaskID].Status; got != events.TaskStatusPending {
		t.Fatalf("child status = %q, want pending", got)
	}
}

func TestSessionServiceTaskWorkflowAllowsActiveChildOnActiveParentPath(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	parent, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    "task-parent",
		Title:     "Performance epic",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	child, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		TaskID:       "task-child",
		ParentTaskID: parent.TaskID,
		Title:        "Optimize backend queries",
	})
	if err != nil {
		t.Fatalf("CreateTask(child) error = %v", err)
	}

	updated, err := service.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-2",
		TaskID:    child.TaskID,
		Status:    events.TaskStatusInProgress,
		Progress:  "query work started",
	})
	if err != nil {
		t.Fatalf("UpdateTaskProgress(child) error = %v", err)
	}
	if updated.Status != events.TaskStatusInProgress {
		t.Fatalf("updated child = %#v", updated)
	}

	state, err := service.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Tasks[parent.TaskID].Status; got != events.TaskStatusInProgress {
		t.Fatalf("parent status = %q, want in_progress", got)
	}
	if got := state.Tasks[child.TaskID].Status; got != events.TaskStatusInProgress {
		t.Fatalf("child status = %q, want in_progress", got)
	}
}

func TestSessionServiceTaskWorkflowRejectsStartingUnrelatedActiveBranch(t *testing.T) {
	store := events.NewMemoryStore()
	service, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}

	if _, err := service.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		TaskID:    "task-parent",
		Title:     "Performance epic",
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	_, err = service.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-2",
		TaskID:    "task-other",
		Title:     "Unrelated workstream",
		Status:    events.TaskStatusInProgress,
	})
	if err == nil || !errors.Is(err, ErrTaskAnotherInProgress) {
		t.Fatalf("CreateTask(unrelated) error = %v, want ErrTaskAnotherInProgress", err)
	}
}

func appendConfiguredTurnForTaskWorkflowTest(t *testing.T, service *SessionService, sessionID, turnID string) {
	t.Helper()
	_, err := service.append(context.Background(), events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnConfigured,
		Payload: newTurnConfiguredPayload(
			TurnCapabilities{
				AgentID:    "engineer",
				ModelRoute: baseModelRoute(),
				AllowedTools: []string{
					"read",
					"task_workflow",
				},
			},
			false,
			false,
			"",
			ResponseStyleDefault,
			false,
		),
	})
	if err != nil {
		t.Fatalf("append(turn_configured) error = %v", err)
	}
}
