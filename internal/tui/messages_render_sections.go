package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type transcriptTurnRenderOptions struct {
	suppressHistoryCompaction bool
}

func renderTurnTranscriptSections(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int) []transcriptSection {
	return renderTurnTranscriptSectionsWithOptions(m, state, turnID, turn, width, transcriptTurnRenderOptions{})
}

func renderTurnTranscriptSectionsWithOptions(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int, options transcriptTurnRenderOptions) []transcriptSection {
	if turn == nil {
		return nil
	}

	sections := make([]transcriptSection, 0, len(turn.Transcript)+3)
	hasCompactionTranscriptEntry := turnHasHistoryCompactionTranscriptEntry(turn)
	compactionRendered := false
	maybeRenderFallbackCompaction := func() {
		if options.suppressHistoryCompaction || hasCompactionTranscriptEntry || compactionRendered {
			return
		}
		if section := renderHistoryCompactionSection(m, state, turnID, turn, width); section != "" {
			sections = append(sections, transcriptSection{content: section})
			compactionRendered = true
		}
	}
	toolRenderer := transcriptToolEntryRendererForModel(m)
	for i := 0; i < len(turn.Transcript); i++ {
		entry := turn.Transcript[i]
		if suppressContextLimitContinuationTranscriptEntry(turn, entry) {
			continue
		}
		if suppressQuestionAnswerContinuationTranscriptEntry(state, turn, entry) {
			continue
		}
		switch entry.Kind {
		case events.TranscriptEntryUser:
			if body := strings.TrimSpace(entry.Text); body != "" {
				sections = append(sections, transcriptSection{
					content:        renderUserSection(m, width, body),
					selectionLines: transcriptRailSelectionLines(m, body, width),
				})
				maybeRenderFallbackCompaction()
			}
		case events.TranscriptEntryAssistant, events.TranscriptEntryWorklog:
			if entry.Kind == events.TranscriptEntryAssistant && suppressAssistantEntryForStructuredReview(turn, i) {
				continue
			}
			streamKey := assistantTranscriptEntryStreamKey(state.SessionID, turnID, i)
			if assistant := renderAssistantTranscriptSectionWithStreamKey(m, turn, entry.Text, width, streamKey); assistant != "" {
				if !isTurnContinuationTranscriptEntry(turn, entry) {
					maybeRenderFallbackCompaction()
				}
				sections = append(sections, transcriptSection{
					content:        assistant,
					selectionLines: assistantTranscriptSelectionLinesWithStreamKey(m, strings.TrimRight(strings.TrimSpace(entry.Text), "\n"), width, streamKey),
				})
				if isTurnContinuationTranscriptEntry(turn, entry) {
					maybeRenderFallbackCompaction()
				}
			} else {
				maybeRenderFallbackCompaction()
			}
		case events.TranscriptEntryCompaction:
			if options.suppressHistoryCompaction {
				continue
			}
			if section := renderHistoryCompactionSummarySection(m, entry.Text, width); section != "" {
				sections = append(sections, transcriptSection{content: section})
				compactionRendered = true
			}
		case events.TranscriptEntryReview:
			maybeRenderFallbackCompaction()
			if review := renderReviewTranscriptSection(m, turn, width); review != "" {
				sections = append(sections, transcriptSection{
					content:        review,
					selectionLines: reviewTranscriptSelectionLines(m, turn, width),
				})
			}
		case events.TranscriptEntryReasoning:
			maybeRenderFallbackCompaction()
			if reasoning := renderReasoningTranscriptSection(m, entry.Text, width, isTurnFinished(turn)); reasoning != "" {
				sections = append(sections, transcriptSection{content: reasoning})
			}
		case events.TranscriptEntryTool:
			maybeRenderFallbackCompaction()
			if toolRenderer.BatchConsecutive() {
				refs := make([]sessionToolCallRef, 0, 4)
				for i < len(turn.Transcript) && turn.Transcript[i].Kind == events.TranscriptEntryTool {
					callID := turn.Transcript[i].CallID
					call := turn.ToolCalls[callID]
					if shouldRenderToolCallInTranscriptForLayout(m, turn, callID, call) {
						refs = append(refs, sessionToolCallRef{TurnID: turnID, CallID: callID})
					}
					i++
				}
				i--
				if len(refs) > 0 {
					sections = append(sections, toolRenderer.RenderBatch(m, state, refs, width)...)
				}
				continue
			}
			call := turn.ToolCalls[entry.CallID]
			if section, ok := toolRenderer.RenderOne(m, state, turnID, turn, entry.CallID, call, width); ok {
				sections = append(sections, section)
			}
		}
	}
	if !compactionRendered {
		maybeRenderFallbackCompaction()
	}
	previewStreamKey := assistantPreviewTranscriptStreamKey(state.SessionID, turnID)
	if assistant := renderAssistantPreviewTranscriptSectionWithStreamKey(m, turn, turn.StreamingText, width, previewStreamKey); assistant != "" {
		sections = append(sections, transcriptSection{
			content:        assistant,
			selectionLines: assistantTranscriptSelectionLinesWithStreamKey(m, strings.TrimRight(strings.TrimSpace(turn.StreamingText), "\n"), width, previewStreamKey),
		})
	}
	sections = append(sections, renderLiveToolCallPreviewSections(m, state, turnID, turn, width)...)
	if delegation := renderDelegationSection(m, turn, width); delegation != "" {
		sections = append(sections, transcriptSection{content: delegation})
	}
	return sections
}

