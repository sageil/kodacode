package events

import (
	"errors"
	"strings"
)

type PermissionRequestKind string

const (
	PermissionRequestKindPath      PermissionRequestKind = "path"
	PermissionRequestKindExecution PermissionRequestKind = "execution"
	PermissionRequestKindNetwork   PermissionRequestKind = "network"
)

type PermissionRequestedPayload struct {
	Kind             PermissionRequestKind
	RequestID        string
	ExecutionID      string
	ToolCallID       string
	Access           string
	Path             string
	WorkingDirectory string
	ToolName         string
	Command          string
	Reason           string
}

func (PermissionRequestedPayload) eventType() Type { return TypePermissionRequested }

func (p PermissionRequestedPayload) validate() error {
	if strings.TrimSpace(p.RequestID) == "" {
		return errors.New("request_id is required")
	}
	if p.Kind == "" {
		return errors.New("kind is required")
	}
	if strings.TrimSpace(p.ToolCallID) == "" {
		return errors.New("tool_call_id is required")
	}
	if strings.TrimSpace(p.ToolName) == "" {
		return errors.New("tool_name is required")
	}
	if strings.TrimSpace(p.Command) == "" {
		return errors.New("command is required")
	}
	switch p.Kind {
	case PermissionRequestKindPath:
		if strings.TrimSpace(p.Access) == "" {
			return errors.New("access is required for path permission")
		}
		if strings.TrimSpace(p.Path) == "" {
			return errors.New("path is required for path permission")
		}
	case PermissionRequestKindExecution:
		if strings.TrimSpace(p.ExecutionID) == "" {
			return errors.New("execution_id is required for execution permission")
		}
		if strings.TrimSpace(p.WorkingDirectory) == "" {
			return errors.New("working_directory is required for execution permission")
		}
	case PermissionRequestKindNetwork:
		if strings.TrimSpace(p.Path) == "" {
			return errors.New("path is required for network permission")
		}
	default:
		return errors.New("kind must be path, execution, or network")
	}
	return nil
}

type PermissionDecision string

const (
	PermissionDecisionApproved PermissionDecision = "approved"
	PermissionDecisionDenied   PermissionDecision = "denied"
)

type PermissionScope string

const (
	PermissionScopeOnce    PermissionScope = "once"
	PermissionScopeSession PermissionScope = "session"
)

type PermissionResolvedPayload struct {
	RequestID  string
	Decision   PermissionDecision
	Scope      PermissionScope
	GrantPath  string
	GrantPaths []string
	Recursive  bool
}

func (PermissionResolvedPayload) eventType() Type { return TypePermissionResolved }

func (p PermissionResolvedPayload) validate() error {
	if strings.TrimSpace(p.RequestID) == "" {
		return errors.New("request_id is required")
	}

	switch p.Decision {
	case PermissionDecisionApproved:
		switch p.Scope {
		case PermissionScopeOnce:
			if strings.TrimSpace(p.GrantPath) != "" {
				return errors.New("grant_path must be empty for once approval")
			}
			if len(p.GrantPaths) != 0 {
				return errors.New("grant_paths must be empty for once approval")
			}
		case PermissionScopeSession:
		default:
			return errors.New("scope is required for approved permission")
		}
	case PermissionDecisionDenied:
		if p.Scope != "" {
			return errors.New("scope must be empty when permission is denied")
		}
		if strings.TrimSpace(p.GrantPath) != "" {
			return errors.New("grant_path must be empty when permission is denied")
		}
		if len(p.GrantPaths) != 0 {
			return errors.New("grant_paths must be empty when permission is denied")
		}
	default:
		return errors.New("decision is required")
	}

	return nil
}
