package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderInlineQuestionPrompt(m Model, width int) string {
	if m.pendingInteractionSubmissionInFlight() {
		return ""
	}
	pending := m.pendingQuestion()
	if pending == nil {
		return ""
	}

	width = max(width, 1)
	accentColor := colorFor(m.theme, "secondary", "#8be9fd")
	textColor := colorFor(m.theme, "text", "#ecf0ff")
	subtextColor := colorFor(m.theme, "subtext", "#9da8ca")
	selectedColor := colorFor(m.theme, "primary", "#7cc7ff")

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(subtextColor))
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentColor)).
		Bold(true)
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(selectedColor)).
		Bold(true)
	optionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(textColor))

	topBorder := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentColor)).
		Render(strings.Repeat("▔", width))
	bottomSep := dimStyle.Render(strings.Repeat("─", width))

	lines := []string{topBorder}
	for _, line := range wrapTranscriptText(strings.TrimSpace(pending.Question), max(min(width-8, 96), 20)) {
		lines = append(lines, centerInlinePromptLine(promptStyle.Render(line), width))
	}
	if title := pendingQuestionPlanTitle(m.projector.CurrentState(), *pending); title != "" {
		lines = append(lines, centerInlinePromptLine(dimStyle.Render("Plan: "+title), width))
	}
	lines = append(lines, centerInlinePromptLine(dimStyle.Render(questionPanelHints(*pending)), width))

	optionWidth := max(min(width-10, 96), 18)
	for idx, option := range pending.Options {
		bullet := terminalIcon(terminalIconUnselected) + " "
		style := optionStyle
		if idx == m.interaction.cursor {
			bullet = terminalIcon(terminalIconSelected) + " "
			style = selectedStyle
		}
		number := fmt.Sprintf("%d.", idx+1)
		numberRendered := dimStyle.Render(number)
		labelWidth := max(optionWidth-lipgloss.Width(number)-1-lipgloss.Width(bullet), 8)
		wrapped := wrapTranscriptText(strings.TrimSpace(option), labelWidth)
		for lineIdx, line := range wrapped {
			if lineIdx == 0 {
				lines = append(lines, centerInlinePromptLine(numberRendered+" "+style.Render(bullet+line), width))
				continue
			}
			continuation := strings.Repeat(" ", lipgloss.Width(number)+1)
			lines = append(lines, centerInlinePromptLine(continuation+style.Render(strings.Repeat(" ", lipgloss.Width(bullet))+line), width))
		}
	}

	lines = append(lines, bottomSep)
	return strings.Join(lines, "\n")
}

func questionPromptPanelHeight(m Model, width int) int {
	panel := renderInlineQuestionPrompt(m, width)
	if strings.TrimSpace(panel) == "" {
		return 0
	}
	return lipgloss.Height(panel) + 2
}

func pendingQuestionPlanTitle(state events.SessionState, pending events.QuestionRequestState) string {
	if strings.TrimSpace(pending.Purpose) != events.QuestionPurposePlannerPlanDecision {
		return ""
	}
	planID := strings.TrimSpace(pending.PlanID)
	if planID == "" {
		return ""
	}
	plan := state.Plans[planID]
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.Title)
}

func questionPanelHints(pending events.QuestionRequestState) string {
	if pending.Multiple {
		return "↑/↓ choose · 1-9 quick select · enter confirm · one answer stored"
	}
	if count := len(pending.Options); count > 0 && count <= 9 {
		if count == 1 {
			return "1 choose · enter confirm"
		}
		return fmt.Sprintf("1-%d quick select · ↑/↓ choose · enter confirm", count)
	}
	return "↑/↓ choose · enter confirm"
}

func centerInlinePromptLine(content string, width int) string {
	pad := max((width-lipgloss.Width(content))/2, 0)
	return strings.Repeat(" ", pad) + content
}
