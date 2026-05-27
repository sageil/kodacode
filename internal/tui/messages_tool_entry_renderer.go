package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type transcriptToolEntryRenderer interface {
	BatchConsecutive() bool
	ToolCallsVisible(m Model) bool
	ShouldRenderCall(m Model, turn *events.TurnState, callID string, call *events.ToolCallState) bool
	RenderBatch(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection
	RenderOne(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool)
	RenderLivePreview(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int) []transcriptSection
}

type shellTranscriptToolEntryRenderer struct{}
type wideTranscriptToolEntryRenderer struct{}
type classicTranscriptToolEntryRenderer struct{}

func transcriptToolEntryRendererForModel(m Model) transcriptToolEntryRenderer {
	if shellLayoutEnabled(m) {
		return shellTranscriptToolEntryRenderer{}
	}
	if isWideShell(m) {
		return wideTranscriptToolEntryRenderer{}
	}
	return classicTranscriptToolEntryRenderer{}
}

func (shellTranscriptToolEntryRenderer) BatchConsecutive() bool { return true }

func (shellTranscriptToolEntryRenderer) ToolCallsVisible(m Model) bool {
	return m.shellToolCallsVisible
}

func (shellTranscriptToolEntryRenderer) ShouldRenderCall(_ Model, turn *events.TurnState, callID string, call *events.ToolCallState) bool {
	if isMutationToolCall(call) && transcriptOwnsToolCallRow(call) {
		return true
	}
	return shouldRenderToolCallInTranscript(turn, callID, call)
}

func (shellTranscriptToolEntryRenderer) RenderBatch(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	if !m.shellToolCallsVisible {
		return nil
	}
	refs = filterPendingQuestionToolRefs(m, refs)
	if len(refs) == 0 {
		return nil
	}
	return renderShellTurnToolOutcomeSections(m, state, refs, width)
}

func (r shellTranscriptToolEntryRenderer) RenderOne(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool) {
	if !m.shellToolCallsVisible {
		return transcriptSection{}, false
	}
	if !r.ShouldRenderCall(m, turn, callID, call) {
		return transcriptSection{}, false
	}
	ref := sessionToolCallRef{TurnID: turnID, CallID: callID}
	if content := strings.TrimSpace(renderShellToolTranscriptSection(m, state, ref, call, width)); content != "" {
		return transcriptSection{content: content, toolRefs: []sessionToolCallRef{ref}}, true
	}
	return transcriptSection{}, false
}

func (r shellTranscriptToolEntryRenderer) RenderLivePreview(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int) []transcriptSection {
	if !r.ToolCallsVisible(m) {
		return nil
	}
	return renderToolCallPreviewSections(m, state, turnID, turn, width)
}

func (wideTranscriptToolEntryRenderer) BatchConsecutive() bool { return true }

func (wideTranscriptToolEntryRenderer) ToolCallsVisible(Model) bool { return true }

func (wideTranscriptToolEntryRenderer) ShouldRenderCall(_ Model, turn *events.TurnState, callID string, call *events.ToolCallState) bool {
	return shouldRenderToolCallInTranscript(turn, callID, call)
}

func (wideTranscriptToolEntryRenderer) RenderBatch(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	refs = filterPendingQuestionToolRefs(m, refs)
	if len(refs) == 0 {
		return nil
	}
	return renderCompactWideTurnToolOutcomeSections(m, state, refs, width)
}

func (wideTranscriptToolEntryRenderer) RenderOne(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool) {
	return renderSingleToolTranscriptSection(m, state, turnID, turn, callID, call, width)
}

func (wideTranscriptToolEntryRenderer) RenderLivePreview(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int) []transcriptSection {
	return renderToolCallPreviewSections(m, state, turnID, turn, width)
}

func (classicTranscriptToolEntryRenderer) BatchConsecutive() bool { return false }

func (classicTranscriptToolEntryRenderer) ToolCallsVisible(Model) bool { return true }

func (classicTranscriptToolEntryRenderer) ShouldRenderCall(_ Model, turn *events.TurnState, callID string, call *events.ToolCallState) bool {
	return shouldRenderToolCallInTranscript(turn, callID, call)
}

func (classicTranscriptToolEntryRenderer) RenderBatch(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	refs = filterPendingQuestionToolRefs(m, refs)
	if len(refs) == 0 {
		return nil
	}
	return renderClassicTurnToolOutcomeSections(m, state, refs, width)
}

func (classicTranscriptToolEntryRenderer) RenderOne(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool) {
	return renderSingleToolTranscriptSection(m, state, turnID, turn, callID, call, width)
}

func (classicTranscriptToolEntryRenderer) RenderLivePreview(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int) []transcriptSection {
	return renderToolCallPreviewSections(m, state, turnID, turn, width)
}

func renderSingleToolTranscriptSection(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool) {
	if section := strings.TrimSpace(renderTurnToolTranscriptSection(m, state, turnID, turn, callID, call, width)); section != "" {
		ref := sessionToolCallRef{TurnID: turnID, CallID: callID}
		return transcriptSection{content: section, toolRefs: []sessionToolCallRef{ref}}, true
	}
	return transcriptSection{}, false
}
