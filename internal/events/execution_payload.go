package events

import (
	"errors"
	"strings"
)

type ExecutionDeclaredPayload struct {
	ExecutionID      string
	ToolCallID       string
	ToolName         string
	Kind             string
	Intent           string
	Effect           string
	Command          []string
	CommandPreview   string
	WorkingDirectory string
	TimeoutMS        int64
	OutputLimit      int
}

func (ExecutionDeclaredPayload) eventType() Type { return TypeExecutionDeclared }

func (p ExecutionDeclaredPayload) validate() error {
	switch {
	case strings.TrimSpace(p.ExecutionID) == "":
		return errors.New("execution_id is required")
	case strings.TrimSpace(p.ToolCallID) == "":
		return errors.New("tool_call_id is required")
	case strings.TrimSpace(p.ToolName) == "":
		return errors.New("tool_name is required")
	case strings.TrimSpace(p.Kind) == "":
		return errors.New("kind is required")
	case len(p.Command) == 0 || strings.TrimSpace(p.Command[0]) == "":
		return errors.New("command is required")
	case strings.TrimSpace(p.WorkingDirectory) == "":
		return errors.New("working_directory is required")
	case p.TimeoutMS < 0:
		return errors.New("timeout_ms must not be negative")
	case p.OutputLimit < 0:
		return errors.New("output_limit must not be negative")
	default:
		return nil
	}
}

type ExecutionApprovalRequestedPayload struct {
	RequestID             string
	ExecutionID           string
	ToolCallID            string
	ToolName              string
	Command               string
	WorkingDirectory      string
	Reason                string
	PrefixRule            []string
	SessionGrantPaths     []string
	NetworkTargets        []string
	AvailableDecisions    []ExecutionApprovalDecision
	ProposedExecPolicy    *ExecutionPolicyAmendment
	ProposedNetworkPolicy *ExecutionNetworkPolicyAmendment
}

func (ExecutionApprovalRequestedPayload) eventType() Type { return TypeExecutionApprovalRequested }

func (p ExecutionApprovalRequestedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.RequestID) == "":
		return errors.New("request_id is required")
	case strings.TrimSpace(p.ExecutionID) == "":
		return errors.New("execution_id is required")
	case strings.TrimSpace(p.ToolCallID) == "":
		return errors.New("tool_call_id is required")
	case strings.TrimSpace(p.ToolName) == "":
		return errors.New("tool_name is required")
	case strings.TrimSpace(p.Command) == "":
		return errors.New("command is required")
	case strings.TrimSpace(p.WorkingDirectory) == "":
		return errors.New("working_directory is required")
	case len(p.AvailableDecisions) == 0:
		return errors.New("available_decisions is required")
	default:
		return nil
	}
}

type ExecutionApprovalDecision string

const (
	ExecutionApprovalDecisionAccept               ExecutionApprovalDecision = "accept"
	ExecutionApprovalDecisionAcceptForSession     ExecutionApprovalDecision = "acceptForSession"
	ExecutionApprovalDecisionAcceptWithExecPolicy ExecutionApprovalDecision = "acceptWithExecpolicyAmendment"
	ExecutionApprovalDecisionApplyNetworkPolicy   ExecutionApprovalDecision = "applyNetworkPolicyAmendment"
	ExecutionApprovalDecisionDecline              ExecutionApprovalDecision = "decline"
	ExecutionApprovalDecisionCancel               ExecutionApprovalDecision = "cancel"
)

type ExecutionPolicyAmendment struct {
	AllowLoginShell *bool `json:"allow_login_shell,omitempty"`
}

type ExecutionNetworkPolicyAmendment struct {
	Enabled bool `json:"enabled"`
}

type ExecutionApprovalResolvedPayload struct {
	RequestID            string
	Decision             ExecutionApprovalDecision
	GrantPaths           []string
	GrantPrefixRule      []string
	GrantNetworkTargets  []string
	AppliedExecPolicy    *ExecutionPolicyAmendment
	AppliedNetworkPolicy *ExecutionNetworkPolicyAmendment
}

func (ExecutionApprovalResolvedPayload) eventType() Type { return TypeExecutionApprovalResolved }

func (p ExecutionApprovalResolvedPayload) validate() error {
	if strings.TrimSpace(p.RequestID) == "" {
		return errors.New("request_id is required")
	}
	switch p.Decision {
	case ExecutionApprovalDecisionAccept, ExecutionApprovalDecisionDecline, ExecutionApprovalDecisionCancel:
		if len(p.GrantPaths) != 0 {
			return errors.New("grant_paths must be empty for non-session execution approvals")
		}
		if len(p.GrantPrefixRule) != 0 {
			return errors.New("grant_prefix_rule must be empty for non-session execution approvals")
		}
		if len(p.GrantNetworkTargets) != 0 {
			return errors.New("grant_network_targets must be empty for non-session execution approvals")
		}
	case ExecutionApprovalDecisionAcceptForSession:
	case ExecutionApprovalDecisionAcceptWithExecPolicy:
		if p.AppliedExecPolicy == nil {
			return errors.New("applied_exec_policy is required")
		}
		if len(p.GrantPaths) != 0 {
			return errors.New("grant_paths must be empty for exec policy amendments")
		}
		if len(p.GrantPrefixRule) != 0 {
			return errors.New("grant_prefix_rule must be empty for exec policy amendments")
		}
		if len(p.GrantNetworkTargets) != 0 {
			return errors.New("grant_network_targets must be empty for exec policy amendments")
		}
	case ExecutionApprovalDecisionApplyNetworkPolicy:
		if p.AppliedNetworkPolicy == nil {
			return errors.New("applied_network_policy is required")
		}
		if len(p.GrantPaths) != 0 {
			return errors.New("grant_paths must be empty for network policy amendments")
		}
		if len(p.GrantPrefixRule) != 0 {
			return errors.New("grant_prefix_rule must be empty for network policy amendments")
		}
		if len(p.GrantNetworkTargets) != 0 {
			return errors.New("grant_network_targets must be empty for network policy amendments")
		}
	default:
		return errors.New("decision is required")
	}

	return nil
}

