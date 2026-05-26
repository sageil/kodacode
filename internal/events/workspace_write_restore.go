package events

import (
	"errors"
	"strings"
)

const TypeWorkspaceWriteRestored Type = "workspace_write_restored"

type WorkspaceWriteRestoredPayload struct {
	SourceTurnID string                      `json:"source_turn_id,omitempty"`
	Restores     []WorkspaceWriteRestoreItem `json:"restores,omitempty"`
}

func (WorkspaceWriteRestoredPayload) eventType() Type { return TypeWorkspaceWriteRestored }

func (p WorkspaceWriteRestoredPayload) validate() error {
	if strings.TrimSpace(p.SourceTurnID) == "" {
		return errors.New("source_turn_id is required")
	}
	if len(p.Restores) == 0 {
		return errors.New("restores is required")
	}
	for _, restore := range p.Restores {
		if !restore.valid() {
			return errors.New("restores contains invalid entries")
		}
	}
	return nil
}

type WorkspaceWriteRestoreItem struct {
	CallID        string `json:"call_id,omitempty"`
	Path          string `json:"path,omitempty"`
	ExistedBefore bool   `json:"existed_before,omitempty"`
}

func (r WorkspaceWriteRestoreItem) valid() bool {
	return strings.TrimSpace(r.CallID) != "" && strings.TrimSpace(r.Path) != ""
}
