package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type stubTaskManager struct {
	listTasks   []TaskRecord
	createTask  TaskCreateResult
	updatedTask TaskRecord
}

func (s stubTaskManager) ListTasks() ([]TaskRecord, error) { return s.listTasks, nil }
func (s stubTaskManager) CreateTask(TaskCreateRequest) (TaskCreateResult, error) {
	return s.createTask, nil
}
func (s stubTaskManager) UpdateTaskProgress(TaskProgressUpdateRequest) (TaskRecord, error) {
	return s.updatedTask, nil
}

func TestTaskWorkflowToolExecuteCreateIncludesReminder(t *testing.T) {
	result, err := NewTaskWorkflowTool().Execute(context.Background(), ExecutionContext{
		TaskManager: stubTaskManager{
			createTask: TaskCreateResult{
				Task:    TaskRecord{TaskID: "task-child", ParentTaskID: "task-parent", Title: "Apply backend optimizations", Status: "pending"},
				Message: "created child task",
			},
		},
	}, json.RawMessage(`{"action":"create","title":"Apply backend optimizations","parent_task_id":"task-parent"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !containsAll(result.Output,
		`"task_id":"task-child"`,
		`"parent_task_id":"task-parent"`,
		`created child task`,
		`finish child tasks before finishing the parent`,
	) {
		t.Fatalf("output = %q", result.Output)
	}
}
func (s stubTaskManager) BlockTask(TaskBlockRequest) (TaskRecord, error) {
	return s.updatedTask, nil
}
func (s stubTaskManager) CompleteTask(TaskCompleteRequest) (TaskRecord, error) {
	return s.updatedTask, nil
}
func (s stubTaskManager) ReviewTask(TaskReviewRequest) (TaskRecord, error) {
	return s.updatedTask, nil
}

func TestTaskWorkflowToolExecuteListUsesTaskManager(t *testing.T) {
	result, err := NewTaskWorkflowTool().Execute(context.Background(), ExecutionContext{
		TaskManager: stubTaskManager{
			listTasks: []TaskRecord{{TaskID: "task-1", Title: "Implement", Status: "in_progress"}},
		},
	}, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output == "" || !containsAll(result.Output, `"task_id":"task-1"`, `"title":"Implement"`, `"active_task_id":"task-1"`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestTaskWorkflowToolExecuteListPrefersDeepestActiveTask(t *testing.T) {
	result, err := NewTaskWorkflowTool().Execute(context.Background(), ExecutionContext{
		TaskManager: stubTaskManager{
			listTasks: []TaskRecord{
				{TaskID: "task-parent", Title: "Epic", Status: "in_progress"},
				{TaskID: "task-child", ParentTaskID: "task-parent", Title: "Implement", Status: "in_progress"},
			},
		},
	}, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !containsAll(result.Output, `"active_task_id":"task-child"`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestTaskReviewToolExecuteReviewUsesTaskManager(t *testing.T) {
	result, err := NewTaskReviewTool().Execute(context.Background(), ExecutionContext{
		TaskManager: stubTaskManager{
			updatedTask: TaskRecord{TaskID: "task-1", ReviewStatus: "concern", ReviewSummary: "cache drift remains"},
		},
	}, json.RawMessage(`{"action":"review","task_id":"task-1","review_status":"concern","review_summary":"cache drift remains"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output == "" || !containsAll(result.Output, `"task_id":"task-1"`, `"review_status":"concern"`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestTaskWorkflowToolExecuteRequiresTaskManager(t *testing.T) {
	_, err := NewTaskWorkflowTool().Execute(context.Background(), ExecutionContext{}, json.RawMessage(`{"action":"list"}`))
	if !errors.Is(err, ErrTaskManagerRequired) {
		t.Fatalf("Execute() error = %v, want ErrTaskManagerRequired", err)
	}
}

func TestTaskWorkflowToolDefinitionMentionsParentTasksAndActivePath(t *testing.T) {
	definition := NewTaskWorkflowTool().Definition()
	combined := strings.Join([]string{
		definition.Description,
		definition.ProviderDescription,
		string(definition.InputSchema),
	}, "\n")
	if !containsAll(combined, "parent_task_id", "active task path", "summary", "child tasks", "completed parent") {
		t.Fatalf("task_workflow definition missing parent/path guidance: %q", combined)
	}
}

func TestTaskWorkflowToolExecuteUpdateReturnsTaskRecord(t *testing.T) {
	result, err := NewTaskWorkflowTool().Execute(context.Background(), ExecutionContext{
		TaskManager: stubTaskManager{
			updatedTask: TaskRecord{TaskID: "task-1", Title: "Apply backend optimizations", Status: "in_progress", Progress: "started"},
		},
	}, json.RawMessage(`{"action":"update","task_id":"task-1","status":"in_progress","progress":"started"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !containsAll(result.Output, `"task_id":"task-1"`) {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestParseTaskWorkflowInputAllowsMinimalCreatePayload(t *testing.T) {
	input, err := parseTaskWorkflowInput(json.RawMessage(`{"action":"create","title":"Add caching"}`))
	if err != nil {
		t.Fatalf("parseTaskWorkflowInput() error = %v", err)
	}
	if input.Action != taskActionCreate {
		t.Fatalf("input.Action = %q, want %q", input.Action, taskActionCreate)
	}
	if input.Title != "Add caching" {
		t.Fatalf("input.Title = %q, want %q", input.Title, "Add caching")
	}
	if input.Status != "pending" {
		t.Fatalf("input.Status = %q, want %q", input.Status, "pending")
	}
}

func TestParseTaskWorkflowInputNormalizesStatusBlockedToBlockAction(t *testing.T) {
	input, err := parseTaskWorkflowInput(json.RawMessage(`{"action":"update","task_id":"task-1","status":"blocked","notes":"waiting on API schema"}`))
	if err != nil {
		t.Fatalf("parseTaskWorkflowInput() error = %v", err)
	}
	if input.Action != taskActionBlock {
		t.Fatalf("input.Action = %q, want %q", input.Action, taskActionBlock)
	}
	if input.BlockReason != "waiting on API schema" {
		t.Fatalf("input.BlockReason = %q", input.BlockReason)
	}
	if input.Notes != "" || input.Status != "" {
		t.Fatalf("normalized input = %#v", input)
	}
}

func TestParseTaskWorkflowInputNormalizesStatusCompleteToCompleteAction(t *testing.T) {
	input, err := parseTaskWorkflowInput(json.RawMessage(`{"action":"update","task_id":"task-1","status":"complete","progress":"done"}`))
	if err != nil {
		t.Fatalf("parseTaskWorkflowInput() error = %v", err)
	}
	if input.Action != taskActionComplete {
		t.Fatalf("input.Action = %q, want %q", input.Action, taskActionComplete)
	}
	if input.TaskID != "task-1" {
		t.Fatalf("input.TaskID = %q", input.TaskID)
	}
	if input.Summary != "done" {
		t.Fatalf("input.Summary = %q, want progress text", input.Summary)
	}
	if input.Progress != "" || input.Notes != "" || input.Status != "" {
		t.Fatalf("normalized input = %#v", input)
	}
}

func TestParseTaskWorkflowInputNormalizesStatusCompletedToCompleteAction(t *testing.T) {
	input, err := parseTaskWorkflowInput(json.RawMessage(`{"action":"update","task_id":"task-1","status":"completed","notes":"wrapped up"}`))
	if err != nil {
		t.Fatalf("parseTaskWorkflowInput() error = %v", err)
	}
	if input.Action != taskActionComplete {
		t.Fatalf("input.Action = %q, want %q", input.Action, taskActionComplete)
	}
	if input.Summary != "wrapped up" {
		t.Fatalf("input.Summary = %q", input.Summary)
	}
}

func TestParseTaskWorkflowInputRejectsInvalidAction(t *testing.T) {
	_, err := parseTaskWorkflowInput(json.RawMessage(`{"action":"review","task_id":"task-1","title":null,"kind":null,"status":null,"notes":null,"progress":null,"block_reason":null,"summary":null}`))
	if !errors.Is(err, ErrTaskWorkflowActionInvalid) {
		t.Fatalf("parseTaskWorkflowInput() error = %v, want ErrTaskWorkflowActionInvalid", err)
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
}

func TestParseTaskWorkflowInputRejectsCreateFieldsThatWouldBeIgnored(t *testing.T) {
	_, err := parseTaskWorkflowInput(json.RawMessage(`{"action":"create","title":"Add caching","summary":"done"}`))
	if !errors.Is(err, ErrTaskWorkflowFieldUnsupported) {
		t.Fatalf("parseTaskWorkflowInput() error = %v, want ErrTaskWorkflowFieldUnsupported", err)
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Fatalf("parseTaskWorkflowInput() error = %q, want unsupported field name", err.Error())
	}
}

func TestParseTaskReviewInputRejectsInvalidAction(t *testing.T) {
	_, err := parseTaskReviewInput(json.RawMessage(`{"action":"complete","task_id":"task-1","review_status":null,"review_summary":null}`))
	if !errors.Is(err, ErrTaskReviewActionInvalid) {
		t.Fatalf("parseTaskReviewInput() error = %v, want ErrTaskReviewActionInvalid", err)
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
}

func TestParseTaskReviewInputRejectsListFieldsThatWouldBeIgnored(t *testing.T) {
	_, err := parseTaskReviewInput(json.RawMessage(`{"action":"list","task_id":"task-1"}`))
	if !errors.Is(err, ErrTaskReviewFieldUnsupported) {
		t.Fatalf("parseTaskReviewInput() error = %v, want ErrTaskReviewFieldUnsupported", err)
	}
	if !strings.Contains(err.Error(), "task_id") {
		t.Fatalf("parseTaskReviewInput() error = %q, want unsupported field name", err.Error())
	}
}

func TestParseTaskWorkflowInputWrapsMissingTitleWithHelpfulContractMessage(t *testing.T) {
	_, err := parseTaskWorkflowInput(json.RawMessage(`{"action":"create"}`))
	if !errors.Is(err, ErrTaskInputTitleRequired) {
		t.Fatalf("parseTaskWorkflowInput() error = %v, want ErrTaskInputTitleRequired", err)
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	if got := err.Error(); !containsAll(got,
		"`task_workflow` failed.",
		`Use action "list", "create", "update", "block", or "complete"`,
		"title is required",
	) {
		t.Fatalf("err.Error() = %q", got)
	}
}

func TestParseTaskWorkflowInputRequiresSummaryForComplete(t *testing.T) {
	_, err := parseTaskWorkflowInput(json.RawMessage(`{"action":"complete","task_id":"task-1"}`))
	if !errors.Is(err, ErrTaskSummaryRequired) {
		t.Fatalf("parseTaskWorkflowInput() error = %v, want ErrTaskSummaryRequired", err)
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
}

func TestParseTaskReviewInputAllowsMinimalListPayload(t *testing.T) {
	input, err := parseTaskReviewInput(json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("parseTaskReviewInput() error = %v", err)
	}
	if input.Action != taskActionList {
		t.Fatalf("input.Action = %q, want %q", input.Action, taskActionList)
	}
}

func TestParseTaskReviewInputWrapsMissingSummaryWithHelpfulContractMessage(t *testing.T) {
	_, err := parseTaskReviewInput(json.RawMessage(`{"action":"review","task_id":"task-1","review_status":"pass","review_summary":null}`))
	if !errors.Is(err, ErrTaskReviewSummaryNeeded) {
		t.Fatalf("parseTaskReviewInput() error = %v, want ErrTaskReviewSummaryNeeded", err)
	}
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("errors.Is(err, ErrInvalidArguments) = false, err = %v", err)
	}
	if got := err.Error(); !containsAll(got,
		"`task_review` failed.",
		`Use action "list" or "review"`,
		"review_summary is required",
	) {
		t.Fatalf("err.Error() = %q", got)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
