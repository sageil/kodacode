package app

import (
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func pendingDelegatedHandoffState(state events.SessionState, handoffID string) (string, *events.AgentHandoffState) {
	handoffID = strings.TrimSpace(handoffID)
	if handoffID == "" {
		return "", nil
	}
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		handoff := turn.Handoffs[handoffID]
		if handoff == nil || !delegatedHandoffPending(handoff) {
			continue
		}
		return turnID, handoff
	}
	return "", nil
}

func firstPendingDelegatedHandoffState(state events.SessionState) (string, *events.AgentHandoffState) {
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turnID := state.TurnOrder[idx]
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		for handoffIdx := len(turn.HandoffOrder) - 1; handoffIdx >= 0; handoffIdx-- {
			handoff := turn.Handoffs[turn.HandoffOrder[handoffIdx]]
			if handoff == nil || !delegatedHandoffPending(handoff) {
				continue
			}
			return turnID, handoff
		}
	}
	return "", nil
}

func delegatedHandoffPending(handoff *events.AgentHandoffState) bool {
	if handoff == nil {
		return false
	}
	switch handoff.Status {
	case events.AgentResultStatusPendingPermission, events.AgentResultStatusPendingQuestion:
		return true
	default:
		return false
	}
}

func copyAgentHandoffState(state *events.AgentHandoffState) *events.AgentHandoffState {
	if state == nil {
		return nil
	}
	copyState := *state
	copyState.AllowedTools = slices.Clone(state.AllowedTools)
	copyState.QuestionOptions = append([]string(nil), state.QuestionOptions...)
	if state.ExecutionApproval != nil {
		copyApproval := *state.ExecutionApproval
		copyApproval.PrefixRule = append([]string(nil), state.ExecutionApproval.PrefixRule...)
		copyApproval.SessionGrantPaths = append([]string(nil), state.ExecutionApproval.SessionGrantPaths...)
		copyApproval.NetworkTargets = append([]string(nil), state.ExecutionApproval.NetworkTargets...)
		copyApproval.AvailableDecisions = append([]events.ExecutionApprovalDecision(nil), state.ExecutionApproval.AvailableDecisions...)
		if state.ExecutionApproval.ProposedExecPolicy != nil {
			copyPolicy := *state.ExecutionApproval.ProposedExecPolicy
			if state.ExecutionApproval.ProposedExecPolicy.AllowLoginShell != nil {
				allowLoginShell := *state.ExecutionApproval.ProposedExecPolicy.AllowLoginShell
				copyPolicy.AllowLoginShell = &allowLoginShell
			}
			copyApproval.ProposedExecPolicy = &copyPolicy
		}
		if state.ExecutionApproval.ProposedNetworkPolicy != nil {
			copyNetwork := *state.ExecutionApproval.ProposedNetworkPolicy
			copyApproval.ProposedNetworkPolicy = &copyNetwork
		}
		copyState.ExecutionApproval = &copyApproval
	}
	return &copyState
}
