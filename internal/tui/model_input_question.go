package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

func (m Model) handleQuestionInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	pending := m.pendingQuestion()
	if pending == nil {
		return m, nil, false
	}
	if m.interactionResolutionInFlight() {
		return m, nil, true
	}

	maxChoice := max(len(pending.Options)-1, 0)
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
		updated, cmd := m.startQuestionResolution(0)
		return updated, cmd, true
	case "2":
		updated, cmd := m.startQuestionResolution(1)
		return updated, cmd, true
	case "3":
		updated, cmd := m.startQuestionResolution(2)
		return updated, cmd, true
	case "4":
		updated, cmd := m.startQuestionResolution(3)
		return updated, cmd, true
	case "5":
		updated, cmd := m.startQuestionResolution(4)
		return updated, cmd, true
	case "6":
		updated, cmd := m.startQuestionResolution(5)
		return updated, cmd, true
	case "7":
		updated, cmd := m.startQuestionResolution(6)
		return updated, cmd, true
	case "8":
		updated, cmd := m.startQuestionResolution(7)
		return updated, cmd, true
	case "9":
		updated, cmd := m.startQuestionResolution(8)
		return updated, cmd, true
	case "enter":
		updated, cmd := m.startQuestionResolution(m.interaction.cursor)
		return updated, cmd, true
	default:
		return m, nil, false
	}
}

func (m Model) startQuestionResolution(choice int) (tea.Model, tea.Cmd) {
	pending := m.pendingQuestion()
	if pending == nil {
		return m, nil
	}
	answer := questionChoice(choice, pending)
	if answer == "" {
		return m, nil
	}
	m.busy = true
	m.interaction.resolveReq = pending.QuestionID
	m.userText = ""
	turnID := m.turnID
	if strings.TrimSpace(pending.TurnID) != "" {
		turnID = strings.TrimSpace(pending.TurnID)
	}
	return m, tea.Batch(
		answerQuestionCmd(
			m.ctx,
			m.controller,
			m.sessionID,
			turnID,
			pending.QuestionID,
			"",
			answer,
			m.skillIDs,
		),
		m.ensureAnimTicking(),
	)
}

func questionChoice(choice int, pending *events.QuestionRequestState) string {
	if pending == nil || choice < 0 || choice >= len(pending.Options) {
		return ""
	}
	return pending.Options[choice]
}
