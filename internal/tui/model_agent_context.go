package tui

import "github.com/sageil/kodacode/internal/events"

type agentContextSelection struct {
	HandoffID string
	AgentID   string
}

func orderedAgentContextSelections(turn *events.TurnState) []agentContextSelection {
	if turn == nil {
		return nil
	}
	handoffIDs := orderedHandoffIDs(turn)
	if len(handoffIDs) == 0 {
		return nil
	}
	contexts := make([]agentContextSelection, 0, len(handoffIDs))
	seenAgents := make(map[string]struct{}, len(handoffIDs))
	for idx := len(handoffIDs) - 1; idx >= 0; idx-- {
		handoff := turn.Handoffs[handoffIDs[idx]]
		if handoff == nil {
			continue
		}
		agentID := handoff.ChildAgentID
		if agentID == "" {
			agentID = "delegated"
		}
		if _, ok := seenAgents[agentID]; ok {
			continue
		}
		seenAgents[agentID] = struct{}{}
		contexts = append(contexts, agentContextSelection{
			HandoffID: handoff.HandoffID,
			AgentID:   agentID,
		})
	}
	return contexts
}
