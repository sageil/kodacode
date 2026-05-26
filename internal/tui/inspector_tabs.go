package tui

import "strings"

const (
	inspectorTabDetails = iota
	inspectorTabTools
	inspectorTabTasks
)

func inspectorTasksTabEnabled(m Model) bool {
	return strings.TrimSpace(currentTasksAgentID(m)) == "engineer"
}

func inspectorTabEnabled(m Model, tab int) bool {
	if tab < 0 || tab >= len(inspectorTabs) {
		return false
	}
	if tab == inspectorTabTasks {
		return inspectorTasksTabEnabled(m)
	}
	return true
}

func effectiveInspectorTab(m Model) int {
	if len(inspectorTabs) == 0 {
		return 0
	}

	tab := m.inspector.tab
	if tab < 0 {
		tab = inspectorTabDetails
	}
	if tab >= len(inspectorTabs) {
		tab = len(inspectorTabs) - 1
	}
	if inspectorTabEnabled(m, tab) {
		return tab
	}

	for prev := tab - 1; prev >= 0; prev-- {
		if inspectorTabEnabled(m, prev) {
			return prev
		}
	}
	for next := tab + 1; next < len(inspectorTabs); next++ {
		if inspectorTabEnabled(m, next) {
			return next
		}
	}
	return inspectorTabDetails
}

func stepInspectorTab(m Model, delta int) int {
	current := effectiveInspectorTab(m)
	if delta == 0 {
		return current
	}
	for next := current + delta; next >= 0 && next < len(inspectorTabs); next += delta {
		if inspectorTabEnabled(m, next) {
			return next
		}
	}
	return current
}

func (m *Model) syncInspectorTabAvailability() bool {
	if m == nil {
		return false
	}
	next := effectiveInspectorTab(*m)
	if m.inspector.tab == next {
		return false
	}
	m.inspector.tab = next
	return true
}
