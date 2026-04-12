package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/v1/internal/logging"
)

func (m *Messages) buildSubagentActivityRegions(msgIndex, startLine int) {
	msg := m.messages[msgIndex]
	line := startLine + 2 // +1 header, +1 blank line before activities
	for actIdx, act := range msg.SubagentActivities {
		m.subagentActivityRegions = append(m.subagentActivityRegions, subagentActivityRegion{
			line:          line,
			msgIndex:      msgIndex,
			activityIndex: actIdx,
		})
		line++
		if act.Expanded && act.Output != "" {
			n := strings.Count(strings.TrimRight(act.Output, "\n"), "\n") + 1
			if n > 20 {
				n = 21
			}
			line += n
		}
	}
}

// handleMouseClick processes a mouse click event and returns true if it was handled.
func (m *Messages) handleMouseClick(msg tea.MouseClickMsg) bool {
	m.userScrolled = true
	m.autoScroll = false

	// Match clicks against existing regions — these correspond to what the
	// user actually sees on screen. Do NOT re-render first: a pending render
	// would shift content positions, making regions disagree with the display.
	contentLine := msg.Y - m.screenY + m.vp.YOffset()
	logging.Debugf("click: Y=%d screenY=%d YOffset=%d → contentLine=%d (regions=%d hunks=%d msgs=%d)",
		msg.Y, m.screenY, m.vp.YOffset(), contentLine, len(m.toolRegions), len(m.hunkRegions), len(m.messages))

	// 1. Tool header clicks (first line of each tool region) — always toggle
	//    collapse, regardless of hunks or other overlapping regions.
	for _, tr := range m.toolRegions {
		if contentLine == tr.startLine {
			mi := tr.msgIndex
			tm := &m.messages[mi]
			tm.Collapsed = !tm.Collapsed
			tm.UserExpanded = !tm.Collapsed
			m.invalidateFrom(mi)
			m.needsRender = true
			return true
		}
	}

	// 2. Reasoning header clicks (first line of each msg region with reasoning).
	for _, mr := range m.msgRegions {
		if contentLine == mr.startLine {
			mm := &m.messages[mr.msgIndex]
			if mm.Role == "assistant" && mm.Reasoning != "" && mm.ReasoningDone {
				mm.ReasoningCollapsed = !mm.ReasoningCollapsed
				m.invalidateFrom(mr.msgIndex)
				m.needsRender = true
				return true
			}
		}
	}

	// 3. Hunk accept/reject toggles (exact line match inside tool body).
	for _, hr := range m.hunkRegions {
		if contentLine == hr.line {
			if hr.hunkIndex == -1 {
				logging.Debugf("click: hunk action bar → msgIndex=%d", hr.msgIndex)
				m.applyDiffReview(hr.msgIndex)
			} else {
				if dr, ok := m.diffReviews[hr.msgIndex]; ok && hr.hunkIndex < len(dr.hunks) {
					dr.hunks[hr.hunkIndex].Accepted = !dr.hunks[hr.hunkIndex].Accepted
					dr.dirty = true
					logging.Debugf("click: hunk %d toggled → Accepted=%v", hr.hunkIndex, dr.hunks[hr.hunkIndex].Accepted)
				}
			}
			m.invalidateFrom(hr.msgIndex)
			m.needsRender = true
			return true
		}
	}

	// 4. Subagent activity toggles.
	for _, sr := range m.subagentActivityRegions {
		if contentLine == sr.line {
			smsg := &m.messages[sr.msgIndex]
			if sr.activityIndex < len(smsg.SubagentActivities) {
				act := &smsg.SubagentActivities[sr.activityIndex]
				if act.Done && act.Output != "" {
					act.Expanded = !act.Expanded
					m.invalidateFrom(sr.msgIndex)
					m.needsRender = true
					return true
				}
			}
		}
	}

	// 5. Click anywhere else inside a tool body — toggle collapse.
	for _, tr := range m.toolRegions {
		if contentLine > tr.startLine && contentLine < tr.endLine {
			mi := tr.msgIndex
			tm := &m.messages[mi]
			tm.Collapsed = !tm.Collapsed
			tm.UserExpanded = !tm.Collapsed
			m.invalidateFrom(mi)
			m.needsRender = true
			return true
		}
	}

	// 6. Click anywhere inside a reasoning block — toggle collapse.
	for _, mr := range m.msgRegions {
		if contentLine > mr.startLine && contentLine < mr.endLine {
			mm := &m.messages[mr.msgIndex]
			if mm.Role == "assistant" && mm.Reasoning != "" && mm.ReasoningDone {
				mm.ReasoningCollapsed = !mm.ReasoningCollapsed
				m.invalidateFrom(mr.msgIndex)
				m.needsRender = true
				return true
			}
		}
	}

	if len(m.toolRegions) > 0 {
		logging.Debugf("click: MISS contentLine=%d, regions: %v",
			contentLine, func() string {
				var parts []string
				for _, tr := range m.toolRegions {
					parts = append(parts, fmt.Sprintf("[%d,%d)@msg%d", tr.startLine, tr.endLine, tr.msgIndex))
				}
				return strings.Join(parts, " ")
			}())
	}

	return false
}