func renderLiveToolCallPreviewSections(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int) []transcriptSection {
	if shellLayoutEnabled(m) && !m.shellToolCallsVisible {
		return nil
	}
	if turn == nil || turn.Status != events.TurnStatusRunning {
		return nil
	}
	refs := make([]sessionToolCallRef, 0, len(turn.ToolCallOrder))
	for _, callID := range orderedToolCallIDs(turn) {
		call := turn.ToolCalls[callID]
		if !shouldRenderLiveToolCallPreview(turn, call) {
			continue
		}
		refs = append(refs, sessionToolCallRef{TurnID: turnID, CallID: callID})
	}
	refs = filterPendingQuestionToolRefs(m, refs)
	if len(refs) == 0 {
		return nil
	}
	rows := deriveTurnToolOutcomeRows(state, refs)
	sections := make([]transcriptSection, 0, len(rows))
	for _, row := range rows {
		summary := strings.TrimSpace(renderLiveToolCallPreviewSection(m, state, row, width))
		if summary == "" {
			continue
		}
		sections = append(sections, transcriptSection{
			content:  summary,
			toolRefs: []sessionToolCallRef{row.Ref},
		})
	}
	return sections
}

func shouldRenderLiveToolCallPreview(turn *events.TurnState, call *events.ToolCallState) bool {
	if turn == nil || turn.Status != events.TurnStatusRunning || call == nil || call.Completed {
		return false
	}
	if outcomeCategoryForTool(call) != toolOutcomeCommand {
		return false
	}
	return toolCallVisibleInRuntime(turn, call)
}

func renderLiveToolCallPreviewSection(m Model, state events.SessionState, row toolOutcomeRow, width int) string {
	if shellLayoutEnabled(m) {
		_, call := sessionToolCall(state, row.Ref)
		return renderShellToolOutcomeLine(m, state, row, call, width, selectedToolMatchesSession(m, state.SessionID, row.Ref))
	}
	title := strings.TrimSpace(row.Label)
	if detail := strings.TrimSpace(row.Detail); detail != "" {
		if title == "" {
			title = detail
		} else {
			title += " · " + detail
		}
	}
	return renderOutcomeSummaryTranscriptSection(m, title, "", row.Status, width)
}

