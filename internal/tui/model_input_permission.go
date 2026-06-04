package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

func (m Model) handleInlinePermissionInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.pendingExecution() == nil && m.pendingPermission() == nil {
		return m, nil, false
	}
	if m.interactionResolutionInFlight() {
		return m, nil, true
	}

	maxChoice := m.permissionChoiceCount() - 1
	switch msg.String() {
	case "up", "k":
		if m.interaction.cursor > 0 {
			m.interaction.cursor--
		}
		return m, nil, true
	case "down", "j":
		if m.interaction.cursor < maxChoice {
			m.interaction.cursor++
		}
		return m, nil, true
	case "1":
		updated, cmd := m.startPermissionResolution(0)
		return updated, cmd, true
	case "2":
		updated, cmd := m.startPermissionResolution(1)
		return updated, cmd, true
	case "3":
		updated, cmd := m.startPermissionResolution(2)
		return updated, cmd, true
	case "4":
		updated, cmd := m.startPermissionResolution(3)
		return updated, cmd, true
	case "5":
		updated, cmd := m.startPermissionResolution(4)
		return updated, cmd, true
	case "enter":
		updated, cmd := m.startPermissionResolution(m.interaction.cursor)
		return updated, cmd, true
	default:
		return m, nil, false
	}
}

func (m Model) handlePermissionInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.interactionResolutionInFlight() || m.chrome.focus != focusInspector {
		return m, nil
	}
	if m.pendingExecution() != nil || m.pendingPermission() != nil {
		return m, nil
	}
	maxChoice := m.permissionChoiceCount() - 1

	switch msg.String() {
	case "up", "k":
		if m.interaction.cursor > 0 {
			m.interaction.cursor--
		}
		return m, nil
	case "down", "j":
		if m.interaction.cursor < maxChoice {
			m.interaction.cursor++
		}
		return m, nil
	case "1":
		return m.startPermissionResolution(0)
	case "2":
		return m.startPermissionResolution(1)
	case "3":
		return m.startPermissionResolution(2)
	case "4":
		return m.startPermissionResolution(3)
	case "5":
		return m.startPermissionResolution(4)
	case "enter":
		return m.startPermissionResolution(m.interaction.cursor)
	default:
		return m, nil
	}
}

func (m Model) startPermissionResolution(choice int) (tea.Model, tea.Cmd) {
	if pending := m.pendingExecution(); pending != nil {
		decision, execPolicy, networkPolicy := executionApprovalChoice(choice, pending)
		m.busy = true
		m.interaction.resolveReq = pending.RequestID
		m.userText = ""
		turnID := m.turnID
		if strings.TrimSpace(pending.TurnID) != "" {
			turnID = strings.TrimSpace(pending.TurnID)
		}
		return m, resolvePermissionCmd(
			m.ctx,
			m.controller,
			m.sessionID,
			turnID,
			pending.RequestID,
			"",
			m.skillIDs,
			"",
			"",
			"",
			false,
			decision,
			execPolicy,
			networkPolicy,
		)
	}
	if pending := m.pendingPermission(); pending != nil {
		decision, scope, grantPath, recursive := permissionChoice(choice, pending.Kind, pending.Path)
		m.busy = true
		m.interaction.resolveReq = pending.RequestID
		m.userText = ""
		turnID := m.turnID
		if strings.TrimSpace(pending.TurnID) != "" {
			turnID = strings.TrimSpace(pending.TurnID)
		}
		return m, resolvePermissionCmd(
			m.ctx,
			m.controller,
			m.sessionID,
			turnID,
			pending.RequestID,
			"",
			m.skillIDs,
			decision,
			scope,
			grantPath,
			recursive,
			"",
			nil,
			nil,
		)
	}
	if pending := m.pendingQuestion(); pending != nil {
		if choice := questionChoice(choice, pending); choice != "" {
			m.busy = true
			m.interaction.resolveReq = pending.QuestionID
			m.userText = ""
			turnID := m.turnID
			if strings.TrimSpace(pending.TurnID) != "" {
				turnID = strings.TrimSpace(pending.TurnID)
			}
			return m, answerQuestionCmd(
				m.ctx,
				m.controller,
				m.sessionID,
				turnID,
				pending.QuestionID,
				"",
				choice,
				m.skillIDs,
			)
		}
		return m, nil
	}
	return m, nil
}

func permissionChoice(choice int, pendingKind events.PermissionRequestKind, pendingPath string) (events.PermissionDecision, events.PermissionScope, string, bool) {
	switch choice {
	case 0:
		return events.PermissionDecisionApproved, events.PermissionScopeOnce, "", false
	case 1:
		return events.PermissionDecisionApproved, events.PermissionScopeSession, permissionGrantTarget(pendingKind, pendingPath), false
	default:
		return events.PermissionDecisionDenied, "", "", false
	}
}

func permissionGrantTarget(kind events.PermissionRequestKind, pendingPath string) string {
	if kind == events.PermissionRequestKindExecution || kind == events.PermissionRequestKindNetwork {
		return ""
	}
	return pendingPath
}

func (m Model) permissionChoiceCount() int {
	if pending := m.pendingQuestion(); pending != nil {
		return max(len(pending.Options), 1)
	}
	count := 3
	if pending := effectiveExecutionApprovalChoiceState(m); pending != nil {
		for _, decision := range pending.AvailableDecisions {
			switch decision {
			case events.ExecutionApprovalDecisionApplyNetworkPolicy, events.ExecutionApprovalDecisionAcceptWithExecPolicy:
				count++
			}
		}
	}
	return count
}

func effectiveExecutionApprovalChoiceState(m Model) *events.ExecutionApprovalState {
	return m.pendingExecution()
}

func executionApprovalChoice(choice int, pending *events.ExecutionApprovalState) (events.ExecutionApprovalDecision, *events.ExecutionPolicyAmendment, *events.ExecutionNetworkPolicyAmendment) {
	if pending == nil {
		return events.ExecutionApprovalDecisionDecline, nil, nil
	}
	switch choice {
	case 0:
		return events.ExecutionApprovalDecisionAccept, nil, nil
	case 1:
		return events.ExecutionApprovalDecisionAcceptForSession, nil, nil
	case 3:
		for _, decision := range pending.AvailableDecisions {
			if decision == events.ExecutionApprovalDecisionApplyNetworkPolicy {
				return decision, nil, pending.ProposedNetworkPolicy
			}
		}
		for _, decision := range pending.AvailableDecisions {
			if decision == events.ExecutionApprovalDecisionAcceptWithExecPolicy {
				return decision, pending.ProposedExecPolicy, nil
			}
		}
	case 4:
		for _, decision := range pending.AvailableDecisions {
			if decision == events.ExecutionApprovalDecisionAcceptWithExecPolicy {
				return decision, pending.ProposedExecPolicy, nil
			}
		}
	}
	return events.ExecutionApprovalDecisionDecline, nil, nil
}
