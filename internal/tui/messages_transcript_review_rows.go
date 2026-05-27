package tui

import (
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type transcriptReviewRow struct {
	turnID    string
	entrySeq  int64
	entryIdx  int
	model     string
	review    *events.ReviewState
	signature string
	width     int
	focused   bool
}

func newReviewTranscriptRow(turnID string, turn *events.TurnState, entry events.TranscriptEntryState, entryIdx int, width int) transcriptReviewRow {
	model := ""
	if turn != nil && turn.Config != nil {
		model = strings.TrimSpace(turn.Config.Model)
	}
	review := cloneTranscriptReviewState(nil)
	if turn != nil {
		review = cloneTranscriptReviewState(turn.Review)
	}
	row := transcriptReviewRow{
		turnID:   strings.TrimSpace(turnID),
		entrySeq: entry.Sequence,
		entryIdx: entryIdx,
		model:    model,
		review:   review,
		width:    max(width, 1),
	}
	row.signature = reviewTranscriptPlainText(row.turnState())
	return row
}

func cloneTranscriptReviewState(review *events.ReviewState) *events.ReviewState {
	if review == nil {
		return nil
	}
	out := *review
	out.Findings = append([]events.ReviewFindingState(nil), review.Findings...)
	return &out
}

func (r transcriptReviewRow) section(m Model) (transcriptSection, bool) {
	if r.review == nil {
		return transcriptSection{}, false
	}
	content := r.render(m)
	if strings.TrimSpace(content) == "" {
		return transcriptSection{}, false
	}
	return transcriptSection{
		content:        content,
		selectionLines: r.selectionLines(m),
	}, true
}

func (r transcriptReviewRow) render(m Model) string {
	return cachedTranscriptRender("review_row", m, r.width, func() string {
		return renderReviewTranscriptSection(m, r.turnState(), r.width)
	}, r.cacheParts()...)
}

func (r transcriptReviewRow) selectionLines(m Model) []transcriptSelectionLine {
	return reviewTranscriptSelectionLines(m, r.turnState(), r.width)
}

func (r transcriptReviewRow) turnState() *events.TurnState {
	if r.review == nil {
		return nil
	}
	turn := &events.TurnState{
		TurnID: strings.TrimSpace(r.turnID),
		Review: cloneTranscriptReviewState(r.review),
	}
	if strings.TrimSpace(r.model) != "" {
		turn.Config = &events.TurnConfigState{Model: strings.TrimSpace(r.model)}
	}
	return turn
}

func (r transcriptReviewRow) cacheParts() []string {
	return []string{
		strings.TrimSpace(r.turnID),
		strconv.FormatInt(r.entrySeq, 10),
		strconv.Itoa(r.entryIdx),
		strings.TrimSpace(r.model),
		strconv.FormatBool(r.focused),
		strings.TrimSpace(r.signature),
	}
}