func suppressContextLimitContinuationTranscriptEntry(turn *events.TurnState, entry events.TranscriptEntryState) bool {
	if turn == nil || turn.ContinuationStart == nil {
		return false
	}
	if strings.TrimSpace(turn.ContinuationStart.Reason) != events.TurnContinuationReasonContextLimit {
		return false
	}
	return isTurnContinuationTranscriptEntry(turn, entry)
}

func suppressQuestionAnswerContinuationTranscriptEntry(state events.SessionState, turn *events.TurnState, entry events.TranscriptEntryState) bool {
	if turn == nil || turn.ContinuationStart == nil {
		return false
	}
	if strings.TrimSpace(turn.ContinuationStart.Reason) != events.TurnContinuationReasonQuestionAnswer {
		return false
	}
	switch entry.Kind {
	case events.TranscriptEntryWorklog:
		return isTurnContinuationTranscriptEntry(turn, entry)
	case events.TranscriptEntryUser:
		return previousQuestionAnswerMatchesUserEntry(state, turn, entry)
	default:
		return false
	}
}

func previousQuestionAnswerMatchesUserEntry(state events.SessionState, turn *events.TurnState, entry events.TranscriptEntryState) bool {
	answer := strings.TrimSpace(entry.Text)
	if answer == "" || turn == nil || turn.ContinuationStart == nil {
		return false
	}
	previousTurnID := strings.TrimSpace(turn.ContinuationStart.PreviousTurnID)
	previous := state.Turns[previousTurnID]
	if previous == nil {
		return false
	}
	for _, callID := range previous.ToolCallOrder {
		call := previous.ToolCalls[callID]
		if call == nil || strings.TrimSpace(call.ToolName) != "question" {
			continue
		}
		ref := sessionToolCallRef{TurnID: previousTurnID, CallID: callID}
		if strings.TrimSpace(questionToolAnswer(state, ref, call)) == answer {
			return true
		}
	}
	return false
}

func renderHistoryCompactionSection(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int) string {
	if suppressInheritedHistoryContinuation(turn) {
		return ""
	}
	compaction := effectiveTurnHistoryContinuation(state, turnID, turn)
	if turn == nil || compaction == nil {
		return ""
	}
	return renderHistoryCompactionSummarySection(m, historyCompactionSummaryText(compaction), width)
}

func suppressInheritedHistoryContinuation(turn *events.TurnState) bool {
	if turn == nil || turn.Continuation != nil || turn.ContinuationStart == nil {
		return false
	}
	return true
}

func renderHistoryCompactionSummarySection(m Model, summary string, width int) string {
	body := trimHistoryCompactionBody(strings.TrimSpace(summary))
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return renderCompactionSummaryTranscriptCard(m, body, width)
}

func isTurnContinuationTranscriptEntry(turn *events.TurnState, entry events.TranscriptEntryState) bool {
	if turn == nil || turn.ContinuationStart == nil {
		return false
	}
	if entry.Kind != events.TranscriptEntryWorklog {
		return false
	}
	expected := turnContinuationTranscriptText(turn.ContinuationStart.Reason)
	return strings.TrimSpace(entry.Text) == strings.TrimSpace(expected)
}

func turnContinuationTranscriptText(reason string) string {
	switch strings.TrimSpace(reason) {
	case events.TurnContinuationReasonContextLimit:
		return "Continuing automatically after the previous turn reached the model input limit."
	case events.TurnContinuationReasonQuestionAnswer:
		return "Continuing in a new turn after the user answered a pending question."
	default:
		return "Continuing automatically from the previous turn."
	}
}

func trimHistoryCompactionBody(body string) string {
	trimmed := strings.TrimSpace(body)
	for _, header := range historyCompactionSummaryHeaders {
		if len(trimmed) >= len(header) && strings.EqualFold(trimmed[:len(header)], header) {
			return strings.TrimSpace(trimmed[len(header):])
		}
	}
	return trimmed
}

