package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

var (
	ErrTaskIDRequired          = errors.New("task_id is required")
	ErrTaskTitleRequired       = errors.New("title is required")
	ErrTaskReviewSummaryNeeded = errors.New("review_summary is required")
)

type CreateTaskInput struct {
	SessionID    string
	TurnID       string
	TaskID       string
	ParentTaskID string
	Title        string
	Kind         string
	Status       string
	Notes        string
}

type CreateTaskOutcome struct {
	Task       events.TaskState
	ParentTask *events.TaskState
}

type UpdateTaskProgressInput struct {
	SessionID string
	TurnID    string
	TaskID    string
	Status    string
	Progress  string
	Notes     string
}

type BlockTaskInput struct {
	SessionID   string
	TurnID      string
	TaskID      string
	BlockReason string
	Notes       string
}

type CompleteTaskInput struct {
	SessionID string
	TurnID    string
	TaskID    string
	Summary   string
}

type ReviewTaskInput struct {
	SessionID     string
	TurnID        string
	TaskID        string
	ReviewStatus  string
	ReviewSummary string
}

func (s *SessionService) ListTasks(ctx context.Context, sessionID string) ([]events.TaskState, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, ErrSessionIDRequired
	}
	state, err := s.Snapshot(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(state.WorkspaceRoot) == "" {
		return nil, ErrSessionNotConfigured
	}
	return orderedTaskStates(state), nil
}

func (s *SessionService) CreateTask(ctx context.Context, input CreateTaskInput) (events.TaskState, error) {
	outcome, err := s.CreateTaskDetailed(ctx, input)
	if err != nil {
		return events.TaskState{}, err
	}
	return outcome.Task, nil
}

func (s *SessionService) CreateTaskDetailed(ctx context.Context, input CreateTaskInput) (CreateTaskOutcome, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return CreateTaskOutcome{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return CreateTaskOutcome{}, ErrTurnIDRequired
	}
	if strings.TrimSpace(input.Title) == "" {
		return CreateTaskOutcome{}, ErrTaskTitleRequired
	}
	state, err := s.Snapshot(ctx, input.SessionID)
	if err != nil {
		return CreateTaskOutcome{}, err
	}
	if strings.TrimSpace(state.WorkspaceRoot) == "" {
		return CreateTaskOutcome{}, ErrSessionNotConfigured
	}
	if taskID := strings.TrimSpace(input.TaskID); taskID != "" && state.Tasks[taskID] != nil {
		return CreateTaskOutcome{}, errors.New("task already exists")
	}
	parentTaskID := strings.TrimSpace(input.ParentTaskID)
	if err := validateTaskParent(state, strings.TrimSpace(input.TaskID), parentTaskID); err != nil {
		return CreateTaskOutcome{}, err
	}

	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = events.TaskStatusPending
	}
	if err := validateTaskInProgressTransition(state, "", parentTaskID, status); err != nil {
		return CreateTaskOutcome{}, err
	}
	workflowID, workflowPhaseID := activeWorkflowTaskBinding(state)
	draft := events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeTaskCreated,
		Payload: events.TaskCreatedPayload{
			TaskID:          strings.TrimSpace(input.TaskID),
			ParentTaskID:    parentTaskID,
			WorkflowID:      workflowID,
			WorkflowPhaseID: workflowPhaseID,
			Title:           strings.TrimSpace(input.Title),
			Kind:            strings.TrimSpace(input.Kind),
			Status:          status,
			Notes:           strings.TrimSpace(input.Notes),
		},
	}
	if err := draft.Validate(); err != nil {
		return CreateTaskOutcome{}, err
	}
	event, err := s.append(ctx, draft)
	if err != nil {
		return CreateTaskOutcome{}, err
	}
	task, err := findTaskBySequence(ctx, s, input.SessionID, event.Sequence)
	if err != nil {
		return CreateTaskOutcome{}, err
	}
	return CreateTaskOutcome{Task: task}, nil
}

func (s *SessionService) UpdateTaskProgress(ctx context.Context, input UpdateTaskProgressInput) (events.TaskState, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return events.TaskState{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return events.TaskState{}, ErrTurnIDRequired
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return events.TaskState{}, ErrTaskIDRequired
	}
	state, task, err := s.existingTask(ctx, input.SessionID, taskID)
	if err != nil {
		return events.TaskState{}, err
	}
	if err := validateTaskInProgressTransition(state, taskID, task.ParentTaskID, input.Status); err != nil {
		return events.TaskState{}, err
	}

	next := *task
	if status := strings.TrimSpace(input.Status); status != "" {
		next.Status = status
		if status != events.TaskStatusBlocked {
			next.BlockReason = ""
		}
		if status != events.TaskStatusCompleted {
			next.CompletedAtSeq = 0
		}
	}
	if progress := strings.TrimSpace(input.Progress); progress != "" {
		next.Progress = progress
	}
	if notes := strings.TrimSpace(input.Notes); notes != "" {
		next.Notes = notes
	}
	if taskStatesEqual(*task, next) {
		return next, nil
	}
	workflowID, workflowPhaseID := activeWorkflowTaskBinding(state)
	draft := events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeTaskProgressUpdated,
		Payload: events.TaskProgressUpdatedPayload{
			TaskID:          taskID,
			WorkflowID:      workflowID,
			WorkflowPhaseID: workflowPhaseID,
			Status:          strings.TrimSpace(input.Status),
			Progress:        strings.TrimSpace(input.Progress),
			Notes:           strings.TrimSpace(input.Notes),
		},
	}
	if err := draft.Validate(); err != nil {
		return events.TaskState{}, err
	}
	if _, err := s.append(ctx, draft); err != nil {
		return events.TaskState{}, err
	}
	return findTaskByID(ctx, s, input.SessionID, taskID)
}

