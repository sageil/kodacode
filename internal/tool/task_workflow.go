package tool

import (
	"context"
	"encoding/json"
)

const TaskWorkflowToolName = "task_workflow"

type TaskWorkflowTool struct{}

func NewTaskWorkflowTool() TaskWorkflowTool {
	return TaskWorkflowTool{}
}

func (TaskWorkflowTool) Definition() Definition {
	return Definition{
		Name:                TaskWorkflowToolName,
		Description:         "Manage saved task workflow state. Use list to inspect tasks, create to add a task, update to change status or record progress, block to record a blocker, and complete to finish a task. Use parent_task_id when creating child tasks. Keep one active task path: a parent task may stay in_progress while child tasks are open, and one child task may be the current concrete step. Do not leave unrelated task branches in_progress at the same time. Do not create child tasks under a completed parent. Use action complete to finish a task, or send update with status complete or completed and it will be treated as complete. Use action block, or send update with status blocked and a reason, to block a task. This tool does not record review outcomes.",
		ProviderDescription: "List tasks or create, update, block, or complete a task. Use parent_task_id for child tasks. Keep one active task path. A parent may stay in_progress while one child task is the current step. Do not leave unrelated task branches in_progress. Do not create child tasks under a completed parent.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["list","create","update","block","complete"],"description":"Workflow task operation to perform."},"task_id":{"type":["string","null"],"description":"Optional caller-provided task identifier for create. Existing task identifier for update, block, or complete. Omit or use null for list."},"parent_task_id":{"type":["string","null"],"description":"Optional parent task id for create. Use this when creating a child task under an existing task. A completed parent task must not receive new children. Omit or use null for top-level tasks."},"title":{"type":["string","null"],"description":"Short task title for create. Omit or use null for other actions."},"kind":{"type":["string","null"],"description":"Optional task kind such as implementation, analysis, report, or epic."},"status":{"type":["string","null"],"description":"Optional workflow status for create or update. Use pending or in_progress when setting active state. On update, blocked is normalized to action block, and complete or completed is normalized to action complete."},"notes":{"type":["string","null"],"description":"Optional persistent task notes for create, update, or block."},"progress":{"type":["string","null"],"description":"Optional short progress update for update."},"block_reason":{"type":["string","null"],"description":"Required blocker reason for block. Omit or use null for other actions."},"summary":{"type":["string","null"],"description":"Required completion summary for complete. Parent tasks complete after child tasks are completed."}},"required":["action"],"additionalProperties":false}`),
		ArgumentExamples: []string{
			`{"action":"create","title":"Add caching"}`,
			`{"action":"create","title":"Add invalidation","parent_task_id":"task-1"}`,
			`{"action":"update","task_id":"task-1","status":"blocked","notes":"waiting on API schema"}`,
			`{"action":"complete","task_id":"task-1","summary":"Verified the cache changes and wrapped up the task."}`,
		},
	}
}

func (TaskWorkflowTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	manager, err := ectx.Tasks()
	if err != nil {
		return Result{}, err
	}
	input, err := parseTaskWorkflowInput(args)
	if err != nil {
		return Result{}, err
	}

	switch input.Action {
	case taskActionList:
		tasks, err := manager.ListTasks()
		if err != nil {
			return Result{}, err
		}
		return Result{Output: taskListOutput(tasks)}, nil
	case taskActionCreate:
		result, err := manager.CreateTask(TaskCreateRequest{
			TaskID:       input.TaskID,
			ParentTaskID: input.ParentTaskID,
			Title:        input.Title,
			Kind:         input.Kind,
			Status:       input.Status,
			Notes:        input.Notes,
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Output: taskCreateOutput(result)}, nil
	case taskActionUpdate:
		task, err := manager.UpdateTaskProgress(TaskProgressUpdateRequest{
			TaskID:   input.TaskID,
			Status:   input.Status,
			Progress: input.Progress,
			Notes:    input.Notes,
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Output: taskRecordMessageOutput(task, "")}, nil
	case taskActionBlock:
		task, err := manager.BlockTask(TaskBlockRequest{
			TaskID:      input.TaskID,
			BlockReason: input.BlockReason,
			Notes:       input.Notes,
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Output: taskRecordOutput(task)}, nil
	case taskActionComplete:
		task, err := manager.CompleteTask(TaskCompleteRequest{
			TaskID:  input.TaskID,
			Summary: input.Summary,
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Output: taskRecordOutput(task)}, nil
	default:
		return Result{}, ErrTaskWorkflowActionInvalid
	}
}

func (TaskWorkflowTool) NormalizedInputKey(args json.RawMessage) (string, error) {
	input, err := parseTaskWorkflowInput(args)
	if err != nil {
		return canonicalToolArgsKey(args)
	}
	key, err := json.Marshal(struct {
		Action       string `json:"action"`
		TaskID       string `json:"task_id"`
		ParentTaskID string `json:"parent_task_id"`
		Title        string `json:"title"`
		Kind         string `json:"kind"`
		Status       string `json:"status"`
		Notes        string `json:"notes"`
		Progress     string `json:"progress"`
		BlockReason  string `json:"block_reason"`
		Summary      string `json:"summary"`
	}{
		Action:       input.Action,
		TaskID:       input.TaskID,
		ParentTaskID: input.ParentTaskID,
		Title:        input.Title,
		Kind:         input.Kind,
		Status:       input.Status,
		Notes:        input.Notes,
		Progress:     input.Progress,
		BlockReason:  input.BlockReason,
		Summary:      input.Summary,
	})
	if err != nil {
		return canonicalToolArgsKey(args)
	}
	return string(key), nil
}
