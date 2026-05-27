package tui

import (
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type transcriptMessageRowKind string

const (
	transcriptMessageRowUser             transcriptMessageRowKind = "user"
	transcriptMessageRowAssistant        transcriptMessageRowKind = "assistant"
	transcriptMessageRowAssistantPreview transcriptMessageRowKind = "assistant_preview"
	transcriptMessageRowWorklog          transcriptMessageRowKind = "worklog"
)

type transcriptMessageRow struct {
	kind      transcriptMessageRowKind
	turnID    string
	entrySeq  int64
	entryIdx  int
	text      string
	streamKey string
	width     int
	focused   bool
}

func newUserTranscriptMessageRow(turnID string, entry events.TranscriptEntryState, entryIdx int, width int) transcriptMessageRow {
	return transcriptMessageRow{
		kind:     transcriptMessageRowUser,
		turnID:   strings.TrimSpace(turnID),
		entrySeq: entry.Sequence,
		entryIdx: entryIdx,
		text:     strings.TrimSpace(entry.Text),
		width:    max(width, 1),
	}
}

func newAssistantTranscriptMessageRow(state events.SessionState, turnID string, entry events.TranscriptEntryState, entryIdx int, width int) transcriptMessageRow {
	kind := transcriptMessageRowAssistant
	if entry.Kind == events.TranscriptEntryWorklog {
		kind = transcriptMessageRowWorklog
	}
	return transcriptMessageRow{
		kind:      kind,
		turnID:    strings.TrimSpace(turnID),
		entrySeq:  entry.Sequence,
		entryIdx:  entryIdx,
		text:      strings.TrimRight(strings.TrimSpace(entry.Text), "\n"),
		streamKey: assistantTranscriptEntryStreamKey(state.SessionID, turnID, entryIdx),
		width:     max(width, 1),
	}
}

func newAssistantPreviewTranscriptMessageRow(state events.SessionState, turnID string, turn *events.TurnState, width int) transcriptMessageRow {
	text := ""
	if turn != nil {
		text = turn.StreamingText
	}
	return transcriptMessageRow{
		kind:      transcriptMessageRowAssistantPreview,
		turnID:    strings.TrimSpace(turnID),
		entrySeq:  -1,
		entryIdx:  -1,
		text:      strings.TrimRight(strings.TrimSpace(text), "\n"),
		streamKey: assistantPreviewTranscriptStreamKey(state.SessionID, turnID),
		width:     max(width, 1),
	}
}

func (r transcriptMessageRow) section(m Model, turn *events.TurnState) (transcriptSection, bool) {
	switch r.kind {
	case transcriptMessageRowUser:
		if r.text == "" {
			return transcriptSection{}, false
		}
		return transcriptSection{
			content:        r.render(m, turn),
			selectionLines: r.selectionLines(m),
		}, true
	case transcriptMessageRowAssistant, transcriptMessageRowWorklog, transcriptMessageRowAssistantPreview:
		if isLocalShellTurn(turn) || turn == nil || r.text == "" {
			return transcriptSection{}, false
		}
		if r.kind == transcriptMessageRowAssistantPreview && turn.Config != nil && turn.Config.HideAssistantPreview {
			return transcriptSection{}, false
		}
		content := r.render(m, turn)
		if strings.TrimSpace(content) == "" {
			return transcriptSection{}, false
		}
		return transcriptSection{
			content:        content,
			selectionLines: r.selectionLines(m),
		}, true
	default:
		return transcriptSection{}, false
	}
}

func (r transcriptMessageRow) render(m Model, turn *events.TurnState) string {
	return cachedTranscriptRender("message_row", m, r.width, func() string {
		return r.renderUncached(m, turn)
	}, r.cacheParts()...)
}

func (r transcriptMessageRow) renderUncached(m Model, turn *events.TurnState) string {
	switch r.kind {
	case transcriptMessageRowUser:
		return renderUserSection(m, r.width, r.text)
	case transcriptMessageRowAssistant, transcriptMessageRowWorklog:
		return renderAssistantTranscriptSectionWithStreamKey(m, turn, r.text, r.width, r.streamKey)
	case transcriptMessageRowAssistantPreview:
		return renderAssistantPreviewTranscriptSectionWithStreamKey(m, turn, r.text, r.width, r.streamKey)
	default:
		return ""
	}
}

func (r transcriptMessageRow) selectionLines(m Model) []transcriptSelectionLine {
	switch r.kind {
	case transcriptMessageRowUser:
		return transcriptRailSelectionLines(m, r.text, r.width)
	case transcriptMessageRowAssistant, transcriptMessageRowWorklog, transcriptMessageRowAssistantPreview:
		return assistantTranscriptSelectionLinesWithStreamKey(m, r.text, r.width, r.streamKey)
	default:
		return nil
	}
}

func (r transcriptMessageRow) cacheParts() []string {
	return []string{
		string(r.kind),
		strings.TrimSpace(r.turnID),
		strconv.FormatInt(r.entrySeq, 10),
		strconv.Itoa(r.entryIdx),
		strings.TrimSpace(r.streamKey),
		strconv.FormatBool(r.focused),
		r.text,
	}
}
