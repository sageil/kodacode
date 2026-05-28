package tui

import (
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type transcriptDelegationRow struct {
	turnID     string
	handoffs   []*events.AgentHandoffState
	selectedID string
	width      int
	focused    bool
}

func newDelegationTranscriptRow(turnID string, turn *events.TurnState, selectedHandoffID string, width int) transcriptDelegationRow {
	if turn == nil || len(turn.HandoffOrder) == 0 {
		return transcriptDelegationRow{
			turnID: strings.TrimSpace(turnID),
			width:  max(width, 1),
		}
	}
	handoffs := make([]*events.AgentHandoffState, 0, len(turn.HandoffOrder))
	for _, handoffID := range turn.HandoffOrder {
		handoffs = append(handoffs, turn.Handoffs[handoffID])
	}
	return newDelegationTranscriptRowForHandoffs(turnID, handoffs, selectedHandoffID, width)
}

func newDelegationTranscriptRowForHandoffs(turnID string, handoffs []*events.AgentHandoffState, selectedHandoffID string, width int) transcriptDelegationRow {
	row := transcriptDelegationRow{
		turnID: strings.TrimSpace(turnID),
		width:  max(width, 1),
	}
	if len(handoffs) == 0 {
		return row
	}
	selectedID := strings.TrimSpace(selectedHandoffID)
	row.handoffs = make([]*events.AgentHandoffState, 0, len(handoffs))
	for _, handoff := range handoffs {
		if handoff == nil {
			continue
		}
		if selectedID != "" && strings.TrimSpace(handoff.HandoffID) == selectedID {
			row.selectedID = selectedID
		}
		if !shouldRenderDelegationRowInTranscript(handoff, row.selectedID) {
			continue
		}
		row.handoffs = append(row.handoffs, cloneTranscriptHandoffState(handoff))
	}
	return row
}

func cloneTranscriptHandoffState(state *events.AgentHandoffState) *events.AgentHandoffState {
	if state == nil {
		return nil
	}
	out := *state
	out.SourceHandoffIDs = append([]string(nil), state.SourceHandoffIDs...)
	out.ProvidedKinds = append([]string(nil), state.ProvidedKinds...)
	out.ExplorationEntries = append([]events.AgentHandoffExplorationEntry(nil), state.ExplorationEntries...)
	out.AllowedTools = append([]string(nil), state.AllowedTools...)
	out.QuestionOptions = append([]string(nil), state.QuestionOptions...)
	if state.ExecutionApproval != nil {
		approval := *state.ExecutionApproval
		out.ExecutionApproval = &approval
	}
	return &out
}

func (r transcriptDelegationRow) section(m Model) (transcriptSection, bool) {
	if len(r.handoffs) == 0 {
		return transcriptSection{}, false
	}
	content := r.render(m)
	if strings.TrimSpace(content) == "" {
		return transcriptSection{}, false
	}
	return transcriptSection{content: content}, true
}

func (r transcriptDelegationRow) render(m Model) string {
	return cachedTranscriptRender("delegation_row", m, r.width, func() string {
		return r.renderUncached(m)
	}, r.cacheParts()...)
}

func (r transcriptDelegationRow) renderUncached(m Model) string {
	rows := make([]string, 0, len(r.handoffs))
	for _, handoff := range r.handoffs {
		if handoff == nil {
			continue
		}
		rows = append(rows, renderDelegationRow(m, handoff, r.width, strings.TrimSpace(handoff.HandoffID) == strings.TrimSpace(r.selectedID)))
	}
	if len(rows) == 0 {
		return ""
	}
	return renderTranscriptBlock(m, "Delegation", strings.Join(rows, "\n"), r.width, transcriptBlockStyle{})
}

func (r transcriptDelegationRow) cacheParts() []string {
	parts := []string{
		strings.TrimSpace(r.turnID),
		strings.TrimSpace(r.selectedID),
		strconv.FormatBool(r.focused),
		strconv.Itoa(len(r.handoffs)),
	}
	for _, handoff := range r.handoffs {
		parts = append(parts, transcriptHandoffSignature(handoff))
	}
	return parts
}

func transcriptHandoffSignature(handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return ""
	}
	hasher := fnv.New64a()
	appendHandoffTranscriptSignature(hasher, handoff)
	return strconv.FormatUint(hasher.Sum64(), 16)
}
