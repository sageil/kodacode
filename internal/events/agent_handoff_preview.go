package events

import (
	"errors"
	"strings"
)

const TypeAgentHandoffPreview Type = "agent_handoff_preview"

type AgentHandoffPreviewPayload struct {
	HandoffID      string
	ChildSessionID string
	ChildTurnID    string
	Active         bool
	ToolName       string
	Action         string
	AssistantText  string
}

func (AgentHandoffPreviewPayload) eventType() Type { return TypeAgentHandoffPreview }

func (p AgentHandoffPreviewPayload) validate() error {
	switch {
	case strings.TrimSpace(p.HandoffID) == "":
		return errors.New("handoff_id is required")
	case strings.TrimSpace(p.ChildSessionID) == "":
		return errors.New("child_session_id is required")
	case strings.TrimSpace(p.ChildTurnID) == "":
		return errors.New("child_turn_id is required")
	default:
		return nil
	}
}
