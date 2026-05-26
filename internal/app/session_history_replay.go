package app

import (
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/textutil"
)

func cloneReplayedSessionTurn(turn *replayedSessionTurn) *replayedSessionTurn {
	if turn == nil {
		return nil
	}
	return &replayedSessionTurn{
		TurnID:              turn.TurnID,
		Inputs:              normalizePendingToolConversation(append([]provider.Input(nil), turn.Inputs...)),
		RawToolResults:      cloneReplayedToolResults(turn.RawToolResults),
		Executions:          cloneReplayedExecutions(turn.Executions),
		WorkspacePaths:      append([]string(nil), turn.WorkspacePaths...),
		RuntimeNotes:        cloneReplayedSessionRuntimeNotes(turn.RuntimeNotes),
		ToolCallCount:       turn.ToolCallCount,
		Terminal:            turn.Terminal,
		TerminalSequence:    turn.TerminalSequence,
		TerminalStatus:      turn.TerminalStatus,
		TerminalError:       turn.TerminalError,
		TerminalRetryable:   turn.TerminalRetryable,
		SuccessfulToolCalls: turn.SuccessfulToolCalls,
		FailedToolCalls:     turn.FailedToolCalls,
		committedAssistant:  turn.committedAssistant,
		UserText:            turn.UserText,
		UserAttachments:     cloneProviderAttachments(turn.UserAttachments),
		AssistantText:       turn.AssistantText,
		ReasoningText:       turn.ReasoningText,
		ReusedResults:       append([]string(nil), turn.ReusedResults...),
		ToolNames:           append([]string(nil), turn.ToolNames...),
		FailedToolNames:     append([]string(nil), turn.FailedToolNames...),
	}
}

func appendReplayedTurnReasoning(turn *replayedSessionTurn, content string) {
	if turn == nil || content == "" {
		return
	}
	if turn.reasoning == nil {
		turn.reasoning = &textutil.StringAccumulator{}
	}
	turn.ReasoningText = turn.reasoning.Append(turn.ReasoningText, content)
}

func cloneReplayedToolResults(results map[string]replayedToolResult) map[string]replayedToolResult {
	if len(results) == 0 {
		return nil
	}
	cloned := make(map[string]replayedToolResult, len(results))
	for callID, result := range results {
		result.StructuredResult = cloneStructuredResult(result.StructuredResult)
		result.OutputBlob = cloneToolResultBlobRef(result.OutputBlob)
		result.ErrorBlob = cloneToolResultBlobRef(result.ErrorBlob)
		cloned[callID] = result
	}
	return cloned
}

func cloneToolResultBlobRef(ref *events.ToolResultBlobRef) *events.ToolResultBlobRef {
	if ref == nil {
		return nil
	}
	copyRef := *ref
	return &copyRef
}

func cloneReplayedExecutions(executions map[string]replayedExecution) map[string]replayedExecution {
	if len(executions) == 0 {
		return nil
	}
	cloned := make(map[string]replayedExecution, len(executions))
	for callID, execution := range executions {
		cloned[callID] = execution
	}
	return cloned
}

func cloneReplayedSessionRuntimeNotes(notes []replayedSessionRuntimeNote) []replayedSessionRuntimeNote {
	if len(notes) == 0 {
		return nil
	}
	cloned := make([]replayedSessionRuntimeNote, len(notes))
	copy(cloned, notes)
	return cloned
}

func (t *replayedSessionTurn) recordToolResult(result replayedToolResult) {
	if t.RawToolResults == nil {
		t.RawToolResults = make(map[string]replayedToolResult)
	}
	t.RawToolResults[result.CallID] = result
}

func (t *replayedSessionTurn) recordExecution(execution replayedExecution, callID string) {
	if t == nil || strings.TrimSpace(callID) == "" {
		return
	}
	if t.Executions == nil {
		t.Executions = make(map[string]replayedExecution)
	}
	t.Executions[callID] = execution
}

func (t *replayedSessionTurn) execution(callID string) *replayedExecution {
	if t == nil || t.Executions == nil {
		return nil
	}
	execution, ok := t.Executions[callID]
	if !ok {
		return nil
	}
	copyExecution := execution
	return &copyExecution
}

