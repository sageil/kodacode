package tui

import (
	"hash"
	"hash/fnv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func transcriptPaneCacheKey(m Model, state events.SessionState, width int) uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, "transcript-pane")
	writeTranscriptSignatureInt(hasher, max(width, 1))
	appendTranscriptPaneCacheSignature(hasher, m, state)
	return hasher.Sum64()
}

func splitTranscriptPaneCacheKey(m Model, state events.SessionState, width, height int, borderless bool) uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, "split-transcript-pane")
	writeTranscriptSignatureInt(hasher, max(width, 1))
	writeTranscriptSignatureInt(hasher, max(height, 1))
	writeTranscriptSignatureBool(hasher, borderless)
	appendTranscriptPaneCacheSignature(hasher, m, state)
	return hasher.Sum64()
}

func appendTranscriptPaneCacheSignature(hasher hash.Hash64, m Model, state events.SessionState) {
	if hasher == nil {
		return
	}
	writeTranscriptSignatureString(hasher, modelThemeCacheKey(m))
	writeTranscriptSignatureBool(hasher, isWideShell(m))
	writeTranscriptSignatureString(hasher, string(m.chrome.focus))
	writeTranscriptSignatureString(hasher, strings.TrimSpace(m.turnID))
	writeTranscriptSignatureBool(hasher, dialogHidesTranscriptScrollbar(m.dialog))
	appendPendingTranscriptPromptSignature(hasher, state)
	writeTranscriptSignatureInt(hasher, m.interaction.cursor)
	writeTranscriptSignatureBool(hasher, m.pendingInteractionSubmissionInFlight())
	writeTranscriptSignatureInt(hasher, m.messages.Width())
	writeTranscriptSignatureInt(hasher, m.messages.Height())
	writeTranscriptSignatureInt(hasher, m.messages.YOffset())
	writeTranscriptSignatureInt64(hasher, m.messages.ContentVersion())
	writeTranscriptSignatureBool(hasher, m.messages.softWrap)
	writeTranscriptSignatureBool(hasher, m.transcriptView.cursorInitialized)
	writeTranscriptSignatureInt(hasher, m.transcriptView.cursorLine)
	writeTranscriptSignatureInt(hasher, m.transcriptView.cursorColumn)
	writeTranscriptSignatureInt(hasher, m.transcriptView.cursorGoalColumn)
	writeTranscriptSignatureBool(hasher, m.transcriptView.visualActive)
	writeTranscriptSignatureInt(hasher, m.transcriptView.visualAnchorLine)
	writeTranscriptSignatureInt(hasher, m.transcriptView.visualAnchorColumn)
}

func appendPendingTranscriptPromptSignature(hasher hash.Hash64, state events.SessionState) {
	if hasher == nil {
		return
	}
	if pending := pendingExecutionFromState(state); pending != nil {
		writeTranscriptSignatureString(hasher, "execution")
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.RequestID))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.TurnID))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.ToolName))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.Command))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.WorkingDirectory))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.Reason))
		for _, decision := range pending.AvailableDecisions {
			writeTranscriptSignatureString(hasher, string(decision))
		}
		return
	}
	if pending := pendingPermissionFromState(state); pending != nil {
		writeTranscriptSignatureString(hasher, "permission")
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.RequestID))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.TurnID))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.ToolName))
		writeTranscriptSignatureString(hasher, string(pending.Kind))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.Access))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.Path))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.WorkingDirectory))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.Command))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.Reason))
		return
	}
	if pending := pendingQuestionFromState(state); pending != nil {
		writeTranscriptSignatureString(hasher, "question")
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.QuestionID))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.TurnID))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.ToolName))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.PlanID))
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.Question))
		writeTranscriptSignatureBool(hasher, pending.Multiple)
		writeTranscriptSignatureString(hasher, strings.TrimSpace(pending.Purpose))
		for _, option := range pending.Options {
			writeTranscriptSignatureString(hasher, strings.TrimSpace(option))
		}
		if planID := strings.TrimSpace(pending.PlanID); planID != "" {
			if plan := state.Plans[planID]; plan != nil {
				writeTranscriptSignatureString(hasher, strings.TrimSpace(plan.Title))
			}
		}
		return
	}
	writeTranscriptSignatureString(hasher, "none")
}
