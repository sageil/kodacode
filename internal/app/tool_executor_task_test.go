package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestToolExecutorExecuteTaskWorkflowToolPersistsDurableTasks(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskWorkflowTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	createArgs, err := json.Marshal(map[string]any{
		"action":         "create",
		"task_id":        nil,
		"title":          "Investigate duplicate reads",
		"kind":           "analysis",
		"status":         nil,
		"notes":          "focus on reuse",
		"progress":       nil,
		"block_reason":   nil,
		"summary":        nil,
		"review_status":  nil,
		"review_summary": nil,
	})
	if err != nil {
		t.Fatalf("json.Marshal(create) error = %v", err)
	}
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  createArgs,
	})
	if err != nil {
		t.Fatalf("Execute(create) error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed", result)
	}

	listArgs := json.RawMessage(`{"action":"list","task_id":null,"title":null,"kind":null,"status":null,"notes":null,"progress":null,"block_reason":null,"summary":null,"review_status":null,"review_summary":null}`)
	listResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  listArgs,
	})
	if err != nil {
		t.Fatalf("Execute(list) error = %v", err)
	}
	if listResult.Status != ToolExecutionStatusExecuted || listResult.Output == "" {
		t.Fatalf("list result = %#v", listResult)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.TaskOrder) != 1 {
		t.Fatalf("task order = %#v", state.TaskOrder)
	}
	taskState := state.Tasks[state.TaskOrder[0]]
	if taskState == nil || taskState.Title != "Investigate duplicate reads" {
		t.Fatalf("task state = %#v", taskState)
	}
}

func TestToolExecutorTaskWorkflowCreateChildKeepsActiveParentInProgress(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskWorkflowTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	parent, err := sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    "task-parent",
		Title:     "Project review",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}

	createArgs := json.RawMessage(`{"action":"create","task_id":"task-child","parent_task_id":"task-parent","title":"Apply backend optimizations"}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  createArgs,
	})
	if err != nil {
		t.Fatalf("Execute(create child) error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed", result)
	}
	if !strings.Contains(result.Output, `finish child tasks before finishing the parent`) {
		t.Fatalf("result output = %q", result.Output)
	}
	if !strings.Contains(result.Output, `"parent_task_id":"task-parent"`) {
		t.Fatalf("result output = %q", result.Output)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Tasks[parent.TaskID].Status; got != events.TaskStatusInProgress {
		t.Fatalf("parent status = %q, want in_progress", got)
	}
}

func TestToolExecutorTaskWorkflowRejectsChildUnderCompletedParent(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskWorkflowTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	parent, err := sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    "task-parent",
		Title:     "Project review",
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	if _, err := sessions.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    parent.TaskID,
		Progress:  "review complete",
	}); err != nil {
		t.Fatalf("UpdateTaskProgress(parent) error = %v", err)
	}
	if _, err := sessions.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    parent.TaskID,
		Summary:   "review complete",
	}); err != nil {
		t.Fatalf("CompleteTask(parent) error = %v", err)
	}

	createArgs := json.RawMessage(`{"action":"create","task_id":"task-child","parent_task_id":"task-parent","title":"Apply backend optimizations"}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  createArgs,
	})
	if err != nil {
		t.Fatalf("Execute(create child) transport error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed", result)
	}
	if result.Error != "task_workflow failed: parent_task_id is already complete. Use a pending parent or omit parent_task_id." {
		t.Fatalf("result error = %q", result.Error)
	}
}

func TestToolExecutorTaskReviewNotFoundReturnsListRecovery(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskReviewTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-review",
		ToolName:   tool.TaskReviewToolName,
		Arguments:  json.RawMessage(`{"action":"review","task_id":"task-33","review_status":"pass","review_summary":"Verified."}`),
	})
	if err != nil {
		t.Fatalf("Execute(review) transport error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed result with tool error", result)
	}
	for _, want := range []string{
		`task_review failed`,
		`task_id task-33 was not found`,
		`returned task_id`,
	} {
		if !strings.Contains(result.Error, want) {
			t.Fatalf("result error = %q, missing %q", result.Error, want)
		}
	}
}

func TestToolExecutorTaskWorkflowUnsupportedFieldsReturnActionPayload(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskWorkflowTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-workflow",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  json.RawMessage(`{"action":"create","title":"Implement fix","summary":"done"}`),
	})
	if err != nil {
		t.Fatalf("Execute(workflow) transport error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed result with tool error", result)
	}
	for _, want := range []string{
		`task_workflow failed`,
		`create does not accept summary`,
		`Remove unsupported fields`,
	} {
		if !strings.Contains(result.Error, want) {
			t.Fatalf("result error = %q, missing %q", result.Error, want)
		}
	}
}

