package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

const (
	shellToolRowKindWidthWide   = 8
	shellToolRowKindWidthNarrow = 6
)

type shellToolTranscriptRow struct {
	ref         sessionToolCallRef
	kind        string
	label       string
	status      string
	selected    bool
	turnOrdinal int
	width       int
}

func newShellToolTranscriptRow(state events.SessionState, row toolOutcomeRow, call *events.ToolCallState, width int, selected bool) shellToolTranscriptRow {
	status := strings.TrimSpace(row.Status)
	if status == "" {
		status = toolStatus(call)
	}
	label := shellToolRowLabel(state, row, call)
	if label == "" {
		label = shellToolPrimaryLabel(state, call)
	}
	if label == "" {
		label = shellToolKind(call)
	}
	return shellToolTranscriptRow{
		ref:         row.Ref,
		kind:        shellToolRowKind(row, call),
		label:       label,
		status:      normalizeOutcomeStatus(status),
		selected:    selected,
		turnOrdinal: sessionToolTurnOrdinal(state, row.Ref.TurnID),
		width:       max(width, 1),
	}
}

func (r shellToolTranscriptRow) render(m Model) string {
	return cachedTranscriptRender("shell_tool_row", m, r.width, func() string {
		return r.renderUncached(m)
	}, r.cacheParts()...)
}

func (r shellToolTranscriptRow) cacheParts() []string {
	return []string{
		strings.TrimSpace(r.ref.SessionID),
		strings.TrimSpace(r.ref.TurnID),
		strings.TrimSpace(r.ref.CallID),
		strings.TrimSpace(r.kind),
		strings.TrimSpace(r.label),
		normalizeOutcomeStatus(r.status),
		strconv.FormatBool(r.selected),
		strconv.Itoa(r.turnOrdinal),
	}
}

func (r shellToolTranscriptRow) renderUncached(m Model) string {
	return r.leftText(m, r.width)
}

func (r shellToolTranscriptRow) leftText(m Model, width int) string {
	width = max(width, 1)
	marker := " "
	if r.selected {
		marker = ">"
	}
	kindWidth := shellToolRowKindWidth(width)
	icon := m.toolStatusSymbol(r.status)
	prefixPlain := marker + " " + icon + " "
	if kindWidth > 0 && strings.TrimSpace(r.kind) != "" {
		prefixPlain += padRight(truncateEnd(r.kind, kindWidth), kindWidth) + " "
	}
	labelWidth := max(width-lipgloss.Width(prefixPlain), 1)
	text := truncateEnd(r.label, labelWidth)

	markerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	if r.selected {
		markerStyle = markerStyle.Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).Bold(true)
	}
	iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(toolStatusColorHex(m.theme, r.status)))
	kindStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(shellToolLabelColor(m.theme, r.status)))
	if r.selected && normalizeOutcomeStatus(r.status) != "error" {
		labelStyle = labelStyle.Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).Bold(true)
	}

	parts := []string{
		markerStyle.Render(marker),
		" ",
		iconStyle.Render(icon),
		" ",
	}
	if kindWidth > 0 && strings.TrimSpace(r.kind) != "" {
		parts = append(parts, kindStyle.Render(padRight(truncateEnd(r.kind, kindWidth), kindWidth)), " ")
	}
	parts = append(parts, labelStyle.Render(text))
	return strings.Join(parts, "")
}