type ExecutionStartedPayload struct {
	ExecutionID string
	ToolCallID  string
	ToolName    string
	Input       string
}

func (ExecutionStartedPayload) eventType() Type { return TypeExecutionStarted }

func (p ExecutionStartedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.ExecutionID) == "":
		return errors.New("execution_id is required")
	case strings.TrimSpace(p.ToolCallID) == "":
		return errors.New("tool_call_id is required")
	case strings.TrimSpace(p.ToolName) == "":
		return errors.New("tool_name is required")
	case strings.TrimSpace(p.Input) == "":
		return errors.New("input is required")
	default:
		return nil
	}
}

type ExecutionOutputPayload struct {
	ExecutionID string
	ToolCallID  string
	Stream      string
	Chunk       string
}

func (ExecutionOutputPayload) eventType() Type { return TypeExecutionOutput }

func (p ExecutionOutputPayload) validate() error {
	switch {
	case strings.TrimSpace(p.ExecutionID) == "":
		return errors.New("execution_id is required")
	case strings.TrimSpace(p.ToolCallID) == "":
		return errors.New("tool_call_id is required")
	case p.Chunk == "":
		return errors.New("chunk is required")
	default:
		return nil
	}
}

type ExecutionBackgroundStartedPayload struct {
	ExecutionID     string
	ToolCallID      string
	ToolName        string
	PID             int
	ProcessIdentity string
	SupervisorID    string
	LogRef          string
	ReadyPatterns   []string
}

func (ExecutionBackgroundStartedPayload) eventType() Type { return TypeExecutionBackgroundStarted }

func (p ExecutionBackgroundStartedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.ExecutionID) == "":
		return errors.New("execution_id is required")
	case strings.TrimSpace(p.ToolCallID) == "":
		return errors.New("tool_call_id is required")
	case strings.TrimSpace(p.ToolName) == "":
		return errors.New("tool_name is required")
	case p.PID <= 0:
		return errors.New("pid must be > 0")
	case strings.TrimSpace(p.ProcessIdentity) == "":
		return errors.New("process_identity is required")
	case strings.TrimSpace(p.LogRef) == "":
		return errors.New("log_ref is required")
	default:
		return nil
	}
}

type ExecutionBackgroundObservedPayload struct {
	ExecutionID string
	ToolCallID  string
	ToolName    string
	OutputTail  string
	OutputBytes int64
}

func (ExecutionBackgroundObservedPayload) eventType() Type { return TypeExecutionBackgroundObserved }

func (p ExecutionBackgroundObservedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.ExecutionID) == "":
		return errors.New("execution_id is required")
	case strings.TrimSpace(p.ToolCallID) == "":
		return errors.New("tool_call_id is required")
	case strings.TrimSpace(p.ToolName) == "":
		return errors.New("tool_name is required")
	case p.OutputBytes < 0:
		return errors.New("output_bytes must not be negative")
	default:
		return nil
	}
}

type ExecutionBackgroundReadyPayload struct {
	ExecutionID string
	ToolCallID  string
	ToolName    string
	Message     string
	Port        int
}

func (ExecutionBackgroundReadyPayload) eventType() Type { return TypeExecutionBackgroundReady }

func (p ExecutionBackgroundReadyPayload) validate() error {
	switch {
	case strings.TrimSpace(p.ExecutionID) == "":
		return errors.New("execution_id is required")
	case strings.TrimSpace(p.ToolCallID) == "":
		return errors.New("tool_call_id is required")
	case strings.TrimSpace(p.ToolName) == "":
		return errors.New("tool_name is required")
	case strings.TrimSpace(p.Message) == "":
		return errors.New("message is required")
	case p.Port < 0:
		return errors.New("port must not be negative")
	default:
		return nil
	}
}

type ExecutionBackgroundExitedPayload struct {
	ExecutionID string
	ToolCallID  string
	ToolName    string
	ExitCode    *int
	Error       string
}

func (ExecutionBackgroundExitedPayload) eventType() Type { return TypeExecutionBackgroundExited }

func (p ExecutionBackgroundExitedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.ExecutionID) == "":
		return errors.New("execution_id is required")
	case strings.TrimSpace(p.ToolCallID) == "":
		return errors.New("tool_call_id is required")
	case strings.TrimSpace(p.ToolName) == "":
		return errors.New("tool_name is required")
	default:
		return nil
	}
}

type ExecutionBackgroundLostPayload struct {
	ExecutionID string
	ToolCallID  string
	ToolName    string
	Error       string
}

func (ExecutionBackgroundLostPayload) eventType() Type { return TypeExecutionBackgroundLost }

func (p ExecutionBackgroundLostPayload) validate() error {
	switch {
	case strings.TrimSpace(p.ExecutionID) == "":
		return errors.New("execution_id is required")
	case strings.TrimSpace(p.ToolCallID) == "":
		return errors.New("tool_call_id is required")
	case strings.TrimSpace(p.ToolName) == "":
		return errors.New("tool_name is required")
	case strings.TrimSpace(p.Error) == "":
		return errors.New("error is required")
	default:
		return nil
	}
}
