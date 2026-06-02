package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderComposerBar(m Model, state events.SessionState, width int) string {
	disabledMessage := strings.TrimSpace(m.composerDisabledMessage(state))
	disabled := disabledMessage != "" && !m.hasPendingInteraction()
	if isWideShell(m) {
		content := strings.TrimRight(m.composer.View(), "\n")
		if disabled {
			content = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "warning", "#ffd28f"))).
				Render(disabledMessage)
		}
		if m.hasPendingInteraction() {
			content = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "warning", "#ffd28f"))).
				Render(composerBlockedMessage(m))
		}
		parts := make([]string, 0, 2)
		if divider := renderComposerActivityStrip(m, state, width, lineTone(m)); divider != "" {
			parts = append(parts, divider)
		}
		parts = append(parts, content)
		contentBlock := lipgloss.NewStyle().
			Width(max(width, 1)).
			Render(strings.Join(parts, "\n"))
		return contentBlock
	}

	subtitle := "enter submits • shift+enter newline • ctrl+w workflow • @ include path • ctrl+e editor"
	border := lineTone(m)
	focused := m.chrome.focus == focusComposer
	if m.hasPendingInteraction() {
		subtitle = composerBlockedMessage(m)
		border = colorFor(m.theme, "warning", "#ffd28f")
	} else if disabled {
		subtitle = disabledMessage
		border = colorFor(m.theme, "warning", "#ffd28f")
	} else if strings.TrimSpace(m.composerState.err) != "" {
		border = colorFor(m.theme, "error", "#ff9aa6")
	}
	if focused {
		border = colorFor(m.theme, "primary", "#7cc7ff")
		if strings.TrimSpace(m.composerState.err) != "" && !m.hasPendingInteraction() {
			border = colorFor(m.theme, "error", "#ff9aa6")
		}
	}

	contentWidth := max(width, 1)
	content := strings.TrimRight(m.composer.View(), "\n")
	if disabled {
		content = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "warning", "#ffd28f"))).
			Render(truncateEnd(disabledMessage, contentWidth))
	}
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render(truncateEnd(subtitle, contentWidth))
	body := lipgloss.NewStyle().
		Width(contentWidth).
		Render(header + "\n" + content)

	parts := make([]string, 0, 2)
	if topBorder := renderComposerActivityStrip(m, state, contentWidth, border); topBorder != "" {
		parts = append(parts, topBorder)
	}
	parts = append(parts, body)
	return strings.Join(parts, "\n")
}

func renderComposerActivityStrip(m Model, state events.SessionState, width int, borderColor string) string {
	width = max(width, 1)
	lineStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(borderColor))
	activity, ok := composerActivityStripStateFor(m, state)
	if !ok || strings.TrimSpace(activity.Label) == "" {
		return ""
	}

	prefix := lineStyle.Render("── ")
	prefixWidth := lipgloss.Width(prefix)
	innerWidth := max(width-prefixWidth, 1)
	spinnerText := ""
	if activity.Spinning {
		spinnerText = renderLiveSpinner(m) + " "
	}
	labelWidth := max(innerWidth-lipgloss.Width(spinnerText), 1)
	label := truncateEnd(strings.TrimSpace(activity.Label), labelWidth)
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(activity.LabelColor)).
		Bold(true)
	left := labelStyle.Render(label)
	if activity.Spinning {
		left = spinnerText + left
	}

	inner := left
	if metaText := strings.TrimSpace(activity.MetaText); metaText != "" {
		metaWidth := innerWidth - lipgloss.Width(left) - 1
		if metaWidth > 0 {
			metaRendered := lipgloss.NewStyle().
				Foreground(lipgloss.Color(activity.MetaColor)).
				Render(truncateEnd(metaText, metaWidth))
			inner = left + " " + metaRendered
		}
	}

	remaining := width - prefixWidth - lipgloss.Width(inner)
	if remaining < 0 {
		remaining = 0
	}
	return prefix + inner + lineStyle.Render(strings.Repeat("─", remaining))
}

type composerActivityStripState struct {
	Label      string
	LabelColor string
	MetaText   string
	MetaColor  string
	Spinning   bool
}

func composerActivityStripStateFor(m Model, state events.SessionState) (composerActivityStripState, bool) {
	if active, label := m.liveTurnSpinnerState(state); active && strings.TrimSpace(label) != "" {
		metaText, metaColor := transcriptStatusMeta(m, state)
		return composerActivityStripState{
			Label:      label,
			LabelColor: colorFor(m.theme, "primary", "#7cc7ff"),
			MetaText:   metaText,
			MetaColor:  metaColor,
			Spinning:   true,
		}, true
	}
	turn := currentTurn(state, m.turnID)
	if turn != nil && turn.Status == events.TurnStatusCanceled {
		return composerActivityStripState{
			Label:      "Cancelled",
			LabelColor: colorFor(m.theme, "warning", "#ffd28f"),
		}, true
	}
	if turn != nil && turn.Status == events.TurnStatusFailed {
		return composerActivityStripState{}, false
	}
	if len(m.skillIDs) > 0 && !m.busy && !m.hasPendingInteraction() {
		return composerActivityStripState{
			Label:      "Skills",
			LabelColor: colorFor(m.theme, "primary", "#7cc7ff"),
			MetaText:   strings.Join(m.skillIDs, ", "),
			MetaColor:  colorFor(m.theme, "subtext", "#9da8ca"),
		}, true
	}
	if focusPaths := m.orderedPendingFocusPaths(); len(focusPaths) > 0 && !m.busy && !m.hasPendingInteraction() {
		labels := make([]string, 0, len(focusPaths))
		for _, focusPath := range focusPaths {
			labels = append(labels, focusPath.Path)
		}
		return composerActivityStripState{
			Label:      "Focus",
			LabelColor: colorFor(m.theme, "primary", "#7cc7ff"),
			MetaText:   strings.Join(labels, ", "),
			MetaColor:  colorFor(m.theme, "subtext", "#9da8ca"),
		}, true
	}
	return composerActivityStripState{}, false
}

func composerBlockedMessage(m Model) string {
	if m.pendingInteractionSubmissionInFlight() {
		if handoff := activeDelegatedHandoff(m.projector.CurrentState(), m); handoff != nil {
			agentID := strings.TrimSpace(handoff.ChildAgentID)
			if agentID == "" {
				agentID = "delegate"
			}
			return "waiting for " + agentID + " to finish"
		}
		return "waiting for the runtime to continue"
	}
	if m.pendingQuestion() != nil {
		return "answer the question above to continue"
	}
	if m.pendingExecution() != nil || m.pendingPermission() != nil || m.pendingDelegatedPermission() != nil {
		return "resolve the request above to continue"
	}
	return "resolve the request in the inspector to continue"
}
