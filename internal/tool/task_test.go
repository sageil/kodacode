package tool_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func ectx(sid string) tool.ExecutionContext {
	return tool.ExecutionContext{SessionID: sid}
}

func newTestTaskTool() *tool.Tool {
	return tool.NewTaskTool(tool.NewTaskStore(nil))
}

type failingTaskRepo struct {
	createErr error
	updateErr error
	deleteErr error
}

func (f failingTaskRepo) Create(context.Context, repository.Task) (repository.Task, error) {
	return repository.Task{}, f.createErr
}

func (f failingTaskRepo) Update(context.Context, repository.Task) error {
	return f.updateErr
}

func (f failingTaskRepo) Delete(context.Context, string, string) error {
	return f.deleteErr
}

func (f failingTaskRepo) ListBySession(context.Context, string) ([]repository.Task, error) {
	return nil, nil
}

func (f failingTaskRepo) DeleteBySession(context.Context, string) error {
	return nil
}

func TestTaskTool_CreateAndList(t *testing.T) {
	tl := newTestTaskTool()

	res, err := tl.Execute(t.Context(), ectx("s1"), []byte(`{"action":"create","title":"Write tests"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "task 1") {
		t.Fatalf("expected task id task 1, got: %s", res.Output)
	}

	res, err = tl.Execute(t.Context(), ectx("s1"), []byte(`{"action":"create","title":"Review PR","status":"in_progress"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "task 2") {
		t.Fatalf("expected task id task 2, got: %s", res.Output)
	}

	res, err = tl.Execute(t.Context(), ectx("s1"), []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "[ ]") {
		t.Fatalf("expected pending icon, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "[>]") {
		t.Fatalf("expected in_progress icon, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Write tests") || !strings.Contains(res.Output, "Review PR") {
		t.Fatalf("expected both tasks in list, got: %s", res.Output)
	}
}

func TestTaskTool_Update(t *testing.T) {
	tl := newTestTaskTool()

	_, _ = tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Fix bug"}`))
	_, _ = tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 1","status":"in_progress"}`))

	res, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 1","status":"completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Updated task task 1") {
		t.Fatalf("expected update confirmation, got: %s", res.Output)
	}

	res, err = tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "[✓]") {
		t.Fatalf("expected completed icon, got: %s", res.Output)
	}
}

func TestTaskTool_UpdateNotesWithoutReplaceStoresProgress(t *testing.T) {
	tl := newTestTaskTool()

	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Fix bug","notes":"Acceptance criteria:\n- fix the bug\n- keep tests green"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 1","notes":"audit complete, findings documented"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Updated task task 1") {
		t.Fatalf("expected update confirmation, got: %s", res.Output)
	}

	res, err = tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Acceptance criteria:") {
		t.Fatalf("expected durable notes to remain in list output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Progress: audit complete, findings documented") {
		t.Fatalf("expected progress to capture transient note update, got: %s", res.Output)
	}
}

func TestTaskTool_ListShowsReviewAndBlockState(t *testing.T) {
	store := tool.NewTaskStore(nil)
	if _, _, err := store.CreateTaskWithKind(t.Context(), "s2", "Add role-based overrides", tool.TaskKindImplementation, "completed", "Acceptance criteria:\n- role overrides are strongly typed"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateTaskWorkflowState(t.Context(), "s2", "task 1", "blocked", tool.TaskReviewConcern, tool.TaskBlockReasonReviewCap, "Expected Partial<Record<UserRole, number>>; missing production-path override test."); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewTaskTool(store)

	res, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "State: blocked: reviewer retry limit; review: concern") {
		t.Fatalf("expected workflow state in list output, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "Review: Expected Partial<Record<UserRole, number>>") {
		t.Fatalf("expected review summary in list output, got: %s", res.Output)
	}
}

func TestTaskTool_UpdateReplaceNotesOverwritesDurableDefinition(t *testing.T) {
	tl := newTestTaskTool()

	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Fix bug","notes":"Acceptance criteria:\n- old definition"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 1","notes":"Acceptance criteria:\n- new definition","replaceNotes":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Updated task task 1") {
		t.Fatalf("expected update confirmation, got: %s", res.Output)
	}

	res, err = tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Output, "old definition") {
		t.Fatalf("expected old durable notes to be replaced, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "new definition") {
		t.Fatalf("expected new durable notes in list output, got: %s", res.Output)
	}
}

func TestTaskTool_AnalysisTaskRequiresProgressBeforeCompletion(t *testing.T) {
	tl := newTestTaskTool()

	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Audit middleware","kind":"analysis","notes":"Review auth/session/validation middleware"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 1","status":"in_progress"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 1","status":"completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != tool.ErrCodeInvalidArgs {
		t.Fatalf("ErrorCode = %q, want %q", res.ErrorCode, tool.ErrCodeInvalidArgs)
	}
	if !strings.Contains(res.Output, "analysis tasks require a short progress summary before completion") {
		t.Fatalf("unexpected output: %s", res.Output)
	}

	res, err = tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 1","status":"completed","progress":"audit complete, findings documented"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Updated task task 1") {
		t.Fatalf("expected completion after progress summary, got: %s", res.Output)
	}
}