func renderShellTurnToolOutcomeSections(m Model, state events.SessionState, refs []sessionToolCallRef, width int) []transcriptSection {
	rows := deriveUngroupedToolOutcomeRows(state, refs)
	if len(rows) == 0 {
		return nil
	}
	sections := make([]transcriptSection, 0, len(rows))
	compactLines := make([]string, 0, len(rows))
	compactRefs := make(map[sessionToolCallRef]int, len(rows))
	flushCompact := func() {
		if len(compactLines) == 0 {
			return
		}
		lineRefs := make(map[sessionToolCallRef]int, len(compactRefs))
		for ref, line := range compactRefs {
			lineRefs[ref] = line
		}
		sections = append(sections, transcriptSection{
			content:      strings.Join(compactLines, "\n"),
			toolLineRefs: lineRefs,
		})
		compactLines = compactLines[:0]
		clear(compactRefs)
	}
	for _, row := range rows {
		turn, call := sessionToolCall(state, row.Ref)
		if call != nil && strings.TrimSpace(call.ToolName) == "question" {
			flushCompact()
			if content := strings.TrimSpace(renderQuestionOutcomeTranscriptSection(m, state, row.Ref, call, width)); content != "" {
				sections = append(sections, transcriptSection{content: content, toolRefs: []sessionToolCallRef{row.Ref}})
			}
			continue
		}
		if isFailedApplyPatchToolCall(call) {
			selected := selectedToolMatchesSession(m, state.SessionID, row.Ref)
			expanded := expandedToolMatchesSession(m, state.SessionID, row.Ref)
			content := ""
			if expanded {
				content = strings.TrimSpace(renderShellFocusedToolTranscriptSection(m, row.Ref, state, call, width))
			}
			if content == "" {
				content = strings.TrimSpace(renderShellApplyPatchFailureTranscriptSection(m, call, width, selected))
			}
			if content != "" {
				flushCompact()
				sections = append(sections, transcriptSection{content: content, toolRefs: []sessionToolCallRef{row.Ref}})
			}
			continue
		}
		if row.Kind == toolOutcomeMutation || isMutationToolCall(call) {
			if content := strings.TrimSpace(renderShellMutationToolTranscriptSection(m, state, row.Ref, call, width)); content != "" {
				if transcriptRenderedLineCount(content) == 1 {
					compactRefs[row.Ref] = shellCompactLineOffset(compactLines)
					compactLines = append(compactLines, content)
					continue
				}
				flushCompact()
				sections = append(sections, transcriptSection{content: content, toolRefs: []sessionToolCallRef{row.Ref}})
			}
			continue
		}
		selected := selectedToolMatchesSession(m, state.SessionID, row.Ref)
		expanded := expandedToolMatchesSession(m, state.SessionID, row.Ref)
		if expanded {
			if content := strings.TrimSpace(renderShellFocusedToolTranscriptSection(m, row.Ref, state, call, width)); content != "" {
				flushCompact()
				sections = append(sections, transcriptSection{content: content, toolRefs: []sessionToolCallRef{row.Ref}})
				continue
			}
		}
		content := renderShellToolOutcomeLine(m, state, row, call, width, selected)
		if strings.TrimSpace(content) == "" {
			continue
		}
		compactRefs[row.Ref] = shellCompactLineOffset(compactLines)
		compactLines = append(compactLines, content)
		appendShellDelegatedChildToolRows(m, turn, call, width, &compactLines, compactRefs)
	}
	flushCompact()
	return sections
}

func appendShellDelegatedChildToolRows(m Model, turn *events.TurnState, call *events.ToolCallState, width int, lines *[]string, lineRefs map[sessionToolCallRef]int) {
	if lines == nil || lineRefs == nil {
		return
	}
	handoff := delegateHandoffForCall(turn, call)
	if handoff == nil || strings.TrimSpace(handoff.ChildSessionID) == "" {
		return
	}
	childSessionID := strings.TrimSpace(handoff.ChildSessionID)
	childState, ok := m.delegatedSnapshot(childSessionID)
	if !ok {
		if loading := strings.TrimSpace(delegatedInspectorLoadingLabel(m, handoff)); loading != "" {
			*lines = append(*lines, renderShellDelegatedInfoLine(m, loading, width))
		}
		return
	}
	appendShellChildToolRowsForState(m, childSessionID, childState, width, lines, lineRefs)
}

