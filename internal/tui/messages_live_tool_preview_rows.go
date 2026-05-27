package tui

import (
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type liveToolPreviewTranscriptRow struct {
	ref      sessionToolCallRef
	title    string
	status   string
	width    int
	shell    bool
	focused  bool
	shellRow shellToolTranscriptRow
}

func newLiveToolPreviewTranscriptRow(m Model, state events.SessionState, row toolOutcomeRow, width int) liveToolPreviewTranscriptRow {
	_, call := sessionToolCall(state, row.Ref)
	out := liveToolPreviewTranscriptRow{
		ref:    row.Ref,
		status: normalizeOutcomeStatus(row.Status),
		width:  max(width, 1),
		shell:  shellLayoutEnabled(m),
	}
	if out.shell {
		out.shellRow = newShellToolTranscriptRow(state, row, call, width, selectedToolMatchesSession(m, state.SessionID, row.Ref))
		return out
	}
	title := strings.TrimSpace(row.Label)
	if detail := strings.TrimSpace(row.Detail); detail != "" {
		if title == "" {
			title = detail
		} else {
			title += " · " + detail
		}
	}
	out.title = title
	return out
}

func (r liveToolPreviewTranscriptRow) section(m Model) (transcriptSection, bool) {
	content := strings.TrimSpace(r.render(m))
	if content == "" {
		return transcriptSection{}, false
	}
	return transcriptSection{
		content:  content,
		toolRefs: []sessionToolCallRef{r.ref},
	}, true
}

func (r liveToolPreviewTranscriptRow) render(m Model) string {
	return cachedTranscriptRender("live_tool_preview_row", m, r.width, func() string {
		return r.renderUncached(m)
	}, r.cacheParts()...)
}

func (r liveToolPreviewTranscriptRow) renderUncached(m Model) string {
	if r.shell {
		return r.shellRow.render(m)
	}
	return renderOutcomeSummaryTranscriptSection(m, r.title, "", r.status, r.width)
}

func (r liveToolPreviewTranscriptRow) cacheParts() []string {
	parts := []string{
		strings.TrimSpace(r.ref.TurnID),
		strings.TrimSpace(r.ref.CallID),
		strings.TrimSpace(r.title),
		normalizeOutcomeStatus(r.status),
		strconv.FormatBool(r.shell),
		strconv.FormatBool(r.focused),
	}
	if r.shell {
		parts = append(parts, r.shellRow.cacheParts()...)
	}
	return parts
}
