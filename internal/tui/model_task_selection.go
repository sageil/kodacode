package tui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

func (m *Model) moveSelectedInspectorTask(delta int) tea.Cmd {
	state := m.projector.Snapshot()
	if strings.TrimSpace(m.selection.taskID) == "" && delta >= 0 {
		if taskID := inspectorPreferredTaskID(*m, state); taskID != "" {
			return m.selectTask(taskID)
		}
	}
	return m.moveSelectedTaskAcross(inspectorVisibleTaskIDs(*m, state), delta, false)
}

func (m *Model) openSelectedInspectorTaskDialog() tea.Cmd {
	state := m.projector.Snapshot()
	taskID, ok := inspectorActiveTaskID(*m, state)
	if !ok {
		return nil
	}
	m.selectTask(taskID)
	return m.openTaskDialog(taskID)
}

func (m *Model) moveSelectedTaskAcross(taskIDs []string, delta int, clearOnEmpty bool) tea.Cmd {
	if len(taskIDs) == 0 {
		if clearOnEmpty {
			m.clearSelectedTask()
		}
		return nil
	}

	selected := strings.TrimSpace(m.selection.taskID)
	index := indexOfString(taskIDs, selected)
	switch {
	case index < 0 && delta < 0:
		index = len(taskIDs) - 1
	case index < 0:
		index = 0
	default:
		index = max(min(index+delta, len(taskIDs)-1), 0)
	}
	return m.selectTask(taskIDs[index])
}

func (m *Model) selectTask(taskID string) tea.Cmd {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		m.clearSelectedTask()
		return nil
	}
	if strings.TrimSpace(m.selection.taskID) == taskID {
		return nil
	}
	m.selection.taskID = taskID
	m.syncInspectorBody(false)
	m.jumpInspectorToSelectedTask()
	return nil
}

func (m *Model) openTaskDialog(taskID string) tea.Cmd {
	state := m.projector.Snapshot()
	taskID = strings.TrimSpace(taskID)
	task := state.Tasks[taskID]
	if task == nil {
		return nil
	}
	dialog := newTaskDetailDialog(*m, state, taskID, task)
	width, height := dialogRenderSize(*m, state)
	dialog.SetFrame(width, height)
	m.dialog = dialog
	return nil
}

func (m *Model) syncTaskDetailDialog() {
	if m == nil {
		return
	}
	dialog, ok := m.dialog.(*taskDetailDialog)
	if !ok {
		return
	}
	state := m.projector.Snapshot()
	task := state.Tasks[strings.TrimSpace(dialog.taskID)]
	if task == nil {
		m.dialog = nil
		return
	}
	dialog.ApplyTheme(m.theme)
	dialog.SetFrame(dialogRenderSize(*m, state))
	dialog.Sync(*m, state, dialog.taskID, task)
}

func (m *Model) clearSelectedTask() {
	if strings.TrimSpace(m.selection.taskID) == "" {
		return
	}
	m.selection.taskID = ""
	m.syncInspectorBody(false)
}

func (m *Model) jumpInspectorToSelectedTask() {
	if m == nil {
		return
	}
	taskID := strings.TrimSpace(m.selection.taskID)
	if taskID == "" {
		return
	}
	if line, ok := inspectorLineForTaskID(m.inspector.taskLines, taskID); ok {
		m.inspector.body.GotoLine(line)
	}
}

func syncTaskSelectionStateWithState(m *Model, state events.SessionState) {
	if m == nil {
		return
	}
	taskID := strings.TrimSpace(m.selection.taskID)
	if taskID == "" {
		m.selection.taskID = ""
		return
	}
	if state.Tasks[taskID] == nil || indexOfString(orderedSessionTaskIDs(state), taskID) < 0 {
		m.selection.taskID = ""
	}
}

func inspectorActiveTaskID(m Model, state events.SessionState) (string, bool) {
	taskIDs := inspectorVisibleTaskIDs(m, state)
	if len(taskIDs) == 0 {
		return "", false
	}
	selected := strings.TrimSpace(m.selection.taskID)
	if index := indexOfString(taskIDs, selected); index >= 0 {
		return taskIDs[index], true
	}
	if preferred := inspectorPreferredTaskID(m, state); preferred != "" {
		return preferred, true
	}
	return taskIDs[0], true
}

func inspectorPreferredTaskID(m Model, state events.SessionState) string {
	rows := orderedInspectorTaskRows(m, state)
	if len(rows) == 0 {
		return ""
	}
	preferred := ""
	for _, row := range rows {
		if !row.active {
			continue
		}
		preferred = strings.TrimSpace(row.taskID)
	}
	if preferred != "" {
		return preferred
	}
	return strings.TrimSpace(rows[0].taskID)
}

func inspectorVisibleTaskIDs(m Model, state events.SessionState) []string {
	if len(m.inspector.taskLines) > 0 {
		lines := make([]int, 0, len(m.inspector.taskLines))
		for line := range m.inspector.taskLines {
			lines = append(lines, line)
		}
		sort.Ints(lines)
		taskIDs := make([]string, 0, len(lines))
		for _, line := range lines {
			taskID := strings.TrimSpace(m.inspector.taskLines[line])
			if taskID == "" {
				continue
			}
			taskIDs = append(taskIDs, taskID)
		}
		if len(taskIDs) > 0 {
			return taskIDs
		}
	}

	rows := orderedInspectorTaskRows(m, state)
	if len(rows) == 0 {
		return nil
	}
	taskIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if taskID := strings.TrimSpace(row.taskID); taskID != "" {
			taskIDs = append(taskIDs, taskID)
		}
	}
	return taskIDs
}

func orderedSessionTaskIDs(state events.SessionState) []string {
	if len(state.TaskOrder) == 0 {
		return nil
	}
	taskIDs := make([]string, 0, len(state.TaskOrder))
	for _, taskID := range state.TaskOrder {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" || state.Tasks[taskID] == nil {
			continue
		}
		taskIDs = append(taskIDs, taskID)
	}
	return taskIDs
}

func inspectorLineForTaskID(lines map[int]string, taskID string) (int, bool) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || len(lines) == 0 {
		return 0, false
	}
	matching := make([]int, 0, 1)
	for line, candidate := range lines {
		if strings.TrimSpace(candidate) == taskID {
			matching = append(matching, line)
		}
	}
	if len(matching) == 0 {
		return 0, false
	}
	sort.Ints(matching)
	return matching[0], true
}
