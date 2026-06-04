package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

type inspectorToolLineActionKind string

const (
	inspectorToolLineToggleGroup inspectorToolLineActionKind = "toggle_group"
	inspectorToolLineOpenCall    inspectorToolLineActionKind = "open_call"
)

type inspectorToolLineAction struct {
	Kind    inspectorToolLineActionKind
	GroupID string
	Target  inspectorToolTarget
}

type inspectorToolsRender struct {
	Content     string
	LineActions map[int]inspectorToolLineAction
}

func renderGroupedToolsInspector(m Model, state events.SessionState, width int) inspectorToolsRender {
	return renderGroupedToolsInspectorForSession(
		m,
		state,
		state.SessionID,
		filterPendingQuestionToolRefs(m, orderedAllSessionToolCallRefs(state)),
		width,
		"No tool calls in this session.",
		0,
	)
}

func renderGroupedToolsInspectorForSession(m Model, state events.SessionState, sessionID string, refs []sessionToolCallRef, width int, emptyLabel string, depth int) inspectorToolsRender {
	lines, lineActions := renderGroupedToolsInspectorLines(m, state, sessionID, refs, width, depth)
	if len(lines) == 0 {
		return inspectorToolsRender{
			Content: lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
				Render(emptyLabel),
		}
	}
	return inspectorToolsRender{
		Content:     strings.Join(lines, "\n"),
		LineActions: lineActions,
	}
}

func renderGroupedToolsInspectorLines(m Model, state events.SessionState, sessionID string, refs []sessionToolCallRef, width int, depth int) ([]string, map[int]inspectorToolLineAction) {
	groups := buildInspectorToolGroups(state, refs)
	if len(groups) == 0 {
		return nil, nil
	}
	lines := make([]string, 0, len(groups)*4)
	lineActions := make(map[int]inspectorToolLineAction, len(groups)*4)
	lineIndex := 0
	for _, group := range groups {
		if title, ok := flattenedUsedToolGroupTitle(state, group); ok {
			ref := group.Refs[0]
			lines = append(lines, renderInspectorToolGroupHeader(m, title, group.Status, width, false, depth))
			lineActions[lineIndex] = inspectorToolLineAction{
				Kind: inspectorToolLineOpenCall,
				Target: inspectorToolTarget{
					SessionID: sessionID,
					Ref:       ref,
				},
			}
			lineIndex++
			continue
		}
		groupID := inspectorToolGroupID(sessionID, group)
		lines = append(lines, renderInspectorToolGroupHeader(m, inspectorToolGroupTitle(group), group.Status, width, m.isInspectorToolGroupCollapsed(groupID), depth))
		lineActions[lineIndex] = inspectorToolLineAction{
			Kind:    inspectorToolLineToggleGroup,
			GroupID: groupID,
		}
		lineIndex++
		if m.isInspectorToolGroupCollapsed(groupID) {
			continue
		}
		if group.Kind == wideToolGroupTaskList {
			taskLines, taskActions := renderInspectorTaskListGroupLines(m, state, sessionID, group, width, depth)
			for childLine, action := range taskActions {
				lineActions[lineIndex+childLine] = action
			}
			lines = append(lines, taskLines...)
			lineIndex += len(taskLines)
			continue
		}
		for _, ref := range group.Refs {
			_, call := sessionToolCall(state, ref)
			if call == nil {
				continue
			}
			rendered := renderWideToolGroupItemLine(
				m,
				state.WorkspaceRoot,
				ref,
				call,
				max(width-len(inspectorTreeIndent(depth)), 1),
				selectedToolMatchesSession(m, sessionID, ref),
			)
			renderedLines := indentInspectorLines(strings.Split(rendered, "\n"), depth)
			lines = append(lines, renderedLines...)
			for range renderedLines {
				lineActions[lineIndex] = inspectorToolLineAction{
					Kind: inspectorToolLineOpenCall,
					Target: inspectorToolTarget{
						SessionID: sessionID,
						Ref:       ref,
					},
				}
				lineIndex++
			}
		}
	}
	return lines, lineActions
}

