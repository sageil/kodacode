package tui

import "strings"

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

func orderedAgentContextHandoffIDs(turn *events.TurnState) []string {
	contextSelections := orderedAgentContextSelections(turn)
	if len(contextSelections) == 0 {
		return nil
	}
	contexts := make([]string, 0, len(contextSelections)+1)
	contexts = append(contexts, "")
	for _, selection := range contextSelections {
		contexts = append(contexts, selection.HandoffID)
	}
	return contexts
}

func (m *Model) moveSelectedHandoff(delta int) {
	turn := currentTurn(m.projector.Snapshot(), m.turnID)
	contexts := orderedAgentContextHandoffIDs(turn)
	if len(contexts) == 0 {
		if strings.TrimSpace(m.selection.detailTurnID) == strings.TrimSpace(m.turnID) &&
			strings.TrimSpace(m.selection.handoffID) == "" &&
			strings.TrimSpace(m.selection.callTurnID) == "" &&
			strings.TrimSpace(m.selection.callID) == "" {
			return
		}
		m.selection.detailTurnID = m.turnID
		m.selection.handoffID = ""
		m.selection.callSessionID = ""
		m.selection.callTurnID = ""
		m.selection.callID = ""
		m.clearExpandedToolCall()
		_ = m.applyTranscriptRefreshPlan(transcriptRefreshPlan{kind: transcriptRefreshStructure})
		m.syncInspectorBody(true)
		return
	}

	index := indexOfString(contexts, m.selection.handoffID)
	switch {
	case index < 0 && delta < 0:
		index = len(contexts) - 1
	case index < 0:
		index = 0
	default:
		index = max(min(index+delta, len(contexts)-1), 0)
	}
	if strings.TrimSpace(m.selection.detailTurnID) == strings.TrimSpace(m.turnID) &&
		strings.TrimSpace(m.selection.handoffID) == strings.TrimSpace(contexts[index]) &&
		strings.TrimSpace(m.selection.callTurnID) == "" &&
		strings.TrimSpace(m.selection.callID) == "" {
		return
	}
	m.selection.detailTurnID = m.turnID
	m.selection.handoffID = contexts[index]
	m.selection.callSessionID = ""
	m.selection.callTurnID = ""
	m.selection.callID = ""
	m.clearExpandedToolCall()
	_ = m.applyTranscriptRefreshPlan(transcriptRefreshPlan{kind: transcriptRefreshStructure})
	m.syncInspectorBody(true)
}