func (t *replayedSessionTurn) replayInputs() []provider.Input {
	if t == nil || len(t.Inputs) == 0 {
		return nil
	}
	inputs := make([]provider.Input, 0, len(t.Inputs))
	for _, input := range t.Inputs {
		switch input.Kind {
		case provider.InputKindToolCall, provider.InputKindToolResult:
			if strings.TrimSpace(input.CallID) == "" {
				continue
			}
			if _, ok := t.RawToolResults[input.CallID]; !ok {
				continue
			}
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func (t *replayedSessionTurn) hasReplayContent() bool {
	if t == nil {
		return false
	}
	if len(t.Inputs) > 0 || t.ToolCallCount > 0 || strings.TrimSpace(t.UserText) != "" || len(t.UserAttachments) > 0 || strings.TrimSpace(t.AssistantText) != "" {
		return true
	}
	if strings.TrimSpace(t.ReasoningText) != "" {
		return true
	}
	if len(t.ReusedResults) > 0 || len(t.WorkspacePaths) > 0 || len(t.RuntimeNotes) > 0 {
		return true
	}
	return strings.TrimSpace(t.TerminalError) != ""
}

func (t *replayedSessionTurn) postTerminalRuntimeNotes() []replayedSessionRuntimeNote {
	if t == nil || len(t.RuntimeNotes) == 0 || !t.Terminal {
		return nil
	}
	notes := make([]replayedSessionRuntimeNote, 0, len(t.RuntimeNotes))
	for _, note := range t.RuntimeNotes {
		if note.Sequence <= t.TerminalSequence {
			continue
		}
		notes = append(notes, note)
	}
	return notes
}

func appendBackgroundRuntimeNote(turn *replayedSessionTurn, order []string, compaction **events.SessionHistoryContinuationUpdatedPayload, compactionBudgetBytes int, sequence int64, note string) {
	note = strings.TrimSpace(note)
	if turn == nil || note == "" {
		return
	}
	turn.RuntimeNotes = append(turn.RuntimeNotes, replayedSessionRuntimeNote{
		Sequence: sequence,
		Content:  note,
	})
	if *compaction != nil && turnIDInCompactedPrefix(order, *compaction, turn.TurnID) {
		*compaction = appendRuntimeNoteToCompaction(*compaction, turn.TurnID, note, compactionBudgetBytes)
		return
	}
	if !turn.Terminal {
		turn.Inputs = append(turn.Inputs, provider.Input{
			Kind:    provider.InputKindAssistantMessage,
			Content: note,
		})
	}
}

type sessionReplayTimelineEntry struct {
	TurnID   string
	Sequence int64
	Order    int
	Note     replayedSessionRuntimeNote
}

func buildSessionReplayTimeline(order []string, turns map[string]*replayedSessionTurn) []sessionReplayTimelineEntry {
	timeline := make([]sessionReplayTimelineEntry, 0, len(order))
	for index, turnID := range order {
		turn := turns[turnID]
		if turn == nil {
			continue
		}
		timeline = append(timeline, sessionReplayTimelineEntry{
			TurnID:   turnID,
			Sequence: turn.TerminalSequence,
			Order:    index,
		})
		for noteIndex, note := range turn.postTerminalRuntimeNotes() {
			timeline = append(timeline, sessionReplayTimelineEntry{
				Sequence: note.Sequence,
				Order:    len(order) + index + noteIndex,
				Note:     note,
			})
		}
	}
	sort.SliceStable(timeline, func(i, j int) bool {
		if timeline[i].Sequence == timeline[j].Sequence {
			return timeline[i].Order < timeline[j].Order
		}
		return timeline[i].Sequence < timeline[j].Sequence
	})
	return timeline
}

func sessionRuntimeNotesFromCheckpoint(payload []events.SessionHistoryRuntimeNotePayload) []replayedSessionRuntimeNote {
	if len(payload) == 0 {
		return nil
	}
	notes := make([]replayedSessionRuntimeNote, 0, len(payload))
	for _, note := range payload {
		notes = append(notes, replayedSessionRuntimeNote{
			Sequence: note.Sequence,
			Content:  note.Content,
		})
	}
	return notes
}

func appendUniqueValues(existing, values []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	for _, value := range existing {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	existing = append(existing[:0:0], existing...)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		existing = append(existing, trimmed)
		seen[trimmed] = struct{}{}
	}
	return existing
}

func cloneProviderAttachments(attachments []provider.Attachment) []provider.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	cloned := make([]provider.Attachment, len(attachments))
	copy(cloned, attachments)
	return cloned
}

func attachmentsFromUserMessagePayload(attachments []events.UserAttachmentPayload) []provider.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]provider.Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, provider.Attachment{
			Name:     attachment.Name,
			MIMEType: attachment.MIMEType,
			DataURL:  attachment.DataURL,
		})
	}
	return converted
}
