package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type toolOutcomeKind string

const (
	toolOutcomeMutation    toolOutcomeKind = "mutation"
	toolOutcomeCommand     toolOutcomeKind = "command"
	toolOutcomeExploration toolOutcomeKind = "exploration"
	toolOutcomeGeneric     toolOutcomeKind = "generic"
)

type toolOutcomeRow struct {
	Kind   toolOutcomeKind
	Label  string
	Detail string
	Status string
	Ref    sessionToolCallRef
	Path   string
}

func visibleToolSelectionRefs(m Model, state events.SessionState) []sessionToolCallRef {
	if shellLayoutEnabled(m) {
		if !m.shellToolCallsVisible {
			return nil
		}
		if refs := filterNonMutationToolSelectionRefs(state, visibleTranscriptToolRefs(m)); len(refs) > 0 {
			return refs
		}
		return filterNonMutationToolSelectionRefs(state, ungroupedSessionToolSelectionRefs(state))
	}
	if isWideShell(m) {
		rows := deriveSessionToolOutcomeRows(state)
		refs := make([]sessionToolCallRef, 0, len(rows))
		for _, row := range rows {
			if row.Kind == toolOutcomeMutation {
				continue
			}
			if strings.TrimSpace(row.Ref.TurnID) == "" || strings.TrimSpace(row.Ref.CallID) == "" {
				continue
			}
			refs = append(refs, row.Ref)
		}
		if len(refs) > 0 {
			return refs
		}
	}
	return filterNonMutationToolSelectionRefs(state, orderedSessionToolCallRefs(state))
}

func ungroupedSessionToolSelectionRefs(state events.SessionState) []sessionToolCallRef {
	rows := deriveUngroupedToolOutcomeRows(state, orderedAllSessionToolCallRefs(state))
	refs := make([]sessionToolCallRef, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Ref.TurnID) == "" || strings.TrimSpace(row.Ref.CallID) == "" {
			continue
		}
		refs = append(refs, row.Ref)
	}
	return refs
}

func visibleTranscriptToolRefs(m Model) []sessionToolCallRef {
	if len(m.transcriptView.toolLines) == 0 {
		return nil
	}
	type visibleRef struct {
		ref  sessionToolCallRef
		line int
	}
	visible := make([]visibleRef, 0, len(m.transcriptView.toolLines))
	for ref, line := range m.transcriptView.toolLines {
		if strings.TrimSpace(ref.TurnID) == "" || strings.TrimSpace(ref.CallID) == "" {
			continue
		}
		visible = append(visible, visibleRef{ref: ref, line: line})
	}
	sort.SliceStable(visible, func(i, j int) bool {
		if visible[i].line != visible[j].line {
			return visible[i].line < visible[j].line
		}
		if visible[i].ref.TurnID != visible[j].ref.TurnID {
			return visible[i].ref.TurnID < visible[j].ref.TurnID
		}
		return visible[i].ref.CallID < visible[j].ref.CallID
	})
	refs := make([]sessionToolCallRef, 0, len(visible))
	for _, item := range visible {
		refs = append(refs, item.ref)
	}
	return refs
}