func TestTaskTool_UpdateFallsBackToTaskReferenceInTitle(t *testing.T) {
	tl := newTestTaskTool()

	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Update Route Handlers to Consume ProposalService"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task_proposal_refactor","title":"task 1: Update Route Handlers to Consume ProposalService","status":"in_progress"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Updated task task 1") {
		t.Fatalf("expected fallback update on task 1, got: %s", res.Output)
	}

	res, err = tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "[>] task 1  Update Route Handlers to Consume ProposalService") {
		t.Fatalf("expected task 1 to be in progress without title clobber, got: %s", res.Output)
	}
}

func TestTaskTool_UpdateRejectsStartingSecondTaskWhileAnotherIsInProgress(t *testing.T) {
	tl := newTestTaskTool()

	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Task One","status":"in_progress"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Task Two"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 2","status":"in_progress"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != tool.ErrCodeInvalidArgs {
		t.Fatalf("ErrorCode = %q, want %q", res.ErrorCode, tool.ErrCodeInvalidArgs)
	}
	if res.Retryable {
		t.Fatal("Retryable = true, want false")
	}
	if !strings.Contains(res.Output, "task 1 (Task One) is already in_progress") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
}

func TestTaskTool_UpdateRejectsCompletingPendingTaskDirectly(t *testing.T) {
	tl := newTestTaskTool()

	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Task One"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 1","status":"completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != tool.ErrCodeInvalidArgs {
		t.Fatalf("ErrorCode = %q, want %q", res.ErrorCode, tool.ErrCodeInvalidArgs)
	}
	if !strings.Contains(res.Output, "must be in_progress before it can be marked completed") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
}

func TestTaskTool_UpdateRejectsCompletingAnotherTaskWhileOneIsInProgress(t *testing.T) {
	tl := newTestTaskTool()

	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Task One","status":"in_progress"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"create","title":"Task Two"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := tl.Execute(t.Context(), ectx("s2"), []byte(`{"action":"update","id":"task 2","status":"completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != tool.ErrCodeInvalidArgs {
		t.Fatalf("ErrorCode = %q, want %q", res.ErrorCode, tool.ErrCodeInvalidArgs)
	}
	if !strings.Contains(res.Output, "task 1 (Task One) is already in_progress") {
		t.Fatalf("unexpected output: %s", res.Output)
	}
}

