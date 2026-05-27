package tui

import (
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type transcriptSystemRowKind string

const (
	transcriptSystemRowDelegatedPermission transcriptSystemRowKind = "delegated_permission"
)

type transcriptSystemRow struct {
	kind      transcriptSystemRowKind
	title     string
	body      string
	handoffID string
	signature string
	width     int
	focused   bool
}

func newDelegatedPermissionSystemRow(handoff *events.AgentHandoffState, width int) transcriptSystemRow {
	row := transcriptSystemRow{
		kind:      transcriptSystemRowDelegatedPermission,
		title:     "Delegated child waiting on approval",
		body:      "A delegated child turn is blocked on permission. Resolve the approval prompt in this transcript to continue.",
		handoffID: "",
		width:     max(width, 1),
	}
	if handoff != nil {
		row.handoffID = strings.TrimSpace(handoff.HandoffID)
		row.signature = transcriptHandoffSignature(handoff)
	}
	return row
}

func (r transcriptSystemRow) render(m Model) transcriptRender {
	if strings.TrimSpace(r.title) == "" && strings.TrimSpace(r.body) == "" {
		return transcriptRender{}
	}
	return transcriptRender{content: cachedTranscriptRender("system_row", m, r.width, func() string {
		return renderSystemSection(m, r.title, r.body, r.width)
	}, r.cacheParts()...)}
}

func (r transcriptSystemRow) cacheParts() []string {
	return []string{
		string(r.kind),
		strings.TrimSpace(r.handoffID),
		strconv.FormatBool(r.focused),
		strings.TrimSpace(r.signature),
		strings.TrimSpace(r.title),
		strings.TrimSpace(r.body),
	}
}
