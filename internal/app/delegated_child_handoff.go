package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func delegatedChildHandoffForTurn(turn *events.TurnState, sessionID, turnID string) *events.AgentHandoffState {
	if turn == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(turnID) == "" {
		return nil
	}
	for idx := len(turn.HandoffOrder) - 1; idx >= 0; idx-- {
		handoff := turn.Handoffs[turn.HandoffOrder[idx]]
		if handoff == nil {
			continue
		}
		if strings.TrimSpace(handoff.ChildSessionID) != strings.TrimSpace(sessionID) {
			continue
		}
		if strings.TrimSpace(handoff.ChildTurnID) != strings.TrimSpace(turnID) {
			continue
		}
		if strings.TrimSpace(handoff.ParentSessionID) == "" || strings.TrimSpace(handoff.ParentTurnID) == "" {
			continue
		}
		return handoff
	}
	return nil
}
