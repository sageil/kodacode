package events

const (
	WorkflowStatusActive    = "active"
	WorkflowStatusBlocked   = "blocked"
	WorkflowStatusCompleted = "completed"
)

const (
	WorkflowPhaseStatusInProgress = "in_progress"
	WorkflowPhaseStatusBlocked    = "blocked"
	WorkflowPhaseStatusCompleted  = "completed"
)

type WorkflowState struct {
	WorkflowID        string
	Status            string
	CurrentPhaseID    string
	PhaseOrder        []string
	Phases            map[string]*WorkflowPhaseState
	EvidenceOrder     []string
	Evidence          map[string]*WorkflowEvidenceState
	CompletedPhaseIDs []string
	BlockedPhaseIDs   []string
	StopReason        string
	StartedAtSeq      int64
	UpdatedAtSeq      int64
	CompletedAtSeq    int64
}

type WorkflowPhaseState struct {
	PhaseID        string
	Status         string
	StopReason     string
	EvidenceIDs    []string
	StartedAtSeq   int64
	UpdatedAtSeq   int64
	BlockedAtSeq   int64
	CompletedAtSeq int64
}

type WorkflowEvidenceState struct {
	EvidenceID    string
	WorkflowID    string
	PhaseID       string
	Type          string
	ArtifactID    string
	ToolCallID    string
	ExecutionID   string
	TaskID        string
	ReviewID      string
	Command       string
	ExitCode      *int
	Successful    *bool
	Summary       string
	Fields        map[string]string
	RecordedAtSeq int64
}

func cloneWorkflowState(state *WorkflowState) *WorkflowState {
	if state == nil {
		return nil
	}
	out := &WorkflowState{
		WorkflowID:        state.WorkflowID,
		Status:            state.Status,
		CurrentPhaseID:    state.CurrentPhaseID,
		PhaseOrder:        append([]string(nil), state.PhaseOrder...),
		Phases:            make(map[string]*WorkflowPhaseState, len(state.Phases)),
		EvidenceOrder:     append([]string(nil), state.EvidenceOrder...),
		Evidence:          make(map[string]*WorkflowEvidenceState, len(state.Evidence)),
		CompletedPhaseIDs: append([]string(nil), state.CompletedPhaseIDs...),
		BlockedPhaseIDs:   append([]string(nil), state.BlockedPhaseIDs...),
		StopReason:        state.StopReason,
		StartedAtSeq:      state.StartedAtSeq,
		UpdatedAtSeq:      state.UpdatedAtSeq,
		CompletedAtSeq:    state.CompletedAtSeq,
	}
	for id, phase := range state.Phases {
		out.Phases[id] = cloneWorkflowPhaseState(phase)
	}
	for id, evidence := range state.Evidence {
		out.Evidence[id] = cloneWorkflowEvidenceState(evidence)
	}
	return out
}

func cloneWorkflowPhaseState(state *WorkflowPhaseState) *WorkflowPhaseState {
	if state == nil {
		return nil
	}
	copyState := *state
	copyState.EvidenceIDs = append([]string(nil), state.EvidenceIDs...)
	return &copyState
}

func cloneWorkflowEvidenceState(state *WorkflowEvidenceState) *WorkflowEvidenceState {
	if state == nil {
		return nil
	}
	copyState := *state
	copyState.ExitCode = cloneIntPointer(state.ExitCode)
	copyState.Successful = cloneBoolPointer(state.Successful)
	copyState.Fields = cloneStringMap(state.Fields)
	return &copyState
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
