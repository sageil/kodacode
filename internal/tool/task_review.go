package tool

import (
	"context"
	"encoding/json"
)

const TaskReviewToolName = "task_review"

type TaskReviewTool struct{}

func NewTaskReviewTool() TaskReviewTool {
	return TaskReviewTool{}
}

func (TaskReviewTool) Definition() Definition {
	return Definition{
		Name:                TaskReviewToolName,
		Description:         "Inspect durable tasks and record review outcomes. Use list to inspect tasks and review to record a pass, concern, fail, or accepted outcome for an existing task. Send only the fields relevant to the chosen action. This tool does not change workflow status directly.",
		ProviderDescription: "List tasks or record a review outcome for an existing task.",
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["list","review"],"description":"Review task operation to perform."},"task_id":{"type":["string","null"],"description":"Existing task identifier for review. Omit or use null for list."},"review_status":{"type":["string","null"],"enum":["pass","concern","fail","accepted",null],"description":"Required review outcome for review. Omit or use null for list."},"review_summary":{"type":["string","null"],"description":"Required short review summary for review. Omit or use null for list."}},"required":["action"],"additionalProperties":false}`),
		ArgumentExamples: []string{
			`{"action":"list"}`,
			`{"action":"review","task_id":"task-1","review_status":"pass","review_summary":"Verified the changes."}`,
		},
	}
}

func (TaskReviewTool) Execute(_ context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	manager, err := ectx.Tasks()
	if err != nil {
		return Result{}, err
	}
	input, err := parseTaskReviewInput(args)
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
	case taskActionReview:
		task, err := manager.ReviewTask(TaskReviewRequest{
			TaskID:        input.TaskID,
			ReviewStatus:  input.ReviewStatus,
			ReviewSummary: input.ReviewSummary,
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Output: taskRecordOutput(task)}, nil
	default:
		return Result{}, ErrTaskReviewActionInvalid
	}
}

func (TaskReviewTool) NormalizedInputKey(args json.RawMessage) (string, error) {
	input, err := parseTaskReviewInput(args)
	if err != nil {
		return canonicalToolArgsKey(args)
	}
	key, err := json.Marshal(struct {
		Action        string `json:"action"`
		TaskID        string `json:"task_id"`
		ReviewStatus  string `json:"review_status"`
		ReviewSummary string `json:"review_summary"`
	}{
		Action:        input.Action,
		TaskID:        input.TaskID,
		ReviewStatus:  input.ReviewStatus,
		ReviewSummary: input.ReviewSummary,
	})
	if err != nil {
		return canonicalToolArgsKey(args)
	}
	return string(key), nil
}