func renderShellDelegatedToolOutcomeSectionsForHandoffs(m Model, handoffs []*events.AgentHandoffState, width int) []transcriptSection {
	if !shellLayoutEnabled(m) || !m.shellToolCallsVisible || len(handoffs) == 0 {
		return nil
	}
	lines := make([]string, 0, len(handoffs)*2)
	lineRefs := make(map[sessionToolCallRef]int)
	for _, handoff := range handoffs {
		if handoff == nil {
			continue
		}
		appendShellDelegatedHandoffToolRows(m, handoff, width, &lines, lineRefs)
	}
	if len(lines) == 0 {
		return nil
	}
	return []transcriptSection{{
		content:      strings.Join(lines, "\n"),
		toolLineRefs: lineRefs,
	}}
}

func appendShellDelegatedHandoffToolRows(m Model, handoff *events.AgentHandoffState, width int, lines *[]string, lineRefs map[sessionToolCallRef]int) {
	if lines == nil || lineRefs == nil || handoff == nil {
		return
	}
	childSessionID := strings.TrimSpace(handoff.ChildSessionID)
	if childSessionID == "" {
		return
	}
	childState, ok := m.delegatedSnapshot(childSessionID)
	if !ok {
		if loading := strings.TrimSpace(delegatedInspectorLoadingLabel(m, handoff)); loading != "" {
			*lines = append(*lines, renderShellDelegatedInfoLine(m, loading, width))
		}
		return
	}
	appendShellChildToolRowsForState(m, childSessionID, childState, width, lines, lineRefs)
}

func appendShellChildToolRowsForState(m Model, childSessionID string, childState events.SessionState, width int, lines *[]string, lineRefs map[sessionToolCallRef]int) {
	childRefs := filterPendingQuestionToolRefs(m, orderedDelegatedChildToolCallRefs(childState))
	if len(childRefs) == 0 {
		return
	}
	childRows := deriveUngroupedToolOutcomeRows(childState, childRefs)
	for _, childRow := range childRows {
		_, childCall := sessionToolCall(childState, childRow.Ref)
		if childCall == nil {
			continue
		}
		childRef := toolRefForSession(childSessionID, childRow.Ref)
		childRow.Ref = childRef
		selected := selectedToolMatchesSession(m, childSessionID, childRef)
		expanded := expandedToolMatchesSession(m, childSessionID, childRef)
		content := ""
		if expanded {
			content = strings.TrimSpace(renderShellFocusedToolTranscriptSection(m, childRef, childState, childCall, max(width-2, 1)))
		}
		if content == "" && isFailedApplyPatchToolCall(childCall) {
			content = strings.TrimSpace(renderShellApplyPatchFailureTranscriptSection(m, childCall, max(width-2, 1), selected))
		}
		if content == "" {
			content = strings.TrimSpace(renderShellToolOutcomeLine(m, childState, childRow, childCall, max(width-2, 1), selected))
		}
		if content == "" {
			continue
		}
		lineRefs[childRef] = shellCompactLineOffset(*lines)
		*lines = append(*lines, indentShellDelegatedToolBlock(content))
	}
}

func shellCompactLineOffset(lines []string) int {
	offset := 0
	for _, line := range lines {
		offset += transcriptRenderedLineCount(line)
	}
	return offset
}

func renderShellDelegatedInfoLine(m Model, text string, width int) string {
	width = max(width, 1)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Width(width).
		Render("  " + truncateEnd(text, max(width-2, 1)))
}

func indentShellDelegatedToolBlock(block string) string {
	block = strings.TrimRight(block, "\n")
	if strings.TrimSpace(block) == "" {
		return ""
	}
	lines := strings.Split(block, "\n")
	for idx, line := range lines {
		lines[idx] = "  " + line
	}
	return strings.Join(lines, "\n")
}

