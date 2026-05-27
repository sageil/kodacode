package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type transcriptToolEntryRenderer interface {
	BatchConsecutive() bool
	RenderBatch(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection
	RenderOne(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool)
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

func (shellTranscriptToolEntryRenderer) RenderBatch(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	return renderTurnToolOutcomeSections(m, state, refs, width)
}

func (shellTranscriptToolEntryRenderer) RenderOne(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool) {
	return renderSingleToolTranscriptSection(m, state, turnID, turn, callID, call, width)
}

func (wideTranscriptToolEntryRenderer) BatchConsecutive() bool { return true }

func (wideTranscriptToolEntryRenderer) RenderBatch(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	return renderTurnToolOutcomeSections(m, state, refs, width)
}

func (wideTranscriptToolEntryRenderer) RenderOne(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool) {
	return renderSingleToolTranscriptSection(m, state, turnID, turn, callID, call, width)
}

func (classicTranscriptToolEntryRenderer) BatchConsecutive() bool { return false }

func (classicTranscriptToolEntryRenderer) RenderBatch(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	return renderTurnToolOutcomeSections(m, state, refs, width)
}

func (classicTranscriptToolEntryRenderer) RenderOne(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool) {
	return renderSingleToolTranscriptSection(m, state, turnID, turn, callID, call, width)
}

func renderSingleToolTranscriptSection(m Model, state events.SessionState, turnID string, turn *events.TurnState, callID string, call *events.ToolCallState, width int) (transcriptSection, bool) {
	if section := strings.TrimSpace(renderTurnToolTranscriptSection(m, state, turnID, turn, callID, call, width)); section != "" {
		ref := sessionToolCallRef{TurnID: turnID, CallID: callID}
		return transcriptSection{content: section, toolRefs: []sessionToolCallRef{ref}}, true
	}
	return transcriptSection{}, false
}
