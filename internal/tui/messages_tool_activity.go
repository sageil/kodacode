package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderFocusedToolTranscriptSection(m Model, ref sessionToolCallRef, state events.SessionState, call *events.ToolCallState, width int) string {
	if call != nil && strings.TrimSpace(call.ToolName) == "question" {
		return renderQuestionOutcomeTranscriptSection(m, state, ref, call, width)
	}
	if isWideShell(m) {
		switch {
		case isCommandToolCall(call):
			return renderCommandTranscriptSection(m, state.WorkspaceRoot, call, width)
		case call != nil && strings.TrimSpace(call.ToolName) == "bash" && outcomeCategoryForTool(call) == toolOutcomeExploration:
			return renderWideToolSection(m, explorationOutcomeLabel(toolStatus(call)), toolStatus(call), nil, "1 shell", width)
		case call != nil && strings.TrimSpace(call.ToolName) == "read":
			return renderReadTranscriptSectionForSession(m, state.SessionID, ref, state.WorkspaceRoot, call, width)
		case call != nil && (strings.TrimSpace(call.ToolName) == "search" || strings.TrimSpace(call.ToolName) == "locate"):
			return renderSearchTranscriptSection(m, ref, state.WorkspaceRoot, call, width)
		case showMutationToolInTranscript(call):
			return renderMutationToolTimelineSection(m, state.WorkspaceRoot, call, width)
		default:
			return renderWideToolDetailTranscriptSection(m, ref, state, call, width)
		}
	}
	switch {
	case isCommandToolCall(call):
		return renderCommandTranscriptSection(m, state.WorkspaceRoot, call, width)
	case call != nil && strings.TrimSpace(call.ToolName) == "bash" && outcomeCategoryForTool(call) == toolOutcomeExploration:
		return renderWideToolSection(m, explorationOutcomeLabel(toolStatus(call)), toolStatus(call), nil, "1 shell", width)
	case call != nil && (strings.TrimSpace(call.ToolName) == "search" || strings.TrimSpace(call.ToolName) == "locate"):
		return renderSearchTranscriptSection(m, ref, state.WorkspaceRoot, call, width)
	case showMutationToolInTranscript(call):
		return renderMutationToolTimelineSection(m, state.WorkspaceRoot, call, width)
	default:
		return renderToolDetailTranscriptSection(m, ref, state, call, width, true)
	}
}

func renderMCPToolTranscriptSection(m Model, ref sessionToolCallRef, state events.SessionState, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	if isWideShell(m) {
		return renderWideToolDetailTranscriptSection(m, ref, state, call, width)
	}
	return renderToolDetailTranscriptSection(m, ref, state, call, width, false)
}

func renderToolTimelineSection(m Model, state events.SessionState, turn *events.TurnState, call *events.ToolCallState, width int) string {
	if isWideShell(m) && showWideToolTimelineRow(call) {
		return renderWideToolTimelineRow(m, state, call, width)
	}
	switch {
	case showCommandToolInTranscript(turn, call):
		return renderCommandTranscriptSection(m, state.WorkspaceRoot, call, width)
	case showMutationToolInTranscript(call):
		return renderMutationToolTimelineSection(m, state.WorkspaceRoot, call, width)
	default:
		return ""
	}
}

func renderTurnToolOutcomeSections(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	if shellLayoutEnabled(m) && !m.shellToolCallsVisible {
		return nil
	}
	refs = filterPendingQuestionToolRefs(m, refs)
	if len(refs) == 0 {
		return nil
	}
	if shellLayoutEnabled(m) {
		return renderShellTurnToolOutcomeSections(m, state, refs, width)
	}
	if isWideShell(m) {
		return renderCompactWideTurnToolOutcomeSections(m, state, refs, width)
	}

	rows := deriveTurnToolOutcomeRows(state, refs)
	if len(rows) == 0 {
		return nil
	}

	sections := make([]transcriptSection, 0, len(rows)+1)
	for _, row := range rows {
		turn := state.Turns[row.Ref.TurnID]
		_, call := sessionToolCall(state, row.Ref)
		if !shouldRenderToolCallInTranscriptForLayout(m, turn, row.Ref.CallID, call) {
			continue
		}
		selected := selectedToolMatchesSession(m, state.SessionID, row.Ref)
		if selected && row.Kind != toolOutcomeMutation {
			if detail := strings.TrimSpace(renderFocusedToolTranscriptSection(m, row.Ref, state, call, width)); detail != "" {
				sections = append(sections, transcriptSection{
					content:  detail,
					toolRefs: []sessionToolCallRef{row.Ref},
				})
				continue
			}
		}
		summary := strings.TrimSpace(renderToolOutcomeSummarySection(m, state, row, call, width))
		if summary != "" {
			sections = append(sections, transcriptSection{
				content:  summary,
				toolRefs: []sessionToolCallRef{row.Ref},
			})
		}
	}
	return sections
}

func filterPendingQuestionToolRefs(m Model, refs []sessionToolCallRef) []sessionToolCallRef {
	pending := m.pendingQuestion()
	if pending == nil {
		return refs
	}
	filtered := make([]sessionToolCallRef, 0, len(refs))
	for _, ref := range refs {
		if ref.TurnID == strings.TrimSpace(pending.TurnID) && ref.CallID == strings.TrimSpace(pending.ToolCallID) {
			continue
		}
		filtered = append(filtered, ref)
	}
	return filtered
}

