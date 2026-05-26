package events

import (
	"errors"
	"strings"
)

const TypeAgentHandoff Type = "agent_handoff"

type AgentHandoffExplorationEntry struct {
	ToolName string
	Target   string
	Summary  string
}

func (e AgentHandoffExplorationEntry) valid() bool {
	return strings.TrimSpace(e.ToolName) != "" &&
		strings.TrimSpace(e.Target) != "" &&
		strings.TrimSpace(e.Summary) != ""
}

type AgentHandoffPayload struct {
	HandoffID          string
	ToolCallID         string
	ParentSessionID    string
	ParentTurnID       string
	ParentAgentID      string
	ChildSessionID     string
	ChildTurnID        string
	ChildAgentID       string
	Task               string
	ContextSummary     string
	SourceHandoffIDs   []string
	ProvidedKinds      []string
	ExplorationEntries []AgentHandoffExplorationEntry
	Model              string
	AllowedTools       []string
}

func (AgentHandoffPayload) eventType() Type { return TypeAgentHandoff }

func (p AgentHandoffPayload) validate() error {
	switch {
	case strings.TrimSpace(p.HandoffID) == "":
		return errors.New("handoff_id is required")
	case strings.TrimSpace(p.ParentSessionID) == "":
		return errors.New("parent_session_id is required")
	case strings.TrimSpace(p.ParentTurnID) == "":
		return errors.New("parent_turn_id is required")
	case strings.TrimSpace(p.ParentAgentID) == "":
		return errors.New("parent_agent_id is required")
	case strings.TrimSpace(p.ChildSessionID) == "":
		return errors.New("child_session_id is required")
	case strings.TrimSpace(p.ChildTurnID) == "":
		return errors.New("child_turn_id is required")
	case strings.TrimSpace(p.ChildAgentID) == "":
		return errors.New("child_agent_id is required")
	case strings.TrimSpace(p.Task) == "":
		return errors.New("task is required")
	case strings.TrimSpace(p.ContextSummary) == "":
		return errors.New("context_summary is required")
	case strings.TrimSpace(p.Model) == "":
		return errors.New("model is required")
	}
	for _, entry := range p.ExplorationEntries {
		if !entry.valid() {
			return errors.New("exploration_entries contains invalid entries")
		}
	}
	return nil
}
