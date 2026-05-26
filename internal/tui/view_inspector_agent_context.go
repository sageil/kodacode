package tui

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	tuitheme "github.com/sageil/kodacode/internal/tui/theme"
)

type inspectorAgentContext struct {
	AgentLabel     string
	StatusLabel    string
	TurnLabel      string
	ModelLabel     string
	ProviderLabel  string
	ReasoningLabel string
	PWDLabel       string
	ThemeLabel     string
}

func resolveInspectorAgentContext(m Model, state events.SessionState, turnID string, turn *events.TurnState) inspectorAgentContext {
	if isLocalShellTurn(turn) {
		return inspectorAgentContext{
			AgentLabel:    "local shell",
			StatusLabel:   renderStatusForTurn(m, state, turnID),
			TurnLabel:     fallbackRootTurnLabel(state, turnID),
			ModelLabel:    "shell",
			ProviderLabel: "local",
			PWDLabel:      fallbackPWDLabel(state, m),
			ThemeLabel:    fallbackThemeLabel(m),
		}
	}
	agentLabel := inspectorTurnAgentLabel(m, state, turnID, turn)
	modelLabel, providerLabel := inspectorTurnModelDetails(m, state, turnID, turn)
	return inspectorAgentContext{
		AgentLabel:     agentLabel,
		StatusLabel:    renderStatusForTurn(m, state, turnID),
		TurnLabel:      fallbackRootTurnLabel(state, turnID),
		ModelLabel:     modelLabel,
		ProviderLabel:  providerLabel,
		ReasoningLabel: inspectorTurnReasoningLabel(m, state, turnID, turn),
		PWDLabel:       fallbackPWDLabel(state, m),
		ThemeLabel:     fallbackThemeLabel(m),
	}
}

func inspectorTurnAgentLabel(m Model, state events.SessionState, turnID string, turn *events.TurnState) string {
	if inspectorUsesCurrentSessionAgentSelection(m, turnID, turn) {
		return transcriptAgentLabel(m, state, turnID)
	}
	if turn != nil && turn.Config != nil {
		if agentID := strings.TrimSpace(turn.Config.AgentID); agentID != "" {
			return agentID
		}
	}
	if agentID := strings.TrimSpace(m.agentID); agentID != "" {
		return agentID
	}
	return "builder"
}

func inspectorUsesCurrentSessionAgentSelection(m Model, turnID string, turn *events.TurnState) bool {
	if turn == nil || !isTurnFinished(turn) {
		return false
	}
	if strings.TrimSpace(turnID) != strings.TrimSpace(m.turnID) {
		return false
	}
	if strings.TrimSpace(m.selection.callTurnID) != "" || strings.TrimSpace(m.selection.callID) != "" {
		return false
	}
	return true
}

func inspectorTurnModelDetails(m Model, state events.SessionState, turnID string, turn *events.TurnState) (string, string) {
	if inspectorUsesCurrentSessionModelSelection(m, state, turnID, turn) {
		return effectiveSelectedAgentModelDetails(m, state)
	}
	if turn != nil && turn.Config != nil {
		if ref, ok := inspectorTurnModelRef(turn.Config); ok {
			return inspectorModelDetails(ref)
		}
	}
	return effectiveSelectedAgentModelDetails(m, state)
}

func inspectorTurnReasoningLabel(m Model, state events.SessionState, turnID string, turn *events.TurnState) string {
	if inspectorUsesCurrentSessionModelSelection(m, state, turnID, turn) {
		return currentSessionReasoningLabel(m, state)
	}
	if turn != nil && turn.Config != nil {
		return turnReasoningLabel(turn.Config)
	}
	return currentSessionReasoningLabel(m, state)
}

func inspectorUsesCurrentSessionModelSelection(m Model, state events.SessionState, turnID string, turn *events.TurnState) bool {
	if turn == nil || turn.Config == nil || !turn.Config.PreserveSessionModel {
		return false
	}
	return inspectorUsesCurrentSessionAgentSelection(m, turnID, turn)
}

func inspectorTurnModelRef(config *events.TurnConfigState) (provider.ModelRef, bool) {
	if config == nil {
		return provider.ModelRef{}, false
	}
	ref, err := provider.ParseModelRef(strings.TrimSpace(config.Model))
	if err != nil {
		return provider.ModelRef{}, false
	}
	return ref, true
}

func inspectorModelDetails(ref provider.ModelRef) (string, string) {
	modelID := strings.TrimSpace(ref.ModelID)
	if modelID == "" {
		modelID = "unknown"
	}
	providerID := strings.TrimSpace(ref.ProviderID)
	if providerID == "" {
		providerID = "unknown"
	}
	return modelID, providerID
}

func fallbackRootTurnLabel(state events.SessionState, turnID string) string {
	for idx, tid := range state.TurnOrder {
		if tid == turnID {
			return fmt.Sprintf("%d", idx+1)
		}
	}
	return "1"
}

func fallbackPWDLabel(state events.SessionState, m Model) string {
	pwdLabel := inspectorPWDLabel(pickWorkspace(state.WorkspaceRoot, m.workspace))
	if strings.TrimSpace(pwdLabel) == "" {
		return "kodacode"
	}
	return pwdLabel
}

func fallbackThemeLabel(m Model) string {
	if m.theme != nil && strings.TrimSpace(m.theme.Name) != "" {
		return m.theme.Name
	}
	return tuitheme.StaticDefault().Name
}
