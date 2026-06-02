package events

import (
	"errors"
	"strings"
)

var ErrWorkflowStateMismatch = errors.New("workflow state mismatch")

func (p *Projector) ensureWorkflowPhase(phaseID string, sequence int64) *WorkflowPhaseState {
	if p.state.Workflow == nil {
		return nil
	}
	if p.state.Workflow.Phases == nil {
		p.state.Workflow.Phases = make(map[string]*WorkflowPhaseState)
	}
	phaseID = strings.TrimSpace(phaseID)
	phase := p.state.Workflow.Phases[phaseID]
	if phase != nil {
		return phase
	}
	phase = &WorkflowPhaseState{
		PhaseID:      phaseID,
		StartedAtSeq: sequence,
		UpdatedAtSeq: sequence,
	}
	p.state.Workflow.Phases[phaseID] = phase
	p.state.Workflow.PhaseOrder = appendUniqueString(p.state.Workflow.PhaseOrder, phaseID)
	return phase
}

func (p *Projector) applyWorkflowStarted(sequence int64, payload WorkflowStartedPayload) error {
	workflowID := strings.TrimSpace(payload.WorkflowID)
	phaseID := strings.TrimSpace(payload.PhaseID)
	p.state.Workflow = &WorkflowState{
		WorkflowID:     workflowID,
		Status:         WorkflowStatusActive,
		CurrentPhaseID: phaseID,
		Phases:         make(map[string]*WorkflowPhaseState),
		Evidence:       make(map[string]*WorkflowEvidenceState),
		StartedAtSeq:   sequence,
		UpdatedAtSeq:   sequence,
	}
	phase := p.ensureWorkflowPhase(phaseID, sequence)
	phase.Status = WorkflowPhaseStatusInProgress
	phase.StopReason = ""
	phase.BlockedAtSeq = 0
	p.state.Workflow.BlockedPhaseIDs = removeString(p.state.Workflow.BlockedPhaseIDs, phaseID)
	return nil
}

func (p *Projector) applyWorkflowPhaseStarted(sequence int64, payload WorkflowPhaseStartedPayload) error {
	workflow, err := p.requireWorkflow(strings.TrimSpace(payload.WorkflowID))
	if err != nil {
		return err
	}
	phaseID := strings.TrimSpace(payload.PhaseID)
	phase := p.ensureWorkflowPhase(phaseID, sequence)
	phase.Status = WorkflowPhaseStatusInProgress
	phase.StopReason = ""
	phase.BlockedAtSeq = 0
	phase.UpdatedAtSeq = sequence
	workflow.Status = WorkflowStatusActive
	workflow.CurrentPhaseID = phaseID
	workflow.StopReason = ""
	workflow.BlockedPhaseIDs = removeString(workflow.BlockedPhaseIDs, phaseID)
	workflow.UpdatedAtSeq = sequence
	return nil
}

func (p *Projector) applyWorkflowPhaseAdvanced(sequence int64, payload WorkflowPhaseAdvancedPayload) error {
	workflow, err := p.requireWorkflow(strings.TrimSpace(payload.WorkflowID))
	if err != nil {
		return err
	}
	fromPhaseID := strings.TrimSpace(payload.FromPhaseID)
	toPhaseID := strings.TrimSpace(payload.ToPhaseID)
	from := p.ensureWorkflowPhase(fromPhaseID, sequence)
	from.Status = WorkflowPhaseStatusCompleted
	from.StopReason = strings.TrimSpace(payload.StopReason)
	from.CompletedAtSeq = sequence
	from.UpdatedAtSeq = sequence
	workflow.CompletedPhaseIDs = appendUniqueString(workflow.CompletedPhaseIDs, fromPhaseID)
	workflow.BlockedPhaseIDs = removeString(workflow.BlockedPhaseIDs, fromPhaseID)

	to := p.ensureWorkflowPhase(toPhaseID, sequence)
	to.Status = WorkflowPhaseStatusInProgress
	to.StopReason = ""
	to.BlockedAtSeq = 0
	to.CompletedAtSeq = 0
	to.UpdatedAtSeq = sequence
	workflow.Status = WorkflowStatusActive
	workflow.CurrentPhaseID = toPhaseID
	workflow.StopReason = ""
	workflow.BlockedPhaseIDs = removeString(workflow.BlockedPhaseIDs, toPhaseID)
	workflow.UpdatedAtSeq = sequence
	return nil
}

func (p *Projector) applyWorkflowPhaseBlocked(sequence int64, payload WorkflowPhaseBlockedPayload) error {
	workflow, err := p.requireWorkflow(strings.TrimSpace(payload.WorkflowID))
	if err != nil {
		return err
	}
	phaseID := strings.TrimSpace(payload.PhaseID)
	reason := strings.TrimSpace(payload.StopReason)
	phase := p.ensureWorkflowPhase(phaseID, sequence)
	phase.Status = WorkflowPhaseStatusBlocked
	phase.StopReason = reason
	phase.BlockedAtSeq = sequence
	phase.UpdatedAtSeq = sequence
	workflow.Status = WorkflowStatusBlocked
	workflow.CurrentPhaseID = phaseID
	workflow.StopReason = reason
	workflow.BlockedPhaseIDs = appendUniqueString(workflow.BlockedPhaseIDs, phaseID)
	workflow.UpdatedAtSeq = sequence
	return nil
}