func (s *SessionService) appendWorkflowTaskReviewEvidence(ctx context.Context, input ReviewTaskInput) error {
	state, err := s.Snapshot(ctx, input.SessionID)
	if err != nil {
		return err
	}
	workflow := state.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return nil
	}
	reviewPhase, err := s.workflowPhaseIsReview(ctx, state, workflow.WorkflowID, workflow.CurrentPhaseID)
	if err != nil {
		return err
	}
	if !reviewPhase {
		return nil
	}
	successful := taskReviewStatusSuccessful(input.ReviewStatus)
	_, err = s.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    workflowEventTurnID(input.TurnID),
		Type:      events.TypeWorkflowEvidenceRecorded,
		Payload: events.WorkflowEvidenceRecordedPayload{
			EvidenceID: newRuntimeID("workflow-evidence"),
			WorkflowID: workflow.WorkflowID,
			PhaseID:    workflow.CurrentPhaseID,
			Type:       events.WorkflowEvidenceTypeTaskReview,
			TaskID:     strings.TrimSpace(input.TaskID),
			Successful: &successful,
			Summary:    strings.TrimSpace(input.ReviewSummary),
			Fields: map[string]string{
				"review_status": strings.TrimSpace(input.ReviewStatus),
			},
		},
	})
	return err
}

func (s *SessionService) workflowPhaseIsReview(ctx context.Context, state events.SessionState, workflowID, phaseID string) (bool, error) {
	phaseID = strings.TrimSpace(phaseID)
	if phaseID == "" {
		return false, nil
	}
	s.workflowReviewMu.RLock()
	resolver := s.workflowReviewPhaseResolver
	s.workflowReviewMu.RUnlock()
	if resolver != nil {
		return resolver(ctx, state, workflowID, phaseID)
	}
	return false, nil
}

func taskReviewStatusSuccessful(status string) bool {
	switch strings.TrimSpace(status) {
	case events.TaskReviewStatusPass, events.TaskReviewStatusAccepted:
		return true
	default:
		return false
	}
}

func (s *SessionService) BlockTask(ctx context.Context, input BlockTaskInput) (events.TaskState, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return events.TaskState{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return events.TaskState{}, ErrTurnIDRequired
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return events.TaskState{}, ErrTaskIDRequired
	}
	state, task, err := s.existingTask(ctx, input.SessionID, taskID)
	if err != nil {
		return events.TaskState{}, err
	}

	next := *task
	next.Status = events.TaskStatusBlocked
	next.BlockReason = strings.TrimSpace(input.BlockReason)
	next.CompletedAtSeq = 0
	if notes := strings.TrimSpace(input.Notes); notes != "" {
		next.Notes = notes
	}
	if taskStatesEqual(*task, next) {
		return next, nil
	}
	workflowID, workflowPhaseID := activeWorkflowTaskBinding(state)

	draft := events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeTaskBlocked,
		Payload: events.TaskBlockedPayload{
			TaskID:          taskID,
			WorkflowID:      workflowID,
			WorkflowPhaseID: workflowPhaseID,
			BlockReason:     strings.TrimSpace(input.BlockReason),
			Notes:           strings.TrimSpace(input.Notes),
		},
	}
	if err := draft.Validate(); err != nil {
		return events.TaskState{}, err
	}
	if _, err := s.append(ctx, draft); err != nil {
		return events.TaskState{}, err
	}
	return findTaskByID(ctx, s, input.SessionID, taskID)
}

func (s *SessionService) CompleteTask(ctx context.Context, input CompleteTaskInput) (events.TaskState, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return events.TaskState{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return events.TaskState{}, ErrTurnIDRequired
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return events.TaskState{}, ErrTaskIDRequired
	}
	state, task, err := s.existingTask(ctx, input.SessionID, taskID)
	if err != nil {
		return events.TaskState{}, err
	}
	if err := validateTaskCompletion(state, task, input.Summary); err != nil {
		return events.TaskState{}, err
	}

	next := *task
	next.Status = events.TaskStatusCompleted
	next.BlockReason = ""
	if summary := strings.TrimSpace(input.Summary); summary != "" {
		next.Progress = summary
	}
	if taskStatesEqual(*task, next) {
		return next, nil
	}
	workflowID, workflowPhaseID := activeWorkflowTaskBinding(state)
	draft := events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeTaskCompleted,
		Payload: events.TaskCompletedPayload{
			TaskID:          taskID,
			WorkflowID:      workflowID,
			WorkflowPhaseID: workflowPhaseID,
			Summary:         strings.TrimSpace(input.Summary),
		},
	}
	if err := draft.Validate(); err != nil {
		return events.TaskState{}, err
	}
	if _, err := s.append(ctx, draft); err != nil {
		return events.TaskState{}, err
	}
	return findTaskByID(ctx, s, input.SessionID, taskID)
}