func TestToolExecutorTaskReviewUnsupportedFieldsReturnActionPayload(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskReviewTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-review",
		ToolName:   tool.TaskReviewToolName,
		Arguments:  json.RawMessage(`{"action":"review","task_id":"task-1","review_status":"pass","review_summary":"ok","title":"Implement fix"}`),
	})
	if err != nil {
		t.Fatalf("Execute(review) transport error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed result with tool error", result)
	}
	for _, want := range []string{
		`task_review failed`,
		`review accepts only action, task_id, review_status, and review_summary`,
		`Remove title`,
	} {
		if !strings.Contains(result.Error, want) {
			t.Fatalf("result error = %q, missing %q", result.Error, want)
		}
	}
}

func TestToolExecutorTaskWorkflowLeavesSolePendingTaskPendingAfterComplete(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskWorkflowTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	firstCreate := json.RawMessage(`{"action":"create","task_id":"task-a","title":"Audit middleware","kind":"analysis","status":"in_progress","notes":null,"progress":null,"block_reason":null,"summary":null}`)
	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  firstCreate,
	}); err != nil {
		t.Fatalf("Execute(create first) error = %v", err)
	}

	secondCreate := json.RawMessage(`{"action":"create","task_id":"task-b","title":"Refine middleware","kind":"implementation","status":null,"notes":null,"progress":null,"block_reason":null,"summary":null}`)
	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  secondCreate,
	}); err != nil {
		t.Fatalf("Execute(create second) error = %v", err)
	}
	updateArgs := json.RawMessage(`{"action":"update","task_id":"task-a","title":null,"kind":null,"status":"in_progress","notes":null,"progress":"audit started","block_reason":null,"summary":null}`)
	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2b",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  updateArgs,
	}); err != nil {
		t.Fatalf("Execute(update first) error = %v", err)
	}

	completeArgs := json.RawMessage(`{"action":"complete","task_id":"task-a","title":null,"kind":null,"status":null,"notes":null,"progress":null,"block_reason":null,"summary":"audit complete"}`)
	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-3",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  completeArgs,
	}); err != nil {
		t.Fatalf("Execute(complete) error = %v", err)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Tasks["task-a"].Status; got != events.TaskStatusCompleted {
		t.Fatalf("task-a status = %q", got)
	}
	if got := state.Tasks["task-b"].Status; got != events.TaskStatusPending {
		t.Fatalf("task-b status = %q, want pending", got)
	}
}

