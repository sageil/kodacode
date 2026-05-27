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
	inspectorToolLineOpenHandoff inspectorToolLineActionKind = "open_handoff"
)

type inspectorToolLineAction struct {
	Kind    inspectorToolLineActionKind
	GroupID string
	Target  inspectorToolTarget
	Handoff inspectorHandoffTarget
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
	latestDelegatedRefs := latestDelegatedInspectorRefs(state, refs)

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
		if group.Kind == wideToolGroupUsed {
			delegateOnlyLines, delegateOnlyActions, handled := renderInspectorDelegateOnlyGroup(m, state, sessionID, group, width, depth, latestDelegatedRefs)
			if handled {
				for childLine, action := range delegateOnlyActions {
					lineActions[lineIndex+childLine] = action
				}
				lines = append(lines, delegateOnlyLines...)
				lineIndex += len(delegateOnlyLines)
				continue
			}
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
			turn, call := sessionToolCall(state, ref)
			if call == nil {
				continue
			}
			if shouldSkipDedupedDelegatedInspectorRef(turn, ref, call, latestDelegatedRefs) {
				continue
			}
			if delegatedLines, delegatedActions, ok := renderDelegatedInspectorRoot(m, sessionID, turn, ref, call, width, depth+1); ok {
				for childLine, action := range delegatedActions {
					lineActions[lineIndex+childLine] = action
				}
				lines = append(lines, delegatedLines...)
				lineIndex += len(delegatedLines)
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

func renderInspectorDelegateOnlyGroup(m Model, state events.SessionState, sessionID string, group wideToolTranscriptGroup, width int, depth int, latestDelegatedRefs map[sessionToolCallRef]struct{}) ([]string, map[int]inspectorToolLineAction, bool) {
	if len(group.Refs) == 0 {
		return nil, nil, false
	}
	lines := make([]string, 0, len(group.Refs)*4)
	lineActions := make(map[int]inspectorToolLineAction, len(group.Refs)*4)
	lineIndex := 0
	for _, ref := range group.Refs {
		turn, call := sessionToolCall(state, ref)
		if call == nil {
			return nil, nil, false
		}
		if shouldSkipDedupedDelegatedInspectorRef(turn, ref, call, latestDelegatedRefs) {
			continue
		}
		renderedLines, renderedActions, ok := renderDelegatedInspectorRoot(m, sessionID, turn, ref, call, width, depth)
		if !ok {
			return nil, nil, false
		}
		for childLine, action := range renderedActions {
			lineActions[lineIndex+childLine] = action
		}
		lines = append(lines, renderedLines...)
		lineIndex += len(renderedLines)
	}
	return lines, lineActions, true
}

func latestDelegatedInspectorRefs(state events.SessionState, refs []sessionToolCallRef) map[sessionToolCallRef]struct{} {
	if len(refs) == 0 {
		return nil
	}
	latest := make(map[string]sessionToolCallRef)
	for _, ref := range refs {
		turn, call := sessionToolCall(state, ref)
		handoff := delegateHandoffForCall(turn, call)
		if handoff == nil || strings.TrimSpace(handoff.HandoffID) == "" {
			continue
		}
		latest[strings.TrimSpace(handoff.HandoffID)] = ref
	}
	if len(latest) == 0 {
		return nil
	}
	out := make(map[sessionToolCallRef]struct{}, len(latest))
	for _, ref := range latest {
		out[ref] = struct{}{}
	}
	return out
}

func shouldSkipDedupedDelegatedInspectorRef(turn *events.TurnState, ref sessionToolCallRef, call *events.ToolCallState, latest map[sessionToolCallRef]struct{}) bool {
	if len(latest) == 0 || !isDelegateToolCall(call) {
		return false
	}
	handoff := delegateHandoffForCall(turn, call)
	if handoff == nil || strings.TrimSpace(handoff.HandoffID) == "" {
		return false
	}
	_, ok := latest[ref]
	return !ok
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

func renderDelegatedInspectorRoot(m Model, sessionID string, turn *events.TurnState, ref sessionToolCallRef, call *events.ToolCallState, width int, depth int) ([]string, map[int]inspectorToolLineAction, bool) {
	handoff := delegateHandoffForCall(turn, call)
	if handoff == nil || strings.TrimSpace(handoff.ChildSessionID) == "" || strings.TrimSpace(handoff.ChildTurnID) == "" {
		return nil, nil, false
	}

	groupID := delegatedInspectorGroupID(handoff)
	lines := []string{renderInspectorToolGroupHeader(m, delegatedInspectorGroupTitle(handoff, call), delegatedInspectorGroupStatus(handoff), width, m.isInspectorToolGroupCollapsed(groupID), depth)}
	lineActions := map[int]inspectorToolLineAction{
		0: {
			Kind:    inspectorToolLineToggleGroup,
			GroupID: groupID,
		},
	}
	if m.isInspectorToolGroupCollapsed(groupID) {
		return lines, lineActions, true
	}

	lineIndex := len(lines)
	if taskLines, taskActions := renderDelegatedInspectorTaskLines(m, inspectorHandoffTarget{
		SessionID: normalizeToolTargetSessionID(m.sessionID, sessionID),
		TurnID:    strings.TrimSpace(ref.TurnID),
		HandoffID: strings.TrimSpace(handoff.HandoffID),
	}, handoff, call, width, depth+1); len(taskLines) > 0 {
		lines = append(lines, taskLines...)
		for childLine, action := range taskActions {
			lineActions[lineIndex+childLine] = action
		}
		lineIndex += len(taskLines)
	}

	childSessionID := normalizeToolTargetSessionID(m.sessionID, handoff.ChildSessionID)
	childState, ok := m.delegatedSnapshot(handoff.ChildSessionID)
	if !ok {
		lines = append(lines, renderInspectorInfoLines(m, delegatedInspectorLoadingLabel(m, handoff), width, depth+1)...)
		return lines, lineActions, true
	}

	childRefs := orderedDelegatedChildToolCallRefs(childState)
	if len(childRefs) == 0 {
		label := delegatedInspectorNoToolsLabel(handoff)
		if m.delegatedSnapshots.loading[strings.TrimSpace(handoff.ChildSessionID)] {
			label = delegatedInspectorLoadingLabel(m, handoff)
		}
		lines = append(lines, renderInspectorInfoLines(m, label, width, depth+1)...)
		return lines, lineActions, true
	}
	childLines, childActions := renderGroupedToolsInspectorLines(m, childState, childSessionID, childRefs, width, depth+1)
	for childLine, action := range childActions {
		lineActions[lineIndex+childLine] = action
	}
	lines = append(lines, childLines...)
	return lines, lineActions, true
}

func orderedDelegatedChildToolCallRefs(state events.SessionState) []sessionToolCallRef {
	return orderedAllSessionToolCallRefs(state)
}

func delegatedInspectorLoadingLabel(m Model, handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return "Delegated tool activity has not loaded yet."
	}
	if m.delegatedSnapshots.loading[strings.TrimSpace(handoff.ChildSessionID)] {
		if agentID := strings.TrimSpace(handoff.ChildAgentID); agentID != "" {
			return "Loading " + agentID + " tool calls..."
		}
		return "Loading delegated tool calls..."
	}
	if agentID := strings.TrimSpace(handoff.ChildAgentID); agentID != "" {
		return agentID + " tool activity has not loaded yet."
	}
	return "Delegated tool activity has not loaded yet."
}

func delegatedInspectorNoToolsLabel(handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return "This delegated child has not used any tools."
	}
	switch handoff.Status {
	case events.AgentResultStatusCompleted:
		return "This delegated child completed without using tools."
	case events.AgentResultStatusFailed:
		return "This delegated child failed before using any tools."
	case events.AgentResultStatusPendingPermission, events.AgentResultStatusPendingQuestion:
		return "This delegated child has not used any tools yet."
	default:
		return "This delegated child has not used any tools yet."
	}
}

func delegatedInspectorGroupID(handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return "delegated"
	}
	return strings.Join([]string{
		"delegated",
		strings.TrimSpace(handoff.ParentSessionID),
		strings.TrimSpace(handoff.ParentTurnID),
		strings.TrimSpace(handoff.HandoffID),
	}, ":")
}

func delegatedInspectorGroupTitle(handoff *events.AgentHandoffState, call *events.ToolCallState) string {
	agentLabel := delegatedInspectorAgentLabel(handoff, call)
	if agentLabel == "" {
		return "Delegated"
	}
	return agentLabel
}

func delegatedInspectorGroupStatus(handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return "done"
	}
	if handoff.PreviewActive {
		return "running"
	}
	switch handoff.Status {
	case events.AgentResultStatusFailed:
		return "error"
	default:
		return "done"
	}
}

