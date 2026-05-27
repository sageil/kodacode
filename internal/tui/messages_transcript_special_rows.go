package tui

import (
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type transcriptSpecialRowKind string

const (
	transcriptSpecialRowCompaction transcriptSpecialRowKind = "compaction"
	transcriptSpecialRowReasoning  transcriptSpecialRowKind = "reasoning"
)

type transcriptSpecialRow struct {
	kind     transcriptSpecialRowKind
	turnID   string
	entrySeq int64
	entryIdx int
	text     string
	width    int
	dimmed   bool
	focused  bool
}

func newHistoryCompactionTranscriptRow(turnID string, entry events.TranscriptEntryState, entryIdx int, width int) transcriptSpecialRow {
	return transcriptSpecialRow{
		kind:     transcriptSpecialRowCompaction,
		turnID:   strings.TrimSpace(turnID),
		entrySeq: entry.Sequence,
		entryIdx: entryIdx,
		text:     strings.TrimSpace(entry.Text),
		width:    max(width, 1),
	}
}

func newFallbackHistoryCompactionTranscriptRow(state events.SessionState, turnID string, turn *events.TurnState, width int) (transcriptSpecialRow, bool) {
	if suppressInheritedHistoryContinuation(turn) {
		return transcriptSpecialRow{}, false
	}
	compaction := effectiveTurnHistoryContinuation(state, turnID, turn)
	if turn == nil || compaction == nil {
		return transcriptSpecialRow{}, false
	}
	text := strings.TrimSpace(historyCompactionSummaryText(compaction))
	if text == "" {
		return transcriptSpecialRow{}, false
	}
	return transcriptSpecialRow{
		kind:     transcriptSpecialRowCompaction,
		turnID:   strings.TrimSpace(turnID),
		entrySeq: -1,
		entryIdx: -1,
		text:     text,
		width:    max(width, 1),
	}, true
}

func newReasoningTranscriptRow(turnID string, entry events.TranscriptEntryState, entryIdx int, width int, dimmed bool) transcriptSpecialRow {
	return transcriptSpecialRow{
		kind:     transcriptSpecialRowReasoning,
		turnID:   strings.TrimSpace(turnID),
		entrySeq: entry.Sequence,
		entryIdx: entryIdx,
		text:     strings.TrimSpace(entry.Text),
		width:    max(width, 1),
		dimmed:   dimmed,
	}
}

func (r transcriptSpecialRow) section(m Model) (transcriptSection, bool) {
	if r.text == "" {
		return transcriptSection{}, false
	}
	content := r.render(m)
	if strings.TrimSpace(content) == "" {
		return transcriptSection{}, false
	}
	return transcriptSection{content: content}, true
}

func (r transcriptSpecialRow) render(m Model) string {
	return cachedTranscriptRender("special_row", m, r.width, func() string {
		return r.renderUncached(m)
	}, r.cacheParts()...)
}

func (r transcriptSpecialRow) renderUncached(m Model) string {
	switch r.kind {
	case transcriptSpecialRowCompaction:
		return renderHistoryCompactionSummarySection(m, r.text, r.width)
	case transcriptSpecialRowReasoning:
		return renderReasoningTranscriptSection(m, r.text, r.width, r.dimmed)
	default:
		return ""
	}
}

func (r transcriptSpecialRow) cacheParts() []string {
	return []string{
		string(r.kind),
		strings.TrimSpace(r.turnID),
		strconv.FormatInt(r.entrySeq, 10),
		strconv.Itoa(r.entryIdx),
		strconv.FormatBool(r.dimmed),
		strconv.FormatBool(r.focused),
		r.text,
	}
}
