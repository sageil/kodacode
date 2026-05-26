package tool

type DelegateStatus string

const (
	DelegateStatusCompleted         DelegateStatus = "completed"
	DelegateStatusFailed            DelegateStatus = "failed"
	DelegateStatusPendingPermission DelegateStatus = "pending_permission"
	DelegateStatusPendingQuestion   DelegateStatus = "pending_question"
)

type DelegateRequest struct {
	ChildAgentID     string
	Task             string
	ContextSummary   string
	SourceHandoffIDs []string
}

type DelegatePendingPermission struct {
	RequestID        string `json:"request_id"`
	Kind             string `json:"kind,omitempty"`
	ToolName         string `json:"tool_name,omitempty"`
	Access           string `json:"access,omitempty"`
	Path             string `json:"path,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
	Command          string `json:"command,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type DelegatePendingQuestion struct {
	RequestID string   `json:"request_id"`
	ToolName  string   `json:"tool_name,omitempty"`
	Question  string   `json:"question,omitempty"`
	Options   []string `json:"options,omitempty"`
}

type DelegateRecord struct {
	HandoffID         string                     `json:"handoff_id"`
	ChildSessionID    string                     `json:"child_session_id"`
	ChildTurnID       string                     `json:"child_turn_id"`
	ChildAgentID      string                     `json:"child_agent_id"`
	Status            DelegateStatus             `json:"status"`
	AssistantText     string                     `json:"assistant_text,omitempty"`
	Error             string                     `json:"error,omitempty"`
	PendingPermission *DelegatePendingPermission `json:"pending_permission,omitempty"`
	PendingQuestion   *DelegatePendingQuestion   `json:"pending_question,omitempty"`
}

type DelegateManager interface {
	Delegate(DelegateRequest) (DelegateRecord, error)
}

func (e ExecutionContext) Delegates() (DelegateManager, error) {
	if e.DelegateManager == nil {
		return nil, ErrDelegateManagerRequired
	}
	return e.DelegateManager, nil
}