func TestToolExecutorTaskWorkflowRejectsSecondInProgressTask(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskWorkflowTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	firstCreate := json.RawMessage(`{"action":"create","task_id":"task-a","title":"Audit middleware","kind":"analysis","status":"in_progress","notes":null,"progress":null,"block_reason":null,"summary":null}`)
	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  firstCreate,
	}); err != nil {
		t.Fatalf("Execute(create first) error = %v", err)
	}

	secondCreate := json.RawMessage(`{"action":"create","task_id":"task-b","title":"Refine middleware","kind":"implementation","status":null,"notes":null,"progress":null,"block_reason":null,"summary":null}`)
	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  secondCreate,
	}); err != nil {
		t.Fatalf("Execute(create second) error = %v", err)
	}

	updateArgs := json.RawMessage(`{"action":"update","task_id":"task-b","title":null,"kind":null,"status":"in_progress","notes":null,"progress":"starting now","block_reason":null,"summary":null}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-3",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  updateArgs,
	})
	if err != nil {
		t.Fatalf("Execute(update) transport error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed result with tool error", result)
	}
	if result.Error != "task_workflow failed: task-a is already in progress. Update, block, or complete it first." {
		t.Fatalf("result error = %q", result.Error)
	}
}

func TestToolExecutorTaskWorkflowMarksRepeatedFailureAsRetry(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskWorkflowTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	firstCreate := json.RawMessage(`{"action":"create","task_id":"task-a","title":"Audit middleware","kind":"analysis","status":"in_progress","notes":null,"progress":null,"block_reason":null,"summary":null}`)
	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  firstCreate,
	}); err != nil {
		t.Fatalf("Execute(create first) error = %v", err)
	}

	secondCreate := json.RawMessage(`{"action":"create","task_id":"task-b","title":"Refine middleware","kind":"implementation","status":null,"notes":null,"progress":null,"block_reason":null,"summary":null}`)
	if _, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-2",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  secondCreate,
	}); err != nil {
		t.Fatalf("Execute(create second) error = %v", err)
	}

	updateArgs := json.RawMessage(`{"action":"update","task_id":"task-b","title":null,"kind":null,"status":"in_progress","notes":null,"progress":"starting now","block_reason":null,"summary":null}`)
	firstFailure, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-3",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  updateArgs,
	})
	if err != nil {
		t.Fatalf("Execute(first failure) transport error = %v", err)
	}
	if firstFailure.Status != ToolExecutionStatusExecuted || firstFailure.Error == "" {
		t.Fatalf("first failure = %#v", firstFailure)
	}

	secondFailure, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-4",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  updateArgs,
	})
	if err != nil {
		t.Fatalf("Execute(second failure) transport error = %v", err)
	}
	if secondFailure.Status != ToolExecutionStatusExecuted || secondFailure.Error == "" {
		t.Fatalf("second failure = %#v", secondFailure)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	call := state.Turns["turn-1"].ToolCalls["call-4"]
	if call == nil || call.RetryOfCallID != "call-3" {
		t.Fatalf("call-4 state = %#v", call)
	}
}

func TestToolExecutorTaskReviewUsesDelegatedParentTaskScope(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskReviewTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	for _, input := range []CreateSessionInput{
		{SessionID: "session-parent", WorkspaceRoot: root},
		{SessionID: "session-child", WorkspaceRoot: root},
	} {
		if _, err := sessions.CreateSession(context.Background(), input); err != nil {
			t.Fatalf("CreateSession(%s) error = %v", input.SessionID, err)
		}
	}

	taskState, err := sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-parent",
		TurnID:    "turn-setup",
		TaskID:    "task-1",
		Title:     "Apply Performance Enhancements",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	if _, err := sessions.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: "session-parent",
		TurnID:    "turn-setup",
		TaskID:    taskState.TaskID,
		Progress:  "Implementation in progress.",
	}); err != nil {
		t.Fatalf("UpdateTaskProgress(parent) error = %v", err)
	}
	taskState, err = sessions.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: "session-parent",
		TurnID:    "turn-setup",
		TaskID:    taskState.TaskID,
		Summary:   "Implementation finished.",
	})
	if err != nil {
		t.Fatalf("CompleteTask(parent) error = %v", err)
	}

	appendDelegatedHandoffForToolReuseTest(t, sessions, events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-parent",
		ParentTurnID:    "turn-parent",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-child",
		ChildTurnID:     "turn-child",
		ChildAgentID:    "reviewer",
		Task:            "Review the implementation changes",
		ContextSummary:  "Inspect the parent work and record a review outcome.",
		Model:           "openai/gpt-5",
	})

	listResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-child",
		TurnID:     "turn-child",
		ToolCallID: "call-list",
		ToolName:   tool.TaskReviewToolName,
		Arguments:  json.RawMessage(`{"action":"list"}`),
	})
	if err != nil {
		t.Fatalf("Execute(list) error = %v", err)
	}
	if listResult.Status != ToolExecutionStatusExecuted || !strings.Contains(listResult.Output, `"task_id":"task-1"`) || !strings.Contains(listResult.Output, taskState.Title) {
		t.Fatalf("list result = %#v", listResult)
	}

	reviewResult, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-child",
		TurnID:     "turn-child",
		ToolCallID: "call-review",
		ToolName:   tool.TaskReviewToolName,
		Arguments:  json.RawMessage(`{"action":"review","task_id":"task-1","review_status":"pass","review_summary":"Verified the performance changes."}`),
	})
	if err != nil {
		t.Fatalf("Execute(review) error = %v", err)
	}
	if reviewResult.Status != ToolExecutionStatusExecuted || !strings.Contains(reviewResult.Output, `"review_status":"pass"`) {
		t.Fatalf("review result = %#v", reviewResult)
	}

	parentState, err := sessions.Snapshot(context.Background(), "session-parent")
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTask := parentState.Tasks["task-1"]
	if parentTask == nil || parentTask.ReviewStatus != "pass" || parentTask.ReviewSummary != "Verified the performance changes." {
		t.Fatalf("parent task = %#v", parentTask)
	}

	childState, err := sessions.Snapshot(context.Background(), "session-child")
	if err != nil {
		t.Fatalf("Snapshot(child) error = %v", err)
	}
	if len(childState.TaskOrder) != 0 {
		t.Fatalf("child task order = %#v, want no child-owned tasks", childState.TaskOrder)
	}
}

func TestToolExecutorTaskWorkflowRejectsCompletionWithoutRecordedWork(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewTaskWorkflowTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	task, err := sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: "session-1",
		TurnID:    "turn-setup",
		TaskID:    "task-a",
		Title:     "Audit middleware",
		Kind:      "analysis",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeTurnConfigured,
		Payload: newTurnConfiguredPayload(
			TurnCapabilities{
				AgentID:      "engineer",
				ModelRoute:   baseModelRoute(),
				AllowedTools: []string{tool.TaskWorkflowToolName},
			},
			nil,
			false,
			false,
			"",
			ResponseStyleDefault,
			false,
		),
	}); err != nil {
		t.Fatalf("append(turn_configured) error = %v", err)
	}

	completeArgs := json.RawMessage(`{"action":"complete","task_id":"task-a","title":null,"kind":null,"status":null,"notes":null,"progress":null,"block_reason":null,"summary":"audit complete"}`)
	result, err := executor.Execute(context.Background(), ExecuteToolInput{
		SessionID:  "session-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
		ToolName:   tool.TaskWorkflowToolName,
		Arguments:  completeArgs,
	})
	if err != nil {
		t.Fatalf("Execute(complete) transport error = %v", err)
	}
	if result.Status != ToolExecutionStatusExecuted {
		t.Fatalf("result = %#v, want executed", result)
	}
	if result.Error != "" {
		t.Fatalf("result error = %q", result.Error)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Tasks[task.TaskID].Status; got != events.TaskStatusCompleted {
		t.Fatalf("task status = %q, want completed", got)
	}
}