func (p *Projector) applyWorkflowPhaseResumed(sequence int64, payload WorkflowPhaseResumedPayload) error {
	workflow, err := p.requireWorkflow(strings.TrimSpace(payload.WorkflowID))
	if err != nil {
		return err
	}
	phaseID := strings.TrimSpace(payload.PhaseID)
	phase := p.ensureWorkflowPhase(phaseID, sequence)
	phase.Status = WorkflowPhaseStatusInProgress
	phase.StopReason = ""
	phase.BlockedAtSeq = 0
	phase.UpdatedAtSeq = sequence
	workflow.Status = WorkflowStatusActive
	workflow.CurrentPhaseID = phaseID
	workflow.StopReason = ""
	workflow.BlockedPhaseIDs = removeString(workflow.BlockedPhaseIDs, phaseID)
	workflow.UpdatedAtSeq = sequence
	return nil
}

func (p *Projector) applyWorkflowEvidenceRecorded(sequence int64, payload WorkflowEvidenceRecordedPayload) error {
	workflow, err := p.requireWorkflow(strings.TrimSpace(payload.WorkflowID))
	if err != nil {
		return err
	}
	if workflow.Evidence == nil {
		workflow.Evidence = make(map[string]*WorkflowEvidenceState)
	}
	evidenceID := strings.TrimSpace(payload.EvidenceID)
	if _, exists := workflow.Evidence[evidenceID]; exists {
		return errors.New("workflow evidence already exists")
	}
	phaseID := strings.TrimSpace(payload.PhaseID)
	phase := p.ensureWorkflowPhase(phaseID, sequence)
	evidence := &WorkflowEvidenceState{
		EvidenceID:    evidenceID,
		WorkflowID:    strings.TrimSpace(payload.WorkflowID),
		PhaseID:       phaseID,
		Type:          strings.TrimSpace(payload.Type),
		ArtifactID:    strings.TrimSpace(payload.ArtifactID),
		ToolCallID:    strings.TrimSpace(payload.ToolCallID),
		ExecutionID:   strings.TrimSpace(payload.ExecutionID),
		TaskID:        strings.TrimSpace(payload.TaskID),
		ReviewID:      strings.TrimSpace(payload.ReviewID),
		Command:       strings.TrimSpace(payload.Command),
		ExitCode:      cloneIntPointer(payload.ExitCode),
		Successful:    cloneBoolPointer(payload.Successful),
		Summary:       strings.TrimSpace(payload.Summary),
		Fields:        cloneStringMap(payload.Fields),
		RecordedAtSeq: sequence,
	}
	workflow.Evidence[evidenceID] = evidence
	workflow.EvidenceOrder = append(workflow.EvidenceOrder, evidenceID)
	phase.EvidenceIDs = appendUniqueString(phase.EvidenceIDs, evidenceID)
	phase.UpdatedAtSeq = sequence
	workflow.UpdatedAtSeq = sequence
	return nil
}

func (p *Projector) applyWorkflowCompleted(sequence int64, payload WorkflowCompletedPayload) error {
	workflow, err := p.requireWorkflow(strings.TrimSpace(payload.WorkflowID))
	if err != nil {
		return err
	}
	phaseID := strings.TrimSpace(payload.PhaseID)
	phase := p.ensureWorkflowPhase(phaseID, sequence)
	phase.Status = WorkflowPhaseStatusCompleted
	phase.StopReason = strings.TrimSpace(payload.StopReason)
	phase.CompletedAtSeq = sequence
	phase.UpdatedAtSeq = sequence
	workflow.Status = WorkflowStatusCompleted
	workflow.CurrentPhaseID = phaseID
	workflow.StopReason = strings.TrimSpace(payload.StopReason)
	workflow.CompletedPhaseIDs = appendUniqueString(workflow.CompletedPhaseIDs, phaseID)
	workflow.BlockedPhaseIDs = removeString(workflow.BlockedPhaseIDs, phaseID)
	workflow.CompletedAtSeq = sequence
	workflow.UpdatedAtSeq = sequence
	return nil
}

func (p *Projector) requireWorkflow(workflowID string) (*WorkflowState, error) {
	if p.state.Workflow == nil {
		return nil, ErrWorkflowStateMismatch
	}
	if p.state.Workflow.WorkflowID != workflowID {
		return nil, ErrWorkflowStateMismatch
	}
	return p.state.Workflow, nil
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func removeString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if len(values) == 0 || value == "" {
		return values
	}
	out := values[:0]
	for _, existing := range values {
		if existing != value {
			out = append(out, existing)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