func renderToolOutcomeSummarySection(m Model, state events.SessionState, row toolOutcomeRow, call *events.ToolCallState, width int) string {
	if call != nil && strings.TrimSpace(call.ToolName) == "question" {
		return renderQuestionOutcomeTranscriptSection(m, state, row.Ref, call, width)
	}
	if isMCPToolCall(call) {
		return renderMCPToolTranscriptSection(m, row.Ref, state, call, width)
	}
	switch row.Kind {
	case toolOutcomeMutation:
		return renderMutationOutcomeTranscriptSection(m, state.WorkspaceRoot, row, call, width)
	case toolOutcomeCommand:
		return renderOutcomeSummaryTranscriptSection(m, row.Label, row.Detail, row.Status, width)
	case toolOutcomeExploration:
		return renderOutcomeSummaryTranscriptSection(m, row.Label, row.Detail, row.Status, width)
	default:
		return renderOutcomeSummaryTranscriptSection(m, row.Label, row.Detail, row.Status, width)
	}
}

func renderMutationOutcomeTranscriptSection(m Model, workspaceRoot string, row toolOutcomeRow, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	return renderMutationToolTimelineSection(m, workspaceRoot, call, width)
}

func renderOutcomeSummaryTranscriptSection(m Model, label, detail, status string, width int) string {
	return renderWideToolSection(m, strings.TrimSpace(label), status, splitToolMetaLines(detail), "", width)
}

func renderQuestionOutcomeTranscriptSection(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int) string {
	question := strings.TrimSpace(questionToolText(state, ref, call))
	answer := strings.TrimSpace(questionToolAnswer(state, ref, call))
	if question == "" && answer == "" {
		return ""
	}

	lines := make([]string, 0, 4)
	if question != "" {
		questionStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
			Italic(true)
		lines = append(lines, splitWrappedStyledLines(questionStyle.Render(question), max(width, 1))...)
	}
	if answer != "" {
		answerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff")))
		for _, line := range splitWrappedStyledLines(answerStyle.Render(answer), max(width-2, 1)) {
			lines = append(lines, "  "+line)
		}
	}
	return strings.Join(lines, "\n")
}

type wideToolTranscriptGroupKind string

const (
	wideToolGroupExplored wideToolTranscriptGroupKind = "explored"
	wideToolGroupRan      wideToolTranscriptGroupKind = "ran"
	wideToolGroupBlocked  wideToolTranscriptGroupKind = "blocked"
	wideToolGroupQuestion wideToolTranscriptGroupKind = "question"
	wideToolGroupTaskList wideToolTranscriptGroupKind = "task_list"
	wideToolGroupUsed     wideToolTranscriptGroupKind = "used"
	wideToolGroupMutation wideToolTranscriptGroupKind = "mutation"
)

type wideToolTranscriptGroup struct {
	Kind   wideToolTranscriptGroupKind
	Status string
	Refs   []sessionToolCallRef
}

func buildWideToolTranscriptGroups(state events.SessionState, refs []sessionToolCallRef) []wideToolTranscriptGroup {
	groups := make([]wideToolTranscriptGroup, 0, len(refs))
	var current wideToolTranscriptGroup
	flush := func() {
		if len(current.Refs) == 0 {
			return
		}
		groups = append(groups, current)
		current = wideToolTranscriptGroup{}
	}

	for _, ref := range refs {
		_, call := sessionToolCall(state, ref)
		if call == nil {
			continue
		}
		if isApplyPatchNoop(call) {
			continue
		}
		kind := wideToolTranscriptGroupKindForCall(call)
		status := normalizeOutcomeStatus(toolStatus(call))
		if kind == wideToolGroupMutation || kind == wideToolGroupTaskList {
			flush()
			groups = append(groups, wideToolTranscriptGroup{
				Kind:   kind,
				Status: status,
				Refs:   []sessionToolCallRef{ref},
			})
			continue
		}
		if len(current.Refs) == 0 || current.Kind != kind {
			flush()
			current = wideToolTranscriptGroup{
				Kind:   kind,
				Status: status,
				Refs:   []sessionToolCallRef{ref},
			}
			continue
		}
		current.Refs = append(current.Refs, ref)
		current.Status = mergeOutcomeStatus(current.Status, status)
	}
	flush()
	return groups
}

func wideToolTranscriptGroupKindForCall(call *events.ToolCallState) wideToolTranscriptGroupKind {
	if call == nil {
		return wideToolGroupUsed
	}
	if strings.TrimSpace(call.ToolName) == "question" {
		return wideToolGroupQuestion
	}
	if isTaskToolListCall(call) {
		return wideToolGroupTaskList
	}
	switch outcomeCategoryForTool(call) {
	case toolOutcomeMutation:
		return wideToolGroupMutation
	case toolOutcomeCommand:
		return wideToolGroupRan
	case toolOutcomeExploration:
		return wideToolGroupExplored
	default:
		return wideToolGroupUsed
	}
}
