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
		Description:         "Manage workflow tasks. Actions: list; create with title; update with task_id/status/progress/notes; block with task_id/block_reason; complete with task_id/summary. Use parent_task_id only when creating child tasks. Keep one active task path: a parent and one child may both be in_progress, but unrelated branches may not. Use task_review for review outcomes.",
		ProviderDescription: "Manage workflow tasks. Actions: list; create with title; update with task_id/status/progress/notes; block with task_id/block_reason; complete with task_id/summary. Keep one active task path.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["list","create","update","block","complete"],"description":"list, create, update, block, or complete."},"task_id":{"type":["string","null"],"description":"Task id for update, block, or complete. Optional custom id for create."},"parent_task_id":{"type":["string","null"],"description":"Parent id for create."},"title":{"type":["string","null"],"description":"Required for create."},"kind":{"type":["string","null"],"description":"Optional task kind."},"status":{"type":["string","null"],"description":"pending or in_progress. update also accepts blocked, complete, or completed."},"notes":{"type":["string","null"],"description":"Task notes."},"progress":{"type":["string","null"],"description":"Progress update."},"block_reason":{"type":["string","null"],"description":"Required for block."},"summary":{"type":["string","null"],"description":"Required for complete."}},"required":["action"],"additionalProperties":false}`),
		ArgumentExamples: []string{
			`{"action":"list"}`,
			`{"action":"create","title":"Add caching"}`,
			`{"action":"update","task_id":"task-1","status":"in_progress","progress":"started"}`,
			`{"action":"complete","task_id":"task-1","summary":"done"}`,
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
	if input.needsExistingTaskID() && input.TaskID == "" {
		tasks, err := manager.ListTasks()
		if err != nil {
			return Result{}, err
		}
		if activeID := activeTaskID(tasks); activeID != "" {
			input.TaskID = activeID
		} else {
			return Result{}, InvalidArguments(TaskWorkflowToolName, ErrTaskInputIDRequired)
		}
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

func (input taskWorkflowInput) needsExistingTaskID() bool {
	switch input.Action {
	case taskActionUpdate, taskActionBlock, taskActionComplete:
		return true
	default:
		return false
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
