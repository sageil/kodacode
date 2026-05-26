package events

import "testing"

func TestProjectorTracksTaskLifecycle(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "_session", SessionConfiguredPayload{WorkspaceRoot: "/repo"}),
		testEvent(1, "session-1", "turn-1", TaskCreatedPayload{
			Title:  "Investigate duplicate reads",
			Kind:   "analysis",
			Status: TaskStatusPending,
			Notes:  "focus on read tool reuse",
		}),
		testEvent(2, "session-1", "turn-1", TaskProgressUpdatedPayload{
			TaskID:   "task-1",
			Status:   TaskStatusInProgress,
			Progress: "checked runtime reuse path",
		}),
		testEvent(3, "session-1", "turn-1", TaskBlockedPayload{
			TaskID:      "task-1",
			BlockReason: "needs user input",
			Notes:       "waiting on scope clarification",
		}),
		testEvent(4, "session-1", "turn-1", TaskReviewedPayload{
			TaskID:        "task-1",
			ReviewStatus:  TaskReviewStatusConcern,
			ReviewSummary: "reuse key still too literal",
		}),
		testEvent(5, "session-1", "turn-1", TaskCompletedPayload{
			TaskID:  "task-1",
			Summary: "task flow is now durable",
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	if len(state.TaskOrder) != 1 || state.TaskOrder[0] != "task-1" {
		t.Fatalf("task order = %#v", state.TaskOrder)
	}
	task := state.Tasks["task-1"]
	if task == nil {
		t.Fatal("task missing")
	}
	if task.Title != "Investigate duplicate reads" || task.Kind != "analysis" {
		t.Fatalf("task = %#v", task)
	}
	if task.Status != TaskStatusCompleted {
		t.Fatalf("status = %q, want completed", task.Status)
	}
	if task.BlockReason != "" {
		t.Fatalf("block reason = %q, want cleared", task.BlockReason)
	}
	if task.ReviewStatus != TaskReviewStatusConcern || task.ReviewSummary != "reuse key still too literal" {
		t.Fatalf("review = %#v", task)
	}
	if task.Progress != "task flow is now durable" {
		t.Fatalf("progress = %q", task.Progress)
	}
	if task.CreatedAtSeq != 1 || task.UpdatedAtSeq != 5 || task.CompletedAtSeq != 5 {
		t.Fatalf("task seqs = %#v", task)
	}
}

func TestProjectorRejectsTaskUpdateForUnknownTask(t *testing.T) {
	projector := NewProjector("session-1")

	err := projector.Apply(testEvent(0, "session-1", "turn-1", TaskProgressUpdatedPayload{
		TaskID:   "task-missing",
		Status:   TaskStatusInProgress,
		Progress: "started",
	}))
	if err == nil {
		t.Fatal("Apply(task_progress_updated) error = nil, want failure")
	}
}