func (s *SessionService) ReviewTask(ctx context.Context, input ReviewTaskInput) (events.TaskState, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return events.TaskState{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return events.TaskState{}, ErrTurnIDRequired
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return events.TaskState{}, ErrTaskIDRequired
	}
	if strings.TrimSpace(input.ReviewSummary) == "" {
		return events.TaskState{}, ErrTaskReviewSummaryNeeded
	}
	state, task, err := s.existingTask(ctx, input.SessionID, taskID)
	if err != nil {
		return events.TaskState{}, err
	}

	next := *task
	next.ReviewStatus = strings.TrimSpace(input.ReviewStatus)
	next.ReviewSummary = strings.TrimSpace(input.ReviewSummary)
	if taskStatesEqual(*task, next) {
		return next, nil
	}
	workflowID, workflowPhaseID := activeWorkflowTaskBinding(state)

	draft := events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeTaskReviewed,
		Payload: events.TaskReviewedPayload{
			TaskID:          taskID,
			WorkflowID:      workflowID,
			WorkflowPhaseID: workflowPhaseID,
			ReviewStatus:    strings.TrimSpace(input.ReviewStatus),
			ReviewSummary:   strings.TrimSpace(input.ReviewSummary),
		},
	}
	if err := draft.Validate(); err != nil {
		return events.TaskState{}, err
	}
	if _, err := s.append(ctx, draft); err != nil {
		return events.TaskState{}, err
	}
	if err := s.appendWorkflowTaskReviewEvidence(ctx, input); err != nil {
		return events.TaskState{}, err
	}
	return findTaskByID(ctx, s, input.SessionID, taskID)
}

func (s *SessionService) existingTask(ctx context.Context, sessionID, taskID string) (events.SessionState, *events.TaskState, error) {
	state, err := s.Snapshot(ctx, sessionID)
	if err != nil {
		return events.SessionState{}, nil, err
	}
	if strings.TrimSpace(state.WorkspaceRoot) == "" {
		return events.SessionState{}, nil, ErrSessionNotConfigured
	}
	task := state.Tasks[taskID]
	if task == nil {
		return events.SessionState{}, nil, fmt.Errorf("%w: %s", events.ErrTaskNotFound, taskID)
	}
	return state, task, nil
}

func orderedTaskStates(state events.SessionState) []events.TaskState {
	if len(state.TaskOrder) == 0 {
		return nil
	}
	out := make([]events.TaskState, 0, len(state.TaskOrder))
	for _, taskID := range state.TaskOrder {
		task := state.Tasks[taskID]
		if task == nil {
			continue
		}
		out = append(out, *task)
	}
	return out
}

func findTaskBySequence(ctx context.Context, sessions *SessionService, sessionID string, sequence int64) (events.TaskState, error) {
	state, err := sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return events.TaskState{}, err
	}
	for _, taskID := range state.TaskOrder {
		task := state.Tasks[taskID]
		if task != nil && task.CreatedAtSeq == sequence {
			return *task, nil
		}
	}
	return events.TaskState{}, events.ErrTaskNotFound
}

func findTaskByID(ctx context.Context, sessions *SessionService, sessionID, taskID string) (events.TaskState, error) {
	state, err := sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return events.TaskState{}, err
	}
	task := state.Tasks[taskID]
	if task == nil {
		return events.TaskState{}, fmt.Errorf("%w: %s", events.ErrTaskNotFound, taskID)
	}
	return *task, nil
}

func taskStatesEqual(left, right events.TaskState) bool {
	return left.TaskID == right.TaskID &&
		left.ParentTaskID == right.ParentTaskID &&
		left.WorkflowID == right.WorkflowID &&
		left.WorkflowPhaseID == right.WorkflowPhaseID &&
		left.Title == right.Title &&
		left.Kind == right.Kind &&
		left.Status == right.Status &&
		left.Notes == right.Notes &&
		left.Progress == right.Progress &&
		left.BlockReason == right.BlockReason &&
		left.ReviewStatus == right.ReviewStatus &&
		left.ReviewSummary == right.ReviewSummary &&
		left.CompletedAtSeq == right.CompletedAtSeq
}

func activeWorkflowTaskBinding(state events.SessionState) (string, string) {
	workflow := state.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return "", ""
	}
	return strings.TrimSpace(workflow.WorkflowID), strings.TrimSpace(workflow.CurrentPhaseID)
}
