package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

type inspectorTaskRow struct {
	taskID     string
	label      string
	status     string
	active     bool
	treePrefix string
}

type inspectorTasksRender struct {
	Content     string
	LineTaskIDs map[int]string
}

func renderWideTasksList(m Model, state events.SessionState, width int) string {
	label := renderWidePaneTitle(m, "Tasks", "", width, colorFor(m.theme, "secondary", "#ff79c6"))
	rows := orderedWideTaskRows(state)
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, renderInspectorTaskRow(m, row, width))
	}
	return label + "\n" + strings.Join(lines, "\n")
}

func orderedWideTaskRows(state events.SessionState) []inspectorTaskRow {
	return orderedInspectorTaskRows(state)
}

func renderTasksInspector(m Model, state events.SessionState, width int) string {
	return renderTasksInspectorView(m, state, width).Content
}

func renderTasksInspectorView(m Model, state events.SessionState, width int) inspectorTasksRender {
	rows := orderedInspectorTaskRows(state)
	if len(rows) == 0 {
		body := emptyTasksInspectorBody(m)
		if isWideShell(m) {
			return inspectorTasksRender{
				Content: lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
					Render(body),
			}
		}
		return inspectorTasksRender{Content: renderInspectorBlock(m, "Tasks", body, width)}
	}
	lines := make([]string, 0, len(rows))
	lineTaskIDs := make(map[int]string, len(state.TaskOrder))
	for idx, row := range rows {
		lines = append(lines, renderInspectorTaskRow(m, row, width))
		lineTaskIDs[idx] = row.taskID
	}
	return inspectorTasksRender{
		Content:     strings.Join(lines, "\n"),
		LineTaskIDs: lineTaskIDs,
	}
}

func emptyTasksInspectorBody(m Model) string {
	return "No tasks in this session."
}

func currentTasksAgentID(m Model) string {
	if agentID := strings.TrimSpace(m.agentID); agentID != "" {
		return agentID
	}
	return "builder"
}

type inspectorTaskTreeNode struct {
	taskID   string
	task     *events.TaskState
	children []*inspectorTaskTreeNode
}

func orderedInspectorTaskRows(state events.SessionState) []inspectorTaskRow {
	if len(state.TaskOrder) == 0 {
		return nil
	}
	nodes := make(map[string]*inspectorTaskTreeNode, len(state.TaskOrder))
	order := make([]string, 0, len(state.TaskOrder))
	for _, taskID := range state.TaskOrder {
		task := state.Tasks[taskID]
		if task == nil || strings.TrimSpace(task.Title) == "" {
			continue
		}
		resolvedTaskID := strings.TrimSpace(task.TaskID)
		if resolvedTaskID == "" {
			resolvedTaskID = strings.TrimSpace(taskID)
		}
		if resolvedTaskID == "" {
			continue
		}
		nodes[resolvedTaskID] = &inspectorTaskTreeNode{
			taskID: resolvedTaskID,
			task:   task,
		}
		order = append(order, resolvedTaskID)
	}
	if len(order) == 0 {
		return nil
	}
	roots := make([]*inspectorTaskTreeNode, 0, len(order))
	for _, taskID := range order {
		node := nodes[taskID]
		if node == nil || node.task == nil {
			continue
		}
		parentTaskID := strings.TrimSpace(node.task.ParentTaskID)
		if parentTaskID == "" || parentTaskID == taskID {
			roots = append(roots, node)
			continue
		}
		parent := nodes[parentTaskID]
		if parent == nil {
			roots = append(roots, node)
			continue
		}
		parent.children = append(parent.children, node)
	}

	rows := make([]inspectorTaskRow, 0, len(order))
	visited := make(map[string]struct{}, len(order))
	var walk func(node *inspectorTaskTreeNode, branchState []bool)
	walk = func(node *inspectorTaskTreeNode, branchState []bool) {
		if node == nil {
			return
		}
		if _, ok := visited[node.taskID]; ok {
			return
		}
		visited[node.taskID] = struct{}{}
		rows = append(rows, inspectorTaskRow{
			taskID:     node.taskID,
			label:      strings.TrimSpace(node.task.Title),
			status:     inspectorTaskStatus(node.task),
			active:     strings.TrimSpace(node.task.Status) == events.TaskStatusInProgress,
			treePrefix: inspectorTaskTreePrefix(branchState),
		})
		for idx, child := range node.children {
			nextState := make([]bool, 0, len(branchState)+1)
			nextState = append(nextState, branchState...)
			nextState = append(nextState, idx == len(node.children)-1)
			walk(child, nextState)
		}
	}

	for _, root := range roots {
		walk(root, nil)
	}
	for _, taskID := range order {
		if node := nodes[taskID]; node != nil {
			walk(node, nil)
		}
	}
	return rows
}

func inspectorTaskTreePrefix(branchState []bool) string {
	if len(branchState) == 0 {
		return ""
	}
	return strings.Repeat("  ", len(branchState)-1) + " " + terminalIcon(terminalIconBranch) + " "
}

func renderInspectorTaskRow(m Model, row inspectorTaskRow, width int) string {
	labelColor := colorFor(m.theme, "text", "#ecf0ff")
	selected := strings.TrimSpace(row.taskID) != "" && strings.TrimSpace(row.taskID) == strings.TrimSpace(m.selection.taskID)
	if row.active {
		labelColor = colorFor(m.theme, "primary", "#7cc7ff")
	}
	if selected {
		labelColor = colorFor(m.theme, "primary", "#7cc7ff")
	}
	prefixText := row.treePrefix
	statusBadge := taskStatusBadge(row.status)
	prefixRendered := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render(prefixText)
	statusRendered := lipgloss.NewStyle().
		Foreground(taskStatusColor(m.theme, row.status)).
		Render(statusBadge)
	labelWidth := max(width-lipgloss.Width(prefixText)-lipgloss.Width(statusBadge)-1, 1)
	labelRendered := lipgloss.NewStyle().
		Foreground(lipgloss.Color(labelColor)).
		Render(truncateEnd(row.label, labelWidth))
	return prefixRendered + statusRendered + " " + labelRendered
}

func taskStatusBadge(status string) string {
	switch strings.TrimSpace(status) {
	case "done":
		return "[" + terminalIcon(terminalIconCheck) + "]"
	case "running":
		return "[>]"
	case "blocked":
		return "[!]"
	default:
		return "[ ]"
	}
}

func inspectorTaskStatus(task *events.TaskState) string {
	if task == nil {
		return "waiting"
	}
	switch strings.TrimSpace(task.Status) {
	case events.TaskStatusCompleted:
		return "done"
	case events.TaskStatusBlocked:
		return "blocked"
	case events.TaskStatusInProgress:
		return "running"
	default:
		return "waiting"
	}
}
