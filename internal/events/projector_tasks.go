package events

import (
	"errors"
	"strings"
)

var ErrTaskNotFound = errors.New("task not found")

func (p *Projector) ensureTaskStore() {
	if p.state.Tasks == nil {
		p.state.Tasks = make(map[string]*TaskState)
	}
}

func (p *Projector) applyTaskCreated(sequence int64, payload TaskCreatedPayload) error {
	p.ensureTaskStore()

	taskID := resolveTaskID(payload.TaskID, sequence)
	if _, exists := p.state.Tasks[taskID]; exists {
		return errors.New("task already exists")
	}
	task := &TaskState{
		TaskID:          taskID,
		ParentTaskID:    strings.TrimSpace(payload.ParentTaskID),
		WorkflowID:      strings.TrimSpace(payload.WorkflowID),
		WorkflowPhaseID: strings.TrimSpace(payload.WorkflowPhaseID),
		Title:           strings.TrimSpace(payload.Title),
		Kind:            strings.TrimSpace(payload.Kind),
		Status:          strings.TrimSpace(payload.Status),
		Notes:           strings.TrimSpace(payload.Notes),
		CreatedAtSeq:    sequence,
		UpdatedAtSeq:    sequence,
	}
	p.state.Tasks[taskID] = task
	p.state.TaskOrder = append(p.state.TaskOrder, taskID)
	return nil
}

func (p *Projector) applyTaskProgressUpdated(sequence int64, payload TaskProgressUpdatedPayload) error {
	task := p.state.Tasks[strings.TrimSpace(payload.TaskID)]
	if task == nil {
		return ErrTaskNotFound
	}
	if status := strings.TrimSpace(payload.Status); status != "" {
		task.Status = status
		if status != TaskStatusBlocked {
			task.BlockReason = ""
		}
		if status != TaskStatusCompleted {
			task.CompletedAtSeq = 0
		}
	}
	if progress := strings.TrimSpace(payload.Progress); progress != "" {
		task.Progress = progress
	}
	if notes := strings.TrimSpace(payload.Notes); notes != "" {
		task.Notes = notes
	}
	updateTaskWorkflowBinding(task, payload.WorkflowID, payload.WorkflowPhaseID)
	task.UpdatedAtSeq = sequence
	return nil
}

func (p *Projector) applyTaskBlocked(sequence int64, payload TaskBlockedPayload) error {
	task := p.state.Tasks[strings.TrimSpace(payload.TaskID)]
	if task == nil {
		return ErrTaskNotFound
	}
	task.Status = TaskStatusBlocked
	task.BlockReason = strings.TrimSpace(payload.BlockReason)
	if notes := strings.TrimSpace(payload.Notes); notes != "" {
		task.Notes = notes
	}
	updateTaskWorkflowBinding(task, payload.WorkflowID, payload.WorkflowPhaseID)
	task.UpdatedAtSeq = sequence
	task.CompletedAtSeq = 0
	return nil
}

func (p *Projector) applyTaskCompleted(sequence int64, payload TaskCompletedPayload) error {
	task := p.state.Tasks[strings.TrimSpace(payload.TaskID)]
	if task == nil {
		return ErrTaskNotFound
	}
	task.Status = TaskStatusCompleted
	task.BlockReason = ""
	task.CompletedAtSeq = sequence
	task.UpdatedAtSeq = sequence
	if summary := strings.TrimSpace(payload.Summary); summary != "" {
		task.Progress = summary
	}
	updateTaskWorkflowBinding(task, payload.WorkflowID, payload.WorkflowPhaseID)
	return nil
}

func (p *Projector) applyTaskReviewed(sequence int64, payload TaskReviewedPayload) error {
	task := p.state.Tasks[strings.TrimSpace(payload.TaskID)]
	if task == nil {
		return ErrTaskNotFound
	}
	task.ReviewStatus = strings.TrimSpace(payload.ReviewStatus)
	task.ReviewSummary = strings.TrimSpace(payload.ReviewSummary)
	updateTaskWorkflowBinding(task, payload.WorkflowID, payload.WorkflowPhaseID)
	task.UpdatedAtSeq = sequence
	return nil
}

func updateTaskWorkflowBinding(task *TaskState, workflowID, phaseID string) {
	if task == nil {
		return
	}
	if workflowID = strings.TrimSpace(workflowID); workflowID != "" {
		task.WorkflowID = workflowID
	}
	if phaseID = strings.TrimSpace(phaseID); phaseID != "" {
		task.WorkflowPhaseID = phaseID
	}
}