func renderInspectorTaskListGroupLines(m Model, state events.SessionState, sessionID string, group wideToolTranscriptGroup, width int, depth int) ([]string, map[int]inspectorToolLineAction) {
	lines := make([]string, 0, len(group.Refs)*4)
	lineActions := make(map[int]inspectorToolLineAction, len(group.Refs)*4)
	lineIndex := 0
	for _, ref := range group.Refs {
		_, call := sessionToolCall(state, ref)
		if call == nil {
			continue
		}
		tasks, ok := parseTaskToolViewListOutput(call.Output)
		if !ok {
			rendered := renderWideToolGroupItemLine(
				m,
				state.WorkspaceRoot,
				ref,
				call,
				max(width-len(inspectorTreeIndent(depth)), 1),
				selectedToolMatchesSession(m, sessionID, ref),
			)
			renderedLines := indentInspectorLines(strings.Split(rendered, "\n"), depth)
			lines = append(lines, renderedLines...)
			for range renderedLines {
				lineActions[lineIndex] = inspectorToolLineAction{
					Kind: inspectorToolLineOpenCall,
					Target: inspectorToolTarget{
						SessionID: sessionID,
						Ref:       ref,
					},
				}
				lineIndex++
			}
			continue
		}
		if len(tasks) == 0 {
			rendered := renderTaskListGroupItemLine(m, "No tasks", "", max(width-len(inspectorTreeIndent(depth)), 1), selectedToolMatchesSession(m, sessionID, ref))
			renderedLines := indentInspectorLines(strings.Split(rendered, "\n"), depth)
			lines = append(lines, renderedLines...)
			for range renderedLines {
				lineActions[lineIndex] = inspectorToolLineAction{
					Kind: inspectorToolLineOpenCall,
					Target: inspectorToolTarget{
						SessionID: sessionID,
						Ref:       ref,
					},
				}
				lineIndex++
			}
			continue
		}
		for _, task := range tasks {
			rendered := renderTaskListGroupItemLine(
				m,
				taskToolListItemLabel(task),
				taskToolListItemStatus(task),
				max(width-len(inspectorTreeIndent(depth)), 1),
				selectedToolMatchesSession(m, sessionID, ref),
			)
			renderedLines := indentInspectorLines(strings.Split(rendered, "\n"), depth)
			lines = append(lines, renderedLines...)
			for range renderedLines {
				lineActions[lineIndex] = inspectorToolLineAction{
					Kind: inspectorToolLineOpenCall,
					Target: inspectorToolTarget{
						SessionID: sessionID,
						Ref:       ref,
					},
				}
				lineIndex++
			}
		}
	}
	return lines, lineActions
}

func buildInspectorToolGroups(state events.SessionState, refs []sessionToolCallRef) []wideToolTranscriptGroup {
	if len(refs) == 0 {
		return nil
	}

	refsByTurn := make(map[string][]sessionToolCallRef, len(state.TurnOrder))
	for _, ref := range refs {
		if strings.TrimSpace(ref.TurnID) == "" || strings.TrimSpace(ref.CallID) == "" {
			continue
		}
		refsByTurn[ref.TurnID] = append(refsByTurn[ref.TurnID], ref)
	}

	groups := make([]wideToolTranscriptGroup, 0, len(refs))
	for _, turnID := range orderedSessionTurnIDs(state) {
		turnRefs := refsByTurn[turnID]
		if len(turnRefs) == 0 {
			continue
		}
		groups = append(groups, buildWideToolTranscriptGroups(state, turnRefs)...)
	}
	return groups
}

func renderInspectorToolGroupHeader(m Model, title, status string, width int, collapsed bool, depth int) string {
	indicator := m.terminalIcon(terminalIconExpanded)
	if collapsed {
		indicator = m.terminalIcon(terminalIconCollapsed)
	}
	indent := inspectorTreeIndent(depth)
	return indent + renderWideToolHeader(m, indicator+" "+title, status, max(width-len(indent), 1))
}

func inspectorToolGroupTitle(group wideToolTranscriptGroup) string {
	return toolOutcomeGroupLabel(group.Kind, group.Status)
}

func inspectorToolGroupID(sessionID string, group wideToolTranscriptGroup) string {
	if len(group.Refs) == 0 {
		return strings.TrimSpace(sessionID) + ":" + string(group.Kind)
	}
	first := group.Refs[0]
	last := group.Refs[len(group.Refs)-1]
	return strings.Join([]string{
		strings.TrimSpace(sessionID),
		string(group.Kind),
		strings.TrimSpace(first.TurnID),
		strings.TrimSpace(first.CallID),
		strings.TrimSpace(last.CallID),
	}, ":")
}

func (m Model) isInspectorToolGroupCollapsed(groupID string) bool {
	if m.inspector.collapsedToolGroups == nil {
		return false
	}
	return m.inspector.collapsedToolGroups[groupID]
}

func (m *Model) toggleInspectorToolGroup(groupID string) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return
	}
	if m.inspector.collapsedToolGroups == nil {
		m.inspector.collapsedToolGroups = make(map[string]bool)
	}
	if m.inspector.collapsedToolGroups[groupID] {
		delete(m.inspector.collapsedToolGroups, groupID)
		return
	}
	m.inspector.collapsedToolGroups[groupID] = true
}

func renderInspectorInfoLines(m Model, label string, width int, depth int) []string {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	indent := inspectorTreeIndent(depth)
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	wrapped := wrapStructuredText(label, max(width-len(indent), 1))
	if len(wrapped) == 0 {
		wrapped = []string{label}
	}
	lines := make([]string, 0, len(wrapped))
	for _, line := range wrapped {
		lines = append(lines, indent+style.Render(line))
	}
	return lines
}

func inspectorTreeIndent(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("  ", depth)
}

func indentInspectorLines(lines []string, depth int) []string {
	indent := inspectorTreeIndent(depth)
	if indent == "" || len(lines) == 0 {
		return lines
	}
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		rendered = append(rendered, indent+line)
	}
	return rendered
}
