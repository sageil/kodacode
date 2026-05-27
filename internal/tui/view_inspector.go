package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

var inspectorTabs = []string{"Details", "Tools", "Tasks"}

func renderWideRightPanel(m Model, state events.SessionState, width int) string {
	body := renderWideInspectorBody(m, state, width)
	return body
}

func renderWideInspectorBody(m Model, state events.SessionState, width int) string {
	tasksSection := renderWideTasksList(m, state, width)
	agentsSection := renderWideAgentsList(m, state, width)
	switch {
	case tasksSection != "" && agentsSection != "":
		sep := lipgloss.NewStyle().
			Foreground(lipgloss.Color(lineTone(m))).
			Render(strings.Repeat("─", max(width, 1)))
		return tasksSection + "\n" + sep + "\n" + agentsSection
	case tasksSection != "":
		return tasksSection
	default:
		return agentsSection
	}
}

func renderInspectorPane(m Model, state events.SessionState, width int) string {
	tabs := renderInspectorTabs(m, width)
	return tabs + "\n\n" + m.inspector.body.View()
}

func renderInspectorBody(m Model, state events.SessionState, width int) string {
	detailTurnID := effectiveDetailTurnID(m, state)
	detailTurn := inspectorDetailTurn(state, m)
	var body string
	pendingSubmission := m.pendingInteractionSubmissionInFlight()

	switch {
	case !pendingSubmission && m.pendingExecution() != nil:
		body = renderPendingExecutionInspector(m, state, *m.pendingExecution(), width)
	case !pendingSubmission && m.pendingPermission() != nil:
		body = renderPendingPermissionInspector(m, state, *m.pendingPermission(), width)
	default:
		switch effectiveInspectorTab(m) {
		case inspectorTabTools:
			body = renderToolsListInspector(m, state, width)
		case inspectorTabTasks:
			body = renderTasksInspector(m, state, width)
		default:
			body = renderOverviewInspector(m, state, detailTurnID, detailTurn, width)
		}
	}
	return body
}

func renderInspectorTabs(m Model, width int) string {
	tabWidth := max(width/len(inspectorTabs), 1)
	lastTabWidth := max(width-tabWidth*(len(inspectorTabs)-1), 1)
	activeTab := effectiveInspectorTab(m)

	tabs := make([]string, 0, len(inspectorTabs))
	for idx, label := range inspectorTabs {
		w := tabWidth
		if idx == len(inspectorTabs)-1 {
			w = lastTabWidth
		}
		enabled := inspectorTabEnabled(m, idx)
		style := lipgloss.NewStyle().
			Width(w).
			Align(lipgloss.Center).
			Foreground(lipgloss.Color(colorFor(m.theme, "soft", softTextColor))).
			BorderBottom(true).
			BorderStyle(lipgloss.Border{Bottom: "─"}).
			BorderForeground(lipgloss.Color(lineTone(m)))
		if !enabled {
			style = style.Foreground(lipgloss.Color(lineTone(m)))
		}
		if enabled && idx == activeTab {
			style = style.
				Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
				Bold(true).
				BorderForeground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff")))
		}
		tabs = append(tabs, style.Render(label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}
