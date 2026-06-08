package tool

import "errors"

var ErrTaskManagerRequired = errors.New("task manager is required")

type TaskRecord struct {
	TaskID          string `json:"task_id"`
	ParentTaskID    string `json:"parent_task_id,omitempty"`
	WorkflowID      string `json:"workflow_id,omitempty"`
	WorkflowPhaseID string `json:"workflow_phase_id,omitempty"`
	Title           string `json:"title"`
	Kind            string `json:"kind,omitempty"`
	Status          string `json:"status"`
	Notes           string `json:"notes,omitempty"`
	Progress        string `json:"progress,omitempty"`
	BlockReason     string `json:"block_reason,omitempty"`
	ReviewStatus    string `json:"review_status,omitempty"`
	ReviewSummary   string `json:"review_summary,omitempty"`
}

type TaskCreateRequest struct {
	TaskID       string
	ParentTaskID string
	Title        string
	Kind         string
	Status       string
	Notes        string
}

type TaskCreateResult struct {
	Task       TaskRecord  `json:"task"`
	ParentTask *TaskRecord `json:"parent_task,omitempty"`
	Message    string      `json:"message,omitempty"`
}

type TaskProgressUpdateRequest struct {
	TaskID   string
	Status   string
	Progress string
	Notes    string
}

type TaskBlockRequest struct {
	TaskID      string
	BlockReason string
	Notes       string
}

type TaskCompleteRequest struct {
	TaskID  string
	Summary string
}

type TaskReviewRequest struct {
	TaskID        string
	ReviewStatus  string
	ReviewSummary string
}

type TaskManager interface {
	ListTasks() ([]TaskRecord, error)
	CreateTask(TaskCreateRequest) (TaskCreateResult, error)
	UpdateTaskProgress(TaskProgressUpdateRequest) (TaskRecord, error)
	BlockTask(TaskBlockRequest) (TaskRecord, error)
	CompleteTask(TaskCompleteRequest) (TaskRecord, error)
	ReviewTask(TaskReviewRequest) (TaskRecord, error)
}

func (e ExecutionContext) Tasks() (TaskManager, error) {
	if e.TaskManager == nil {
		return nil, ErrTaskManagerRequired
	}
	return e.TaskManager, nil
}