func historyCompactionSummaryText(compaction *events.HistoryContinuationState) string {
	if compaction == nil {
		return ""
	}
	return strings.TrimSpace(compaction.RenderedSummary)
}

func effectiveTurnHistoryContinuation(state events.SessionState, turnID string, turn *events.TurnState) *events.HistoryContinuationState {
	if turn == nil {
		return nil
	}
	if turn.Continuation != nil {
		return turn.Continuation
	}

	currentTurnID := strings.TrimSpace(turnID)
	if currentTurnID == "" {
		currentTurnID = strings.TrimSpace(turn.TurnID)
	}
	current := turn
	seen := make(map[string]struct{}, 4)

	for current != nil && current.Continuation == nil && current.ContinuationStart != nil {
		if currentTurnID != "" {
			if _, ok := seen[currentTurnID]; ok {
				return nil
			}
			seen[currentTurnID] = struct{}{}
		}
		previousTurnID := strings.TrimSpace(current.ContinuationStart.PreviousTurnID)
		if previousTurnID == "" {
			return nil
		}
		previous := state.Turns[previousTurnID]
		if previous == nil {
			return nil
		}
		if previous.Continuation != nil {
			return previous.Continuation
		}
		currentTurnID = previousTurnID
		current = previous
	}
	return nil
}

func turnHasHistoryCompactionTranscriptEntry(turn *events.TurnState) bool {
	if turn == nil {
		return false
	}
	for _, entry := range turn.Transcript {
		if entry.Kind == events.TranscriptEntryCompaction {
			return true
		}
	}
	return false
}

func renderTurnToolTranscriptSection(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) string {
	if shellLayoutEnabled(m) && !m.shellToolCallsVisible {
		return ""
	}
	if !shouldRenderToolCallInTranscriptForLayout(m, turn, callID, call) {
		return ""
	}
	ref := sessionToolCallRef{TurnID: turnID, CallID: callID}
	if shellLayoutEnabled(m) {
		return renderShellToolTranscriptSection(m, state, ref, call, width)
	}
	selected := selectedToolMatchesSession(m, state.SessionID, ref)
	if selected {
		return renderFocusedToolTranscriptSection(m, ref, state, call, width)
	}
	if isMCPToolCall(call) {
		return renderMCPToolTranscriptSection(m, ref, state, call, width)
	}
	return renderToolTimelineSection(m, state, turn, call, width)
}

func suppressAssistantEntryForStructuredReview(turn *events.TurnState, entryIndex int) bool {
	if turn == nil || turn.Review == nil || entryIndex < 0 || entryIndex >= len(turn.Transcript) {
		return false
	}
	if turn.Transcript[entryIndex].Kind != events.TranscriptEntryAssistant {
		return false
	}
	reviewIndex := -1
	for i := entryIndex + 1; i < len(turn.Transcript); i++ {
		if turn.Transcript[i].Kind == events.TranscriptEntryReview {
			reviewIndex = i
			break
		}
	}
	if reviewIndex < 0 {
		return false
	}
	for i := entryIndex + 1; i < reviewIndex; i++ {
		if turn.Transcript[i].Kind == events.TranscriptEntryAssistant {
			return false
		}
	}
	return true
}

func renderDraftTurnSections(m Model, state events.SessionState, width int) []transcriptSection {
	if m.busy {
		return nil
	}
	if strings.TrimSpace(m.userText) == "" {
		return nil
	}
	turn := currentTurn(state, m.turnID)
	if turn != nil && strings.TrimSpace(turn.UserText) != "" {
		return nil
	}
	return []transcriptSection{{content: renderUserSection(m, width, strings.TrimSpace(m.userText))}}
}

func visibleTranscriptTurnIDs(m Model, state events.SessionState) []string {
	turnIDs := orderedSessionTurnIDs(state)
	limit := m.displayTurns
	if limit <= 0 || len(turnIDs) <= limit {
		return turnIDs
	}
	return turnIDs[len(turnIDs)-limit:]
}
