package events

import (
	"errors"
	"strings"
)

const TypeAgentResult Type = "agent_result"

type AgentResultStatus string

const (
	AgentResultStatusCompleted         AgentResultStatus = "completed"
	AgentResultStatusPendingPermission AgentResultStatus = "pending_permission"
	AgentResultStatusPendingQuestion   AgentResultStatus = "pending_question"
	AgentResultStatusFailed            AgentResultStatus = "failed"
)

type AgentResultPayload struct {
	HandoffID           string
	ChildSessionID      string
	ChildTurnID         string
	Status              AgentResultStatus
	AssistantText       string
	Error               string
	PermissionRequestID string
	PermissionKind      PermissionRequestKind
	PermissionToolName  string
	PermissionAccess    string
	PermissionPath      string
	PermissionDir       string
	PermissionCommand   string
	PermissionReason    string
	ExecutionApproval   *ExecutionApprovalState
	QuestionRequestID   string
	QuestionToolName    string
	QuestionText        string
	QuestionOptions     []string
}

func (AgentResultPayload) eventType() Type { return TypeAgentResult }

func (p AgentResultPayload) validate() error {
	switch {
	case strings.TrimSpace(p.HandoffID) == "":
		return errors.New("handoff_id is required")
	case strings.TrimSpace(p.ChildSessionID) == "":
		return errors.New("child_session_id is required")
	case strings.TrimSpace(p.ChildTurnID) == "":
		return errors.New("child_turn_id is required")
	}

	switch p.Status {
	case AgentResultStatusCompleted:
		return nil
	case AgentResultStatusPendingPermission:
		switch {
		case strings.TrimSpace(p.PermissionRequestID) == "":
			return errors.New("permission_request_id is required for pending_permission")
		case p.PermissionKind == "":
			return errors.New("permission_kind is required for pending_permission")
		case strings.TrimSpace(p.PermissionToolName) == "":
			return errors.New("permission_tool_name is required for pending_permission")
		case strings.TrimSpace(p.PermissionCommand) == "":
			return errors.New("permission_command is required for pending_permission")
		}
		switch p.PermissionKind {
		case PermissionRequestKindPath:
			if p.ExecutionApproval != nil {
				return errors.New("execution_approval must be empty for path pending_permission")
			}
			switch {
			case strings.TrimSpace(p.PermissionAccess) == "":
				return errors.New("permission_access is required for path pending_permission")
			case strings.TrimSpace(p.PermissionPath) == "":
				return errors.New("permission_path is required for path pending_permission")
			}
		case PermissionRequestKindExecution:
			switch {
			case strings.TrimSpace(p.PermissionDir) == "":
				return errors.New("permission_dir is required for execution pending_permission")
			case p.ExecutionApproval == nil:
				return errors.New("execution_approval is required for execution pending_permission")
			}
			if err := validateDelegatedExecutionApproval(p); err != nil {
				return err
			}
		case PermissionRequestKindNetwork:
			if p.ExecutionApproval != nil {
				return errors.New("execution_approval must be empty for network pending_permission")
			}
			if strings.TrimSpace(p.PermissionPath) == "" {
				return errors.New("permission_path is required for network pending_permission")
			}
		default:
			return errors.New("permission_kind must be path, execution, or network for pending_permission")
		}
		return nil
	case AgentResultStatusPendingQuestion:
		switch {
		case strings.TrimSpace(p.QuestionRequestID) == "":
			return errors.New("question_request_id is required for pending_question")
		case strings.TrimSpace(p.QuestionText) == "":
			return errors.New("question_text is required for pending_question")
		case len(p.QuestionOptions) == 0:
			return errors.New("question_options is required for pending_question")
		}
		for _, option := range p.QuestionOptions {
			if strings.TrimSpace(option) == "" {
				return errors.New("question_options must not contain empty values")
			}
		}
		return nil
	case AgentResultStatusFailed:
		if strings.TrimSpace(p.Error) == "" {
			return errors.New("error is required for failed")
		}
		return nil
	default:
		return errors.New("status must be completed, pending_permission, pending_question, or failed")
	}
}

func validateDelegatedExecutionApproval(p AgentResultPayload) error {
	pending := p.ExecutionApproval
	switch {
	case strings.TrimSpace(pending.RequestID) == "":
		return errors.New("execution_approval.request_id is required")
	case pending.RequestID != p.PermissionRequestID:
		return errors.New("execution_approval.request_id must match permission_request_id")
	case strings.TrimSpace(pending.ToolName) == "":
		return errors.New("execution_approval.tool_name is required")
	case pending.ToolName != p.PermissionToolName:
		return errors.New("execution_approval.tool_name must match permission_tool_name")
	case strings.TrimSpace(pending.Command) == "":
		return errors.New("execution_approval.command is required")
	case pending.Command != p.PermissionCommand:
		return errors.New("execution_approval.command must match permission_command")
	case strings.TrimSpace(pending.WorkingDirectory) == "":
		return errors.New("execution_approval.working_directory is required")
	case pending.WorkingDirectory != p.PermissionDir:
		return errors.New("execution_approval.working_directory must match permission_dir")
	case len(pending.AvailableDecisions) == 0:
		return errors.New("execution_approval.available_decisions is required")
	}
	return nil
}