func renderShellToolTranscriptSection(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int) string {
	if call != nil && strings.TrimSpace(call.ToolName) == "question" {
		return renderQuestionOutcomeTranscriptSection(m, state, ref, call, width)
	}
	rows := deriveUngroupedToolOutcomeRows(state, []sessionToolCallRef{ref})
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		_, rowCall := sessionToolCall(state, row.Ref)
		if rowCall == nil {
			rowCall = call
		}
		if isFailedApplyPatchToolCall(rowCall) {
			selected := selectedToolMatchesSession(m, state.SessionID, row.Ref)
			expanded := expandedToolMatchesSession(m, state.SessionID, row.Ref)
			if expanded {
				if content := strings.TrimSpace(renderShellFocusedToolTranscriptSection(m, row.Ref, state, rowCall, width)); content != "" {
					lines = append(lines, content)
					continue
				}
			}
			if content := strings.TrimSpace(renderShellApplyPatchFailureTranscriptSection(m, rowCall, width, selected)); content != "" {
				lines = append(lines, content)
			}
			continue
		}
		if row.Kind == toolOutcomeMutation || isMutationToolCall(rowCall) {
			if content := strings.TrimSpace(renderShellMutationToolTranscriptSection(m, state, row.Ref, rowCall, width)); content != "" {
				lines = append(lines, content)
			}
			continue
		}
		selected := selectedToolMatchesSession(m, state.SessionID, row.Ref)
		expanded := expandedToolMatchesSession(m, state.SessionID, row.Ref)
		if expanded {
			if content := strings.TrimSpace(renderShellFocusedToolTranscriptSection(m, row.Ref, state, rowCall, width)); content != "" {
				lines = append(lines, content)
				continue
			}
		}
		if line := strings.TrimSpace(renderShellToolOutcomeLine(m, state, row, rowCall, width, selected)); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func renderShellFocusedToolTranscriptSection(m Model, ref sessionToolCallRef, state events.SessionState, call *events.ToolCallState, width int) string {
	if call != nil && strings.TrimSpace(call.ToolName) == "bash" && outcomeCategoryForTool(call) == toolOutcomeExploration {
		return renderWideToolDetailTranscriptSection(m, ref, state, call, width)
	}
	return renderFocusedToolTranscriptSection(m, ref, state, call, width)
}

func renderShellMutationToolTranscriptSection(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	if display, ok := mutationDisplayFromCall(state.WorkspaceRoot, call); ok && display.Failure != nil {
		return strings.Join(renderMutationFailureSummaryLines(m, display, width), "\n")
	}
	lines := renderShellMutationSuccessLines(m, state, ref, call, width)
	if len(lines) == 0 {
		return renderMutationToolTimelineSection(m, state.WorkspaceRoot, call, width)
	}
	return strings.Join(lines, "\n")
}

func renderShellApplyPatchFailureTranscriptSection(m Model, call *events.ToolCallState, width int, selected bool) string {
	if call == nil {
		return ""
	}
	width = max(width, 1)
	title := "Edit failed"
	if selected {
		title = "> " + title
	}
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "error", "#ff9aa6")))
	titleStyle := errorStyle.Bold(true)
	lines := []string{titleStyle.Render(truncateEnd(title, width))}
	for _, line := range wrapTranscriptText(applyPatchFailureDisplayError(call.Error), width) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, errorStyle.Render(truncateEnd(line, width)))
	}
	return strings.Join(lines, "\n")
}

func applyPatchFailureDisplayError(errorText string) string {
	errorText = strings.TrimSpace(errorText)
	for _, prefix := range []string{
		"`apply_patch` failed",
		"apply_patch failed",
		"`apply_patch` error",
		"apply_patch error",
	} {
		if !strings.HasPrefix(errorText, prefix) {
			continue
		}
		errorText = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(strings.TrimPrefix(errorText, prefix)), ".:;- "))
		break
	}
	if errorText == "" {
		return "The edit could not be applied."
	}
	return uppercaseFirstASCII(errorText)
}

func uppercaseFirstASCII(text string) string {
	if text == "" {
		return ""
	}
	first := text[0]
	if first >= 'a' && first <= 'z' {
		return string(first-'a'+'A') + text[1:]
	}
	return text
}