func filterNonMutationToolSelectionRefs(state events.SessionState, refs []sessionToolCallRef) []sessionToolCallRef {
	if len(refs) == 0 {
		return nil
	}
	filtered := make([]sessionToolCallRef, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.TurnID) == "" || strings.TrimSpace(ref.CallID) == "" {
			continue
		}
		_, call := sessionToolCall(state, ref)
		if call == nil || outcomeCategoryForTool(call) == toolOutcomeMutation {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

func deriveSessionToolOutcomeRows(state events.SessionState) []toolOutcomeRow {
	return deriveToolOutcomeRows(state, orderedAllSessionToolCallRefs(state))
}

func deriveTurnToolOutcomeRows(state events.SessionState, refs []sessionToolCallRef) []toolOutcomeRow {
	return deriveToolOutcomeRows(state, refs)
}

func deriveUngroupedToolOutcomeRows(state events.SessionState, refs []sessionToolCallRef) []toolOutcomeRow {
	rows := make([]toolOutcomeRow, 0, len(refs))
	for _, ref := range refs {
		_, call := sessionToolCall(state, ref)
		if call == nil {
			continue
		}
		if isApplyPatchNoop(call) {
			continue
		}
		switch outcomeCategoryForTool(call) {
		case toolOutcomeMutation:
			rows = append(rows, mutationOutcomeRows(state, ref, call)...)
		case toolOutcomeCommand:
			rows = append(rows, commandOutcomeRow(state, ref, call))
		case toolOutcomeExploration:
			rows = append(rows, explorationOutcomeRow(state, ref, call))
		default:
			rows = append(rows, genericOutcomeRow(state, ref, call))
		}
	}
	return rows
}

func deriveToolOutcomeRows(state events.SessionState, refs []sessionToolCallRef) []toolOutcomeRow {
	rows := make([]toolOutcomeRow, 0, len(refs))
	explorationCounts := map[string]int{}
	var explorationRef sessionToolCallRef
	explorationStatus := ""

	flushExploration := func() {
		if len(explorationCounts) == 0 {
			return
		}
		rows = append(rows, toolOutcomeRow{
			Kind:   toolOutcomeExploration,
			Label:  explorationOutcomeLabel(explorationStatus),
			Detail: explorationSummaryLabel(explorationCounts),
			Status: normalizeOutcomeStatus(explorationStatus),
			Ref:    explorationRef,
		})
		clear(explorationCounts)
		explorationRef = sessionToolCallRef{}
		explorationStatus = ""
	}

	for _, ref := range refs {
		_, call := sessionToolCall(state, ref)
		if call == nil {
			continue
		}
		if isApplyPatchNoop(call) {
			continue
		}
		switch outcomeCategoryForTool(call) {
		case toolOutcomeMutation:
			flushExploration()
			rows = append(rows, mutationOutcomeRows(state, ref, call)...)
		case toolOutcomeCommand:
			flushExploration()
			rows = append(rows, commandOutcomeRow(state, ref, call))
		case toolOutcomeExploration:
			explorationCounts[strings.TrimSpace(call.ToolName)]++
			explorationRef = ref
			explorationStatus = mergeOutcomeStatus(explorationStatus, toolStatus(call))
		default:
			flushExploration()
			rows = append(rows, genericOutcomeRow(state, ref, call))
		}
	}

	flushExploration()
	return rows
}

func orderedAllSessionToolCallRefs(state events.SessionState) []sessionToolCallRef {
	refs := make([]sessionToolCallRef, 0, len(state.TurnOrder)*2)
	for _, turnID := range orderedSessionTurnIDs(state) {
		turn := state.Turns[turnID]
		for _, callID := range orderedToolCallIDs(turn) {
			call := turn.ToolCalls[callID]
			if shouldHideSupersededMutationFailure(turn, callID, call) {
				continue
			}
			if shouldHideSupersededRetriedLogicalToolCall(turn, callID, call) {
				continue
			}
			if shouldHideSupersededDelegateAttempt(turn, callID, call) {
				continue
			}
			refs = append(refs, sessionToolCallRef{TurnID: turnID, CallID: callID})
		}
	}
	return refs
}

func outcomeCategoryForTool(call *events.ToolCallState) toolOutcomeKind {
	if call == nil {
		return toolOutcomeGeneric
	}
	if presenter, ok := toolPresenterForCall(call); ok && presenter.Category != nil {
		return presenter.Category(call)
	}
	return toolOutcomeGeneric
}

func mutationOutcomeRows(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) []toolOutcomeRow {
	paths := mutationOutcomePaths(call)
	if len(paths) == 0 {
		label := strings.TrimSpace(toolDisplayNameForSession(state, call))
		if label == "" {
			label = "tool"
		}
		return []toolOutcomeRow{{
			Kind:   toolOutcomeMutation,
			Label:  label,
			Detail: mutationOutcomeDetail(call, ""),
			Status: toolStatus(call),
			Ref:    ref,
		}}
	}
	label := displayToolPath(state.WorkspaceRoot, paths[0])
	path := paths[0]
	if len(paths) > 1 {
		if pathLabel := mutationGroupedToolPathLabel(state.WorkspaceRoot, paths); pathLabel != "" {
			label = pathLabel
		} else {
			label = fmt.Sprintf("%d files changed", len(paths))
		}
		path = ""
	}
	return []toolOutcomeRow{{
		Kind:   toolOutcomeMutation,
		Label:  label,
		Detail: mutationOutcomeDetail(call, path),
		Status: toolStatus(call),
		Ref:    ref,
		Path:   path,
	}}
}

func mutationOutcomePaths(call *events.ToolCallState) []string {
	if call == nil {
		return nil
	}
	if presenter, ok := toolPresenterForCall(call); ok && presenter.MutationPaths != nil {
		return presenter.MutationPaths(call)
	}
	return nil
}

func mutationOutcomeDetail(call *events.ToolCallState, path string) string {
	if call == nil {
		return ""
	}
	switch strings.TrimSpace(call.ToolName) {
	case "mkdir":
		return "create directory"
	case "write":
		display, ok := mutationDisplayFromCall("", call)
		if ok && strings.TrimSpace(display.Summary) != "" {
			return display.Summary
		}
		return "write file"
	case "apply_patch":
		display, ok := mutationDisplayFromCall("", call)
		if ok && strings.TrimSpace(display.Summary) != "" {
			return display.Summary
		}
		return "edit files"
	case "bash":
		if path != "" {
			return "shell write"
		}
		return "shell mutation"
	default:
		return summarizeInlineValue(strings.TrimSpace(call.Output))
	}
}

func commandOutcomeRow(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) toolOutcomeRow {
	label := strings.TrimSpace(toolDisplayNameForSession(state, call))
	if label == "" {
		label = "tool"
	}
	return toolOutcomeRow{
		Kind:   toolOutcomeCommand,
		Label:  label,
		Detail: commandOutcomeDetail(call),
		Status: toolStatus(call),
		Ref:    ref,
	}
}

func commandOutcomeDetail(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	switch {
	case call.Executing:
		return "(running...)"
	default:
		return ""
	}
}

func genericOutcomeRow(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) toolOutcomeRow {
	label := strings.TrimSpace(toolDisplayNameForSession(state, call))
	if strings.TrimSpace(call.ToolName) == "question" {
		if prompt := strings.TrimSpace(questionToolPrompt(call)); prompt != "" {
			label = prompt
		}
	}
	if label == "" {
		label = "tool"
	}
	detail := strings.TrimSpace(toolPrimaryListSummary(call))
	if errorText := strings.TrimSpace(call.Error); errorText != "" && isTaskToolCall(call) {
		detail = strings.TrimSpace(taskToolErrorSummary(call, errorText))
	}
	if isDelegateToolCall(call) {
		detail = strings.Join(strings.Fields(detail), " ")
	} else if strings.Contains(detail, "\n") {
		detail = summarizeInlineValue(detail)
	}
	return toolOutcomeRow{
		Kind:   toolOutcomeGeneric,
		Label:  label,
		Detail: detail,
		Status: toolStatus(call),
		Ref:    ref,
	}
}

func explorationOutcomeRow(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) toolOutcomeRow {
	label := strings.TrimSpace(groupedToolItemLabel(state.WorkspaceRoot, call))
	if label == "" {
		label = strings.TrimSpace(toolDisplayNameForSession(state, call))
	}
	if label == "" {
		label = "tool"
	}
	detail := strings.TrimSpace(groupedToolItemResultDetail(call))
	if strings.Contains(detail, "\n") {
		detail = summarizeInlineValue(detail)
	}
	return toolOutcomeRow{
		Kind:   toolOutcomeExploration,
		Label:  label,
		Detail: detail,
		Status: toolStatus(call),
		Ref:    ref,
	}
}

func explorationSummaryLabel(counts map[string]int) string {
	parts := make([]string, 0, len(counts))
	appendCountAs := func(name, label string) {
		if counts[name] <= 0 {
			return
		}
		parts = append(parts, fmt.Sprintf("%d %s", counts[name], label))
	}
	appendCount := func(name string) {
		appendCountAs(name, name)
	}
	appendCount("search")
	appendCount("locate")
	appendCount("read")
	appendCountAs("bash", "shell")
	appendCount("refs")
	appendCount("list")
	appendCount("tree")
	appendCount("git_status")
	appendCount("git_diff")
	appendCount("git_show")
	if len(parts) == 0 {
		return "inspected workspace"
	}
	return strings.Join(parts, " · ")
}

func explorationOutcomeLabel(status string) string {
	switch normalizeOutcomeStatus(status) {
	case "running", "declared", "preparing", "building":
		return "Exploring"
	case "error":
		return "Exploration failed"
	default:
		return "Explored"
	}
}

func commandOutcomeGroupLabel(status string) string {
	switch normalizeOutcomeStatus(status) {
	case "running", "declared", "preparing", "building":
		return "Running"
	case "error":
		return "Run failed"
	default:
		return "Ran"
	}
}

func mutationOutcomeGroupLabel(status string) string {
	switch normalizeOutcomeStatus(status) {
	case "running", "declared", "preparing", "building":
		return "Changing"
	case "error":
		return "Change failed"
	default:
		return "Changed"
	}
}

func genericOutcomeGroupLabel(status string) string {
	switch normalizeOutcomeStatus(status) {
	case "running", "declared", "preparing", "building":
		return "Using"
	case "error":
		return "Use failed"
	default:
		return "Used"
	}
}

func toolOutcomeGroupLabel(kind wideToolTranscriptGroupKind, status string) string {
	switch kind {
	case wideToolGroupExplored:
		return explorationOutcomeLabel(status)
	case wideToolGroupRan:
		return commandOutcomeGroupLabel(status)
	case wideToolGroupBlocked:
		return "Blocked"
	case wideToolGroupQuestion:
		return "Question"
	case wideToolGroupTaskList:
		return "List Tasks"
	case wideToolGroupMutation:
		return mutationOutcomeGroupLabel(status)
	default:
		return genericOutcomeGroupLabel(status)
	}
}

func toolOutcomeShowsSpinner(status string) bool {
	switch normalizeOutcomeStatus(status) {
	case "running", "declared", "preparing", "building":
		return true
	default:
		return false
	}
}

func mergeOutcomeStatus(current, next string) string {
	current = strings.TrimSpace(current)
	next = normalizeOutcomeStatus(next)
	if current == "" {
		return next
	}
	current = normalizeOutcomeStatus(current)
	switch {
	case current == "running" || next == "running":
		return "running"
	case current == "declared" || next == "declared":
		return "declared"
	case current == "preparing" || next == "preparing":
		return "preparing"
	case current == "building" || next == "building":
		return "building"
	case current == "partial" || next == "partial":
		return "partial"
	case current == "error" && next == "error":
		return "error"
	case current == "error" || next == "error":
		return "partial"
	default:
		return "done"
	}
}

func normalizeOutcomeStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "done", "partial", "error", "running", "declared", "preparing", "building":
		return strings.TrimSpace(status)
	default:
		return "done"
	}
}
