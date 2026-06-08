package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func (e *ToolExecutor) toolTaskManager(ctx context.Context, input ExecuteToolInput) tool.TaskManager {
	return sessionTaskManager{
		ctx:      ctx,
		sessions: e.sessions,
		input:    input,
	}
}

type sessionTaskManager struct {
	ctx      context.Context
	sessions *SessionService
	input    ExecuteToolInput
}

func (m sessionTaskManager) ListTasks() ([]tool.TaskRecord, error) {
	scope, err := m.taskScope()
	if err != nil {
		return nil, err
	}
	tasks, err := m.sessions.ListTasks(m.ctx, scope.SessionID)
	if err != nil {
		return nil, err
	}
	return mapTaskRecords(tasks), nil
}

func (m sessionTaskManager) CreateTask(request tool.TaskCreateRequest) (tool.TaskCreateResult, error) {
	outcome, err := m.sessions.CreateTaskDetailed(m.ctx, CreateTaskInput{
		SessionID:    m.input.SessionID,
		TurnID:       m.input.TurnID,
		TaskID:       request.TaskID,
		ParentTaskID: request.ParentTaskID,
		Title:        request.Title,
		Kind:         request.Kind,
		Status:       request.Status,
		Notes:        request.Notes,
	})
	if err != nil {
		return tool.TaskCreateResult{}, err
	}
	return tool.TaskCreateResult{
		Task: mapTaskRecord(outcome.Task),
	}, nil
}

func (m sessionTaskManager) UpdateTaskProgress(request tool.TaskProgressUpdateRequest) (tool.TaskRecord, error) {
	task, err := m.sessions.UpdateTaskProgress(m.ctx, UpdateTaskProgressInput{
		SessionID: m.input.SessionID,
		TurnID:    m.input.TurnID,
		TaskID:    request.TaskID,
		Status:    request.Status,
		Progress:  request.Progress,
		Notes:     request.Notes,
	})
	if err != nil {
		return tool.TaskRecord{}, err
	}
	return mapTaskRecord(task), nil
}

func (m sessionTaskManager) BlockTask(request tool.TaskBlockRequest) (tool.TaskRecord, error) {
	task, err := m.sessions.BlockTask(m.ctx, BlockTaskInput{
		SessionID:   m.input.SessionID,
		TurnID:      m.input.TurnID,
		TaskID:      request.TaskID,
		BlockReason: request.BlockReason,
		Notes:       request.Notes,
	})
	if err != nil {
		return tool.TaskRecord{}, err
	}
	return mapTaskRecord(task), nil
}

func (m sessionTaskManager) CompleteTask(request tool.TaskCompleteRequest) (tool.TaskRecord, error) {
	task, err := m.sessions.CompleteTask(m.ctx, CompleteTaskInput{
		SessionID: m.input.SessionID,
		TurnID:    m.input.TurnID,
		TaskID:    request.TaskID,
		Summary:   request.Summary,
	})
	if err != nil {
		return tool.TaskRecord{}, err
	}
	return mapTaskRecord(task), nil
}

func (m sessionTaskManager) ReviewTask(request tool.TaskReviewRequest) (tool.TaskRecord, error) {
	scope, err := m.taskScope()
	if err != nil {
		return tool.TaskRecord{}, err
	}
	task, err := m.sessions.ReviewTask(m.ctx, ReviewTaskInput{
		SessionID:     scope.SessionID,
		TurnID:        scope.TurnID,
		TaskID:        request.TaskID,
		ReviewStatus:  request.ReviewStatus,
		ReviewSummary: request.ReviewSummary,
	})
	if err != nil {
		return tool.TaskRecord{}, err
	}
	return mapTaskRecord(task), nil
}

func mapTaskRecords(tasks []events.TaskState) []tool.TaskRecord {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]tool.TaskRecord, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, mapTaskRecord(task))
	}
	return out
}

func mapTaskRecord(task events.TaskState) tool.TaskRecord {
	return tool.TaskRecord{
		TaskID:          task.TaskID,
		ParentTaskID:    task.ParentTaskID,
		WorkflowID:      task.WorkflowID,
		WorkflowPhaseID: task.WorkflowPhaseID,
		Title:           task.Title,
		Kind:            task.Kind,
		Status:          task.Status,
		Notes:           task.Notes,
		Progress:        task.Progress,
		BlockReason:     task.BlockReason,
		ReviewStatus:    task.ReviewStatus,
		ReviewSummary:   task.ReviewSummary,
	}
}

type taskScope struct {
	SessionID string
	TurnID    string
}

func (m sessionTaskManager) taskScope() (taskScope, error) {
	scope := taskScope{
		SessionID: m.input.SessionID,
		TurnID:    m.input.TurnID,
	}
	if m.sessions == nil || strings.TrimSpace(m.input.ToolName) != tool.TaskReviewToolName {
		return scope, nil
	}
	if strings.TrimSpace(scope.SessionID) == "" || strings.TrimSpace(scope.TurnID) == "" {
		return scope, nil
	}
	return scope, nil
}