func renderShellMutationSuccessLines(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int) []string {
	switch strings.TrimSpace(call.ToolName) {
	case "write":
		return renderWriteMutationLinesWithDiffStyle(m, state.SessionID, ref, state.WorkspaceRoot, call, width, mutationToolDetailDiffStyleAuto)
	case "apply_patch":
		return renderApplyPatchMutationLinesWithDiffStyle(m, state.WorkspaceRoot, call, width, mutationToolDetailDiffStyleAuto)
	case "bash":
		return renderBashMutationLinesWithDiffStyle(m, state.WorkspaceRoot, call, width, mutationToolDetailDiffStyleAuto)
	default:
		return renderMutationSuccessLines(m, state.WorkspaceRoot, call, width)
	}
}

func renderShellToolOutcomeLine(m Model, state events.SessionState, row toolOutcomeRow, call *events.ToolCallState, width int, selected bool) string {
	return newShellToolTranscriptRow(state, row, call, width, selected).render(m)
}

func shellToolRowLabel(state events.SessionState, row toolOutcomeRow, call *events.ToolCallState) string {
	label := strings.TrimSpace(row.Label)
	if shellLayoutToolRowShouldUseConciseLabel(row, call) {
		label = shellToolConciseLabel(state, call, label)
	}
	if detail := strings.TrimSpace(row.Detail); detail != "" {
		if label == "" {
			label = detail
		} else if !strings.Contains(label, detail) {
			label += " · " + detail
		}
	}
	return singleLineToolText(label)
}

func shellLayoutToolRowShouldUseConciseLabel(row toolOutcomeRow, call *events.ToolCallState) bool {
	return call != nil && row.Kind == toolOutcomeExploration
}

func shellToolConciseLabel(state events.SessionState, call *events.ToolCallState, fallback string) string {
	label := strings.TrimSpace(fallback)
	switch strings.TrimSpace(call.ToolName) {
	case "read":
		if input, ok := parseReadToolViewInput(call.Input); ok && len(input.Paths) == 1 {
			label = displayToolBaseName(state.WorkspaceRoot, input.Paths[0])
		} else {
			label = strings.TrimPrefix(label, "Read ")
			label = strings.TrimPrefix(label, "Reading ")
		}
	case "locate":
		if input, ok := parseLocateToolViewInput(call.Input); ok {
			path := displayToolPath(state.WorkspaceRoot, input.Path)
			query := strings.TrimSpace(input.Query)
			switch {
			case path != "." && path != "" && query != "":
				label = query + " under " + path
			case query != "":
				label = query
			case path != "." && path != "":
				label = path
			default:
				label = "workspace"
			}
		} else {
			label = strings.TrimPrefix(label, "Locate ")
			label = strings.TrimPrefix(label, "Locating ")
		}
	case "search":
		label = strings.TrimPrefix(label, "Search ")
		label = strings.TrimPrefix(label, "Searching ")
	case "bash":
		label = strings.TrimPrefix(label, "Shell: ")
		label = strings.TrimPrefix(label, "Shell")
	default:
		display := strings.TrimSpace(toolDisplayNameForSession(state, call))
		for _, prefix := range []string{display + " ", titleCaseASCII(display) + " "} {
			label = strings.TrimPrefix(label, prefix)
		}
	}
	if label == "" {
		label = strings.TrimSpace(fallback)
	}
	return label
}

func titleCaseASCII(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	first := value[0]
	if first >= 'a' && first <= 'z' {
		first -= 'a' - 'A'
	}
	return string(first) + value[1:]
}

func shellToolRowKind(row toolOutcomeRow, call *events.ToolCallState) string {
	if isTaskToolCall(call) {
		return ""
	}
	switch row.Kind {
	case toolOutcomeExploration:
		if call != nil {
			return shellToolKind(call)
		}
		return "scan"
	case toolOutcomeMutation:
		if call != nil {
			return shellToolKind(call)
		}
		return "write"
	case toolOutcomeCommand:
		if call != nil {
			return shellToolKind(call)
		}
		return "cmd"
	default:
		if call != nil {
			return shellToolKind(call)
		}
		return "tool"
	}
}

func shellToolRowKindWidth(width int) int {
	switch {
	case width >= 56:
		return shellToolRowKindWidthWide
	case width >= 38:
		return shellToolRowKindWidthNarrow
	default:
		return 0
	}
}
