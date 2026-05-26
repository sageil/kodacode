package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func traceDialogSessionIndexBody(th *theme.Theme, state events.SessionState) string {
	turnIDs := orderedSessionTurnIDs(state)
	if len(turnIDs) == 0 {
		return strings.Join([]string{
			dialogSectionStyle(th).Render("Session Turn Index"),
			"No turns are available yet.",
		}, "\n")
	}
	sections := []string{
		strings.Join([]string{
			dialogSectionStyle(th).Render("Session Turn Index"),
			fmt.Sprintf("Turns: %d", len(turnIDs)),
			"Submit /trace [turn-number] to inspect one turn in full detail.",
		}, "\n"),
	}
	for _, turnID := range turnIDs {
		turn := state.Turns[turnID]
		if section := traceDialogSessionTurnSection(th, state, turnID, turn); section != "" {
			sections = append(sections, section)
		}
	}
	return strings.Join(sections, "\n\n")
}

func traceDialogSessionTurnSection(th *theme.Theme, state events.SessionState, turnID string, turn *events.TurnState) string {
	if turn == nil {
		return ""
	}
	ordinal := sessionToolTurnOrdinal(state, turnID)
	headerParts := []string{
		fmt.Sprintf("Turn %d", ordinal),
		costDialogTurnStatus(turn),
	}
	if model := costDialogTurnModel(turn); model != "unknown" {
		headerParts = append(headerParts, model)
	}
	if turn.ProviderUsage != nil {
		switch {
		case costDialogPricingUnavailable(turn):
			headerParts = append(headerParts, "pricing unavailable")
		case turn.ProviderUsage.EstimatedInputCost+turn.ProviderUsage.EstimatedOutputCost > 0:
			headerParts = append(headerParts, formatEstimatedCost(turn.ProviderUsage.EstimatedInputCost+turn.ProviderUsage.EstimatedOutputCost))
		}
	}
	lines := []string{
		lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorFor(th, "primary", "#7aa2f7"))).
			Render(strings.Join(headerParts, " • ")),
	}
	if prompt := costDialogPromptPreview(turn); prompt != "" {
		lines = append(lines, "Prompt: "+prompt)
	}
	if turn.ProviderUsage != nil {
		lines = append(lines, "Provider usage: "+traceDialogUsageLine(turn))
	}
	if activity := costDialogTurnActivity(turn); activity != "" {
		lines = append(lines, "Activity: "+activity)
	}
	if signals := costDialogTurnSignals(turn); len(signals) > 0 {
		lines = append(lines, "Signals: "+strings.Join(signals, " • "))
	}
	if errorText := traceDialogPreview(turn.Error, 180); errorText != "" {
		lines = append(lines, "Error: "+errorText)
	}
	return strings.Join(lines, "\n")
}
