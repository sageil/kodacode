package app

import (
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/workspace"
)

type NetworkAuthorizationInput struct {
	SessionID               string
	TurnID                  string
	ToolCallID              string
	Target                  string
	URL                     string
	ToolName                string
	Command                 string
	Reason                  string
	TemporaryNetworkTargets []string
}

type PathAuthorizationInput struct {
	SessionID       string
	TurnID          string
	ToolCallID      string
	Path            string
	Access          workspace.Access
	ToolName        string
	Command         string
	Reason          string
	TemporaryGrants []workspace.Grant
}

type ExecutionAuthorizationInput struct {
	SessionID             string
	TurnID                string
	ExecutionID           string
	ToolCallID            string
	ToolName              string
	Command               string
	WorkingDir            string
	Reason                string
	PrefixRule            []string
	SessionGrantPaths     []string
	NetworkTargets        []string
	AvailableDecisions    []events.ExecutionApprovalDecision
	ProposedExecPolicy    *events.ExecutionPolicyAmendment
	ProposedNetworkPolicy *events.ExecutionNetworkPolicyAmendment
}

type AuthorizationStatus string

const (
	AuthorizationStatusAuthorized AuthorizationStatus = "authorized"
	AuthorizationStatusPending    AuthorizationStatus = "pending"
)

type AuthorizationResult struct {
	Status    AuthorizationStatus
	Decision  workspace.Decision
	RequestID string
	Grants    []workspace.Grant
}

type permissionPolicyDecision struct {
	Action  permissionpolicy.Action
	Subject string
	Value   string
	Matched bool
}

type ResolvePermissionInput struct {
	SessionID              string
	TurnID                 string
	RequestID              string
	Decision               events.PermissionDecision
	Scope                  events.PermissionScope
	GrantPath              string
	Recursive              bool
	ExecutionDecision      events.ExecutionApprovalDecision
	ExecutionExecPolicy    *events.ExecutionPolicyAmendment
	ExecutionNetworkPolicy *events.ExecutionNetworkPolicyAmendment
}
