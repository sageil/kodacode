package events

import (
	"errors"
	"strings"
)

const TypeAgentResultReused Type = "agent_result_reused"

type AgentResultReusedPayload struct {
	HandoffID      string
	ChildSessionID string
	ChildTurnID    string
	Content        string
}

func (AgentResultReusedPayload) eventType() Type { return TypeAgentResultReused }

func (p AgentResultReusedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.HandoffID) == "":
		return errors.New("handoff_id is required")
	case strings.TrimSpace(p.ChildSessionID) == "":
		return errors.New("child_session_id is required")
	case strings.TrimSpace(p.ChildTurnID) == "":
		return errors.New("child_turn_id is required")
	case strings.TrimSpace(p.Content) == "":
		return errors.New("content is required")
	default:
		return nil
	}
}