func renderDelegatedInspectorTaskLines(m Model, target inspectorHandoffTarget, handoff *events.AgentHandoffState, call *events.ToolCallState, width int, depth int) ([]string, map[int]inspectorToolLineAction) {
	task := delegatedInspectorTaskLabel(handoff, call)
	if task == "" {
		return nil, nil
	}
	indent := inspectorTreeIndent(depth)
	contentWidth := max(width-len(indent), 1)
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7aa2f7")))
	line := indent + style.Render(truncateEnd(task, contentWidth))
	return []string{line}, map[int]inspectorToolLineAction{
		0: {
			Kind:    inspectorToolLineOpenHandoff,
			Handoff: target,
		},
	}
}

func delegatedInspectorTaskLabel(handoff *events.AgentHandoffState, call *events.ToolCallState) string {
	if handoff != nil {
		if task := strings.Join(strings.Fields(strings.TrimSpace(handoff.Task)), " "); task != "" {
			return task
		}
	}
	if input, ok := parseDelegateToolViewInput(call.Input); ok {
		return strings.Join(strings.Fields(strings.TrimSpace(input.Task)), " ")
	}
	return ""
}

func delegatedInspectorAgentLabel(handoff *events.AgentHandoffState, call *events.ToolCallState) string {
	agentID := ""
	if handoff != nil {
		agentID = strings.TrimSpace(handoff.ChildAgentID)
	}
	if agentID == "" {
		if input, ok := parseDelegateToolViewInput(call.Input); ok {
			agentID = strings.TrimSpace(input.ChildAgentID)
		}
	}
	if agentID == "" {
		return "Delegated"
	}
	return titleizeDelegatedAgentLabel(agentID)
}

func titleizeDelegatedAgentLabel(agentID string) string {
	normalized := strings.NewReplacer("-", " ", "_", " ").Replace(strings.TrimSpace(agentID))
	words := strings.Fields(normalized)
	if len(words) == 0 {
		return ""
	}
	for idx, word := range words {
		runes := []rune(word)
		if len(runes) == 0 {
			continue
		}
		if runes[0] >= 'a' && runes[0] <= 'z' {
			runes[0] = runes[0] - ('a' - 'A')
		}
		words[idx] = string(runes)
	}
	return strings.Join(words, " ")
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
