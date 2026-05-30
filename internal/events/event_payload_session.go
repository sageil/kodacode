package events

import (
	"errors"
	"strings"
)

type SessionConfiguredPayload struct {
	WorkspaceRoot            string
	AdditionalWorkspaceRoots []string
	PermissionMode           string
}

func (SessionConfiguredPayload) eventType() Type { return TypeSessionConfigured }

func (p SessionConfiguredPayload) validate() error {
	if strings.TrimSpace(p.WorkspaceRoot) == "" {
		return errors.New("workspace_root is required")
	}
	for _, root := range p.AdditionalWorkspaceRoots {
		if strings.TrimSpace(root) == "" {
			return errors.New("additional_workspace_roots must not contain empty paths")
		}
	}
	switch strings.TrimSpace(p.PermissionMode) {
	case "", "auto", "read_only", "full_access":
	default:
		return errors.New("permission_mode must be auto, read_only, or full_access")
	}
	return nil
}

type SessionModelRouteUpdatedPayload struct {
	Model string
}

func (SessionModelRouteUpdatedPayload) eventType() Type { return TypeSessionModelRouteUpdated }

func (p SessionModelRouteUpdatedPayload) validate() error {
	if strings.TrimSpace(p.Model) == "" {
		return errors.New("model is required")
	}
	return nil
}

type SessionWorkspaceRootsAddedPayload struct {
	WorkspaceRoots []string
}

func (SessionWorkspaceRootsAddedPayload) eventType() Type { return TypeSessionWorkspaceRootsAdded }

func (p SessionWorkspaceRootsAddedPayload) validate() error {
	if len(p.WorkspaceRoots) == 0 {
		return errors.New("workspace_roots is required")
	}
	for _, root := range p.WorkspaceRoots {
		if strings.TrimSpace(root) == "" {
			return errors.New("workspace_roots must not contain empty paths")
		}
	}
	return nil
}

type SessionPermissionModeUpdatedPayload struct {
	PermissionMode string
}

func (SessionPermissionModeUpdatedPayload) eventType() Type { return TypeSessionPermissionModeUpdated }

func (p SessionPermissionModeUpdatedPayload) validate() error {
	switch strings.TrimSpace(p.PermissionMode) {
	case "auto", "read_only", "full_access":
		return nil
	default:
		return errors.New("permission_mode must be auto, read_only, or full_access")
	}
}

type SessionProviderLimitUpdatedPayload struct {
	ProviderRequestLimitDisabled bool `json:"provider_request_limit_disabled"`
}

func (SessionProviderLimitUpdatedPayload) eventType() Type {
	return TypeSessionProviderLimitUpdated
}

func (SessionProviderLimitUpdatedPayload) validate() error { return nil }

type SessionBranchedPayload struct {
	ParentSessionID string
	ParentTurnID    string
	ParentSequence  int64
}

func (SessionBranchedPayload) eventType() Type { return TypeSessionBranched }

func (p SessionBranchedPayload) validate() error {
	if strings.TrimSpace(p.ParentSessionID) == "" {
		return errors.New("parent_session_id is required")
	}
	if strings.TrimSpace(p.ParentTurnID) == "" {
		return errors.New("parent_turn_id is required")
	}
	if p.ParentSequence < 0 {
		return errors.New("parent_sequence must be >= 0")
	}
	return nil
}
