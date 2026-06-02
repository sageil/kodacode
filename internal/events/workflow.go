package events

import (
	"errors"
	"strings"
)

const (
	TypeWorkflowStarted          Type = "workflow_started"
	TypeWorkflowPhaseStarted     Type = "workflow_phase_started"
	TypeWorkflowPhaseAdvanced    Type = "workflow_phase_advanced"
	TypeWorkflowPhaseBlocked     Type = "workflow_phase_blocked"
	TypeWorkflowPhaseResumed     Type = "workflow_phase_resumed"
	TypeWorkflowEvidenceRecorded Type = "workflow_evidence_recorded"
	TypeWorkflowCompleted        Type = "workflow_completed"
)

const (
	WorkflowEvidenceTypeApproval           = "approval"
	WorkflowEvidenceTypeDiagnostics        = "diagnostics"
	WorkflowEvidenceTypeGitDiff            = "git_diff"
	WorkflowEvidenceTypePhaseOutput        = "phase_output"
	WorkflowEvidenceTypeReview             = "review"
	WorkflowEvidenceTypeReviewOutcome      = "review_outcome"
	WorkflowEvidenceTypeTaskReview         = "task_review"
	WorkflowEvidenceTypeVerificationResult = "verification_result"
)

type WorkflowStartedPayload struct {
	WorkflowID string `json:"workflow_id"`
	PhaseID    string `json:"phase_id"`
}

func (WorkflowStartedPayload) eventType() Type { return TypeWorkflowStarted }

func (p WorkflowStartedPayload) validate() error {
	if strings.TrimSpace(p.WorkflowID) == "" {
		return errors.New("workflow_id is required")
	}
	if strings.TrimSpace(p.PhaseID) == "" {
		return errors.New("phase_id is required")
	}
	return nil
}

type WorkflowPhaseStartedPayload struct {
	WorkflowID string `json:"workflow_id"`
	PhaseID    string `json:"phase_id"`
}

func (WorkflowPhaseStartedPayload) eventType() Type { return TypeWorkflowPhaseStarted }

func (p WorkflowPhaseStartedPayload) validate() error {
	if strings.TrimSpace(p.WorkflowID) == "" {
		return errors.New("workflow_id is required")
	}
	if strings.TrimSpace(p.PhaseID) == "" {
		return errors.New("phase_id is required")
	}
	return nil
}

type WorkflowPhaseAdvancedPayload struct {
	WorkflowID  string `json:"workflow_id"`
	FromPhaseID string `json:"from_phase_id"`
	ToPhaseID   string `json:"to_phase_id"`
	StopReason  string `json:"stop_reason"`
}

func (WorkflowPhaseAdvancedPayload) eventType() Type { return TypeWorkflowPhaseAdvanced }

func (p WorkflowPhaseAdvancedPayload) validate() error {
	if strings.TrimSpace(p.WorkflowID) == "" {
		return errors.New("workflow_id is required")
	}
	if strings.TrimSpace(p.FromPhaseID) == "" {
		return errors.New("from_phase_id is required")
	}
	if strings.TrimSpace(p.ToPhaseID) == "" {
		return errors.New("to_phase_id is required")
	}
	return nil
}

type WorkflowPhaseBlockedPayload struct {
	WorkflowID string `json:"workflow_id"`
	PhaseID    string `json:"phase_id"`
	StopReason string `json:"stop_reason"`
}

func (WorkflowPhaseBlockedPayload) eventType() Type { return TypeWorkflowPhaseBlocked }

func (p WorkflowPhaseBlockedPayload) validate() error {
	if strings.TrimSpace(p.WorkflowID) == "" {
		return errors.New("workflow_id is required")
	}
	if strings.TrimSpace(p.PhaseID) == "" {
		return errors.New("phase_id is required")
	}
	if strings.TrimSpace(p.StopReason) == "" {
		return errors.New("stop_reason is required")
	}
	return nil
}

type WorkflowPhaseResumedPayload struct {
	WorkflowID string `json:"workflow_id"`
	PhaseID    string `json:"phase_id"`
}

func (WorkflowPhaseResumedPayload) eventType() Type { return TypeWorkflowPhaseResumed }

func (p WorkflowPhaseResumedPayload) validate() error {
	if strings.TrimSpace(p.WorkflowID) == "" {
		return errors.New("workflow_id is required")
	}
	if strings.TrimSpace(p.PhaseID) == "" {
		return errors.New("phase_id is required")
	}
	return nil
}

type WorkflowEvidenceRecordedPayload struct {
	EvidenceID  string            `json:"evidence_id"`
	WorkflowID  string            `json:"workflow_id"`
	PhaseID     string            `json:"phase_id"`
	Type        string            `json:"type"`
	ArtifactID  string            `json:"artifact_id"`
	ToolCallID  string            `json:"tool_call_id"`
	ExecutionID string            `json:"execution_id"`
	TaskID      string            `json:"task_id"`
	ReviewID    string            `json:"review_id"`
	Command     string            `json:"command"`
	ExitCode    *int              `json:"exit_code"`
	Successful  *bool             `json:"successful"`
	Summary     string            `json:"summary"`
	Fields      map[string]string `json:"fields"`
}

func (WorkflowEvidenceRecordedPayload) eventType() Type { return TypeWorkflowEvidenceRecorded }

func (p WorkflowEvidenceRecordedPayload) validate() error {
	if strings.TrimSpace(p.EvidenceID) == "" {
		return errors.New("evidence_id is required")
	}
	if strings.TrimSpace(p.WorkflowID) == "" {
		return errors.New("workflow_id is required")
	}
	if strings.TrimSpace(p.PhaseID) == "" {
		return errors.New("phase_id is required")
	}
	if strings.TrimSpace(p.Type) == "" {
		return errors.New("type is required")
	}
	return nil
}

type WorkflowCompletedPayload struct {
	WorkflowID string `json:"workflow_id"`
	PhaseID    string `json:"phase_id"`
	StopReason string `json:"stop_reason"`
}

func (WorkflowCompletedPayload) eventType() Type { return TypeWorkflowCompleted }

func (p WorkflowCompletedPayload) validate() error {
	if strings.TrimSpace(p.WorkflowID) == "" {
		return errors.New("workflow_id is required")
	}
	if strings.TrimSpace(p.PhaseID) == "" {
		return errors.New("phase_id is required")
	}
	return nil
}