func TestTaskTool_Delete(t *testing.T) {
	tl := newTestTaskTool()

	_, _ = tl.Execute(t.Context(), ectx("s3"), []byte(`{"action":"create","title":"Temporary task"}`))

	res, err := tl.Execute(t.Context(), ectx("s3"), []byte(`{"action":"delete","id":"task 1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "Deleted task task 1: Temporary task") {
		t.Fatalf("expected delete confirmation with title, got: %s", res.Output)
	}

	res, err = tl.Execute(t.Context(), ectx("s3"), []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "No tasks") {
		t.Fatalf("expected no tasks, got: %s", res.Output)
	}
}

func TestTaskTool_SessionIsolation(t *testing.T) {
	tl := newTestTaskTool()

	_, _ = tl.Execute(t.Context(), ectx("a"), []byte(`{"action":"create","title":"Task A"}`))

	res, err := tl.Execute(t.Context(), ectx("b"), []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "No tasks") {
		t.Fatalf("expected no tasks in session b, got: %s", res.Output)
	}
}

func TestTaskTool_Errors(t *testing.T) {
	tl := newTestTaskTool()

	tests := []struct {
		name string
		args string
		code string
	}{
		{"create without title", `{"action":"create"}`, "invalid_args"},
		{"update without id", `{"action":"update","status":"completed"}`, "invalid_args"},
		{"update non-existent", `{"action":"update","id":"task 99","status":"completed"}`, "not_found"},
		{"delete non-existent", `{"action":"delete","id":"task 99"}`, "not_found"},
		{"delete without id", `{"action":"delete"}`, "invalid_args"},
		{"unknown action", `{"action":"archive"}`, "invalid_args"},
		{"empty action", `{"action":""}`, "invalid_args"},
		{"invalid JSON", `not json`, "invalid_args"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tl.Execute(t.Context(), ectx("e"), []byte(tt.args))
			if err != nil {
				t.Fatalf("expected ErrorResult, got Go error: %v", err)
			}
			if res.ErrorCode != tt.code {
				t.Errorf("ErrorCode = %q, want %q (output: %s)", res.ErrorCode, tt.code, res.Output)
			}
		})
	}
}

func TestTaskTool_NotFoundIsNonRetryableAndListsAvailableTasks(t *testing.T) {
	tl := newTestTaskTool()

	if _, err := tl.Execute(t.Context(), ectx("e"), []byte(`{"action":"create","title":"Fix bug"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := tl.Execute(t.Context(), ectx("e"), []byte(`{"action":"update","id":"task_missing","status":"completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Retryable {
		t.Fatal("Retryable = true, want false")
	}
	if !strings.Contains(res.Output, `Available tasks: task 1 (Fix bug)`) {
		t.Fatalf("expected available task refs in error, got: %s", res.Output)
	}
}

func TestTaskStore_GetTasksReturnsSnapshot(t *testing.T) {
	store := tool.NewTaskStore(nil)
	tl := tool.NewTaskTool(store)

	if _, err := tl.Execute(t.Context(), ectx("snap"), []byte(`{"action":"create","title":"Keep state"}`)); err != nil {
		t.Fatal(err)
	}

	snapshot := store.GetTasks("snap")
	if len(snapshot) != 1 {
		t.Fatalf("GetTasks() len = %d, want 1", len(snapshot))
	}
	snapshot[0].Title = "Mutated outside store"
	snapshot[0].Status = "completed"

	fresh := store.GetTasks("snap")
	if fresh[0].Title != "Keep state" {
		t.Fatalf("fresh title = %q, want original task title", fresh[0].Title)
	}
	if fresh[0].Status != "pending" {
		t.Fatalf("fresh status = %q, want pending", fresh[0].Status)
	}
}

func TestTaskStore_EnsureActiveTaskPromotesFirstPending(t *testing.T) {
	store := tool.NewTaskStore(nil)
	tl := tool.NewTaskTool(store)

	if _, err := tl.Execute(t.Context(), ectx("promote"), []byte(`{"action":"create","title":"First task"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Execute(t.Context(), ectx("promote"), []byte(`{"action":"create","title":"Second task"}`)); err != nil {
		t.Fatal(err)
	}

	active, activated, err := store.EnsureActiveTask(t.Context(), "promote")
	if err != nil {
		t.Fatal(err)
	}
	if !activated {
		t.Fatal("activated = false, want true")
	}
	if active == nil || active.ID != "task 1" || active.Status != "in_progress" {
		t.Fatalf("active = %+v, want task 1 in_progress", active)
	}

	tasks := store.GetTasks("promote")
	if tasks[0].Status != "in_progress" {
		t.Fatalf("task 1 status = %q, want in_progress", tasks[0].Status)
	}
	if tasks[1].Status != "pending" {
		t.Fatalf("task 2 status = %q, want pending", tasks[1].Status)
	}
}

func TestTaskStore_EnsureActiveTaskKeepsExistingActive(t *testing.T) {
	store := tool.NewTaskStore(nil)
	tl := tool.NewTaskTool(store)

	if _, err := tl.Execute(t.Context(), ectx("active"), []byte(`{"action":"create","title":"First task","status":"in_progress"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Execute(t.Context(), ectx("active"), []byte(`{"action":"create","title":"Second task"}`)); err != nil {
		t.Fatal(err)
	}

	active, activated, err := store.EnsureActiveTask(t.Context(), "active")
	if err != nil {
		t.Fatal(err)
	}
	if activated {
		t.Fatal("activated = true, want false")
	}
	if active == nil || active.ID != "task 1" || active.Status != "in_progress" {
		t.Fatalf("active = %+v, want existing task 1", active)
	}
}

func TestTaskStore_CloneSessionCopiesTasks(t *testing.T) {
	store := tool.NewTaskStore(nil)
	tl := tool.NewTaskTool(store)

	if _, err := tl.Execute(t.Context(), ectx("parent"), []byte(`{"action":"create","title":"First task","notes":"Acceptance criteria:\n- keep definition"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Execute(t.Context(), ectx("parent"), []byte(`{"action":"create","title":"Second task","status":"in_progress"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Execute(t.Context(), ectx("parent"), []byte(`{"action":"update","id":"task 1","progress":"blocked on tests"}`)); err != nil {
		t.Fatal(err)
	}

	if err := store.CloneSession(t.Context(), "parent", "branch"); err != nil {
		t.Fatalf("CloneSession() error = %v", err)
	}

	parentTasks := store.GetTasks("parent")
	branchTasks := store.GetTasks("branch")
	if len(branchTasks) != len(parentTasks) {
		t.Fatalf("branch task count = %d, want %d", len(branchTasks), len(parentTasks))
	}
	if branchTasks[0].Title != "First task" || branchTasks[1].Status != "in_progress" {
		t.Fatalf("branch tasks = %#v, want cloned task state", branchTasks)
	}
	if branchTasks[0].Notes != "Acceptance criteria:\n- keep definition" {
		t.Fatalf("branch task notes = %q, want cloned durable notes", branchTasks[0].Notes)
	}
	if branchTasks[0].Progress != "blocked on tests" {
		t.Fatalf("branch task progress = %q, want cloned progress", branchTasks[0].Progress)
	}

	branchTasks[0].Title = "Changed only in snapshot"
	freshParent := store.GetTasks("parent")
	if freshParent[0].Title != "First task" {
		t.Fatalf("parent title = %q, want original title after branch snapshot mutation", freshParent[0].Title)
	}
}

func TestTaskStore_CreateTaskWithKindPersistsKind(t *testing.T) {
	store := tool.NewTaskStore(nil)
	task, created, err := store.CreateTaskWithKind(t.Context(), "kinds", "Compile report", tool.TaskKindReport, "pending", "Summarize all findings.")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	if task.Kind != tool.TaskKindReport {
		t.Fatalf("task.Kind = %q, want %q", task.Kind, tool.TaskKindReport)
	}

	got := store.GetTasks("kinds")
	if len(got) != 1 || got[0].Kind != tool.TaskKindReport {
		t.Fatalf("stored tasks = %#v, want report kind", got)
	}
}

func TestTaskTool_CreateReportsPersistenceFailure(t *testing.T) {
	store := tool.NewTaskStore(failingTaskRepo{createErr: errors.New("db down")})
	tl := tool.NewTaskTool(store)

	res, err := tl.Execute(t.Context(), ectx("persist-create"), []byte(`{"action":"create","title":"Write tests"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != tool.ErrCodeInternal {
		t.Fatalf("ErrorCode = %q, want %q", res.ErrorCode, tool.ErrCodeInternal)
	}
	if len(store.GetTasks("persist-create")) != 0 {
		t.Fatalf("expected create rollback, got tasks: %#v", store.GetTasks("persist-create"))
	}
}

func TestTaskTool_UpdateReportsPersistenceFailureAndReverts(t *testing.T) {
	store := tool.NewTaskStore(failingTaskRepo{updateErr: errors.New("db locked")})
	if _, _, err := store.CreateTaskWithKind(t.Context(), "persist-update", "Fix bug", tool.TaskKindImplementation, "in_progress", ""); err != nil {
		t.Fatal(err)
	}
	tl := tool.NewTaskTool(store)

	res, err := tl.Execute(t.Context(), ectx("persist-update"), []byte(`{"action":"update","id":"task 1","status":"completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != tool.ErrCodeInternal {
		t.Fatalf("ErrorCode = %q, want %q", res.ErrorCode, tool.ErrCodeInternal)
	}
	tasks := store.GetTasks("persist-update")
	if len(tasks) != 1 || tasks[0].Status != "in_progress" {
		t.Fatalf("expected update rollback to in_progress, got tasks: %#v", tasks)
	}
}

func TestTaskStore_EnsureActiveTaskRevertsOnPersistenceFailure(t *testing.T) {
	store := tool.NewTaskStore(failingTaskRepo{updateErr: errors.New("db locked")})
	if _, _, err := store.CreateTaskWithKind(t.Context(), "persist-active", "First task", tool.TaskKindImplementation, "pending", ""); err != nil {
		t.Fatal(err)
	}

	active, activated, err := store.EnsureActiveTask(t.Context(), "persist-active")
	if err == nil {
		t.Fatal("expected persistence error")
	}
	if active != nil || activated {
		t.Fatalf("unexpected activation result: active=%+v activated=%v", active, activated)
	}
	tasks := store.GetTasks("persist-active")
	if len(tasks) != 1 || tasks[0].Status != "pending" {
		t.Fatalf("expected rollback to pending, got tasks: %#v", tasks)
	}
}
