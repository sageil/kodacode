package tui

import (
	"fmt"
	"hash/fnv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type shellToolsDialogResult struct {
	Ref sessionToolCallRef
}

type shellToolsDialog struct {
	theme       *theme.Theme
	icons       terminalIconProfile
	sessionID   string
	state       events.SessionState
	refs        []sessionToolCallRef
	cursor      int
	offset      int
	frameWidth  int
	frameHeight int
}

const (
	shellToolsDialogMinWidth      = 48
	shellToolsDialogNaturalWidth  = 104
	shellToolsDialogDefaultHeight = 18
)

func newShellToolsDialog(m Model, state events.SessionState) *shellToolsDialog {
	dialog := &shellToolsDialog{
		theme:       m.theme,
		icons:       m.terminalIcons,
		sessionID:   strings.TrimSpace(m.sessionID),
		state:       state,
		refs:        shellToolsDialogRefs(state),
		frameWidth:  96,
		frameHeight: shellToolsDialogDefaultHeight,
	}
	selected := sessionToolCallRef{
		TurnID: strings.TrimSpace(m.selection.callTurnID),
		CallID: strings.TrimSpace(m.selection.callID),
	}
	if idx := indexOfToolCallRef(dialog.refs, selected); idx >= 0 {
		dialog.cursor = idx
	}
	dialog.ensureCursorVisible()
	return dialog
}

func shellToolsDialogRefs(state events.SessionState) []sessionToolCallRef {
	refs := orderedAllSessionToolCallRefs(state)
	out := make([]sessionToolCallRef, 0, len(refs))
	for _, ref := range refs {
		_, call := sessionToolCall(state, ref)
		if call == nil {
			continue
		}
		out = append(out, ref)
	}
	return out
}

func (d *shellToolsDialog) ID() string { return dialogIDShellTools }

func (d *shellToolsDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
}

func (d *shellToolsDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	d.ensureCursorVisible()
}

func (d *shellToolsDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		switch typed.String() {
		case "q", "esc", "ctrl+c":
			return d, closeDialog(d.ID(), nil)
		case "up", "k":
			d.moveCursor(-1)
			return d, nil
		case "down", "j":
			d.moveCursor(1)
			return d, nil
		case "pgup", "ctrl+u":
			d.moveCursor(-d.bodyHeight())
			return d, nil
		case "pgdown", "ctrl+d":
			d.moveCursor(d.bodyHeight())
			return d, nil
		case "home", "g":
			d.cursor = 0
			d.ensureCursorVisible()
			return d, nil
		case "end", "G":
			d.cursor = max(len(d.refs)-1, 0)
			d.ensureCursorVisible()
			return d, nil
		case "enter":
			ref, ok := d.selectedRef()
			if !ok {
				return d, nil
			}
			return d, closeDialog(d.ID(), shellToolsDialogResult{Ref: ref})
		}
	}
	return d, nil
}

func (d *shellToolsDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := d.dialogWidth()
	contentWidth := max(width-dialogFrameInset*2, 1)
	bodyHeight := d.bodyHeight()
	body := d.renderBody(max(contentWidth-2, 1), bodyHeight)
	prompt := dialogTitleStyle(d.theme).Render("tools") + " " + dialogHintStyle(d.theme).Render(d.countLabelWithRange())
	content := renderPaletteDialogContentSized(d.theme, contentWidth, dialogPaletteFrame{
		Prompt: prompt,
		Body:   body,
		Hint:   "j/k move · enter details · q close",
	}, bodyHeight)
	return drawDialogFrameOnSurfaceWithTone(surface, area, d.theme, width, content, nil, scrollableDetailDialogCardTone)
}

func (d *shellToolsDialog) Sync(m Model, state events.SessionState) {
	selected, _ := d.selectedRef()
	d.theme = m.theme
	d.sessionID = strings.TrimSpace(m.sessionID)
	d.state = state
	d.refs = shellToolsDialogRefs(state)
	switch {
	case len(d.refs) == 0:
		d.cursor = 0
	case strings.TrimSpace(selected.TurnID) != "":
		if idx := indexOfToolCallRef(d.refs, selected); idx >= 0 {
			d.cursor = idx
		} else {
			d.cursor = min(d.cursor, len(d.refs)-1)
		}
	default:
		d.cursor = min(d.cursor, len(d.refs)-1)
	}
	d.ensureCursorVisible()
}

func (d *shellToolsDialog) OverlayCacheKey() uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, d.sessionID)
	writeTranscriptSignatureInt(hasher, d.cursor)
	writeTranscriptSignatureInt(hasher, d.offset)
	for _, ref := range d.refs {
		writeTranscriptSignatureString(hasher, ref.TurnID)
		writeTranscriptSignatureString(hasher, ref.CallID)
		_, call := sessionToolCall(d.state, ref)
		if call == nil {
			continue
		}
		writeTranscriptSignatureString(hasher, call.ToolName)
		writeTranscriptSignatureString(hasher, toolStatus(call))
		writeTranscriptSignatureString(hasher, call.Input)
		writeTranscriptSignatureString(hasher, call.Output)
		writeTranscriptSignatureString(hasher, call.Error)
	}
	return hasher.Sum64()
}

func (d *shellToolsDialog) dialogWidth() int {
	return desiredDialogWidth(d.frameWidth, shellToolsDialogMinWidth, shellToolsDialogNaturalWidth)
}

func (d *shellToolsDialog) bodyHeight() int {
	if d.frameHeight <= 0 {
		return shellToolsDialogDefaultHeight
	}
	return min(max(d.frameHeight-8, 5), shellToolsDialogDefaultHeight)
}

func (d *shellToolsDialog) moveCursor(delta int) {
	if len(d.refs) == 0 {
		d.cursor = 0
		d.offset = 0
		return
	}
	d.cursor = max(min(d.cursor+delta, len(d.refs)-1), 0)
	d.ensureCursorVisible()
}

func (d *shellToolsDialog) ensureCursorVisible() {
	if len(d.refs) == 0 {
		d.cursor = 0
		d.offset = 0
		return
	}
	d.cursor = max(min(d.cursor, len(d.refs)-1), 0)
	height := max(d.bodyHeight(), 1)
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+height {
		d.offset = d.cursor - height + 1
	}
	d.offset = max(min(d.offset, max(len(d.refs)-height, 0)), 0)
}

func (d *shellToolsDialog) selectedRef() (sessionToolCallRef, bool) {
	if len(d.refs) == 0 || d.cursor < 0 || d.cursor >= len(d.refs) {
		return sessionToolCallRef{}, false
	}
	return d.refs[d.cursor], true
}

func (d *shellToolsDialog) countLabel() string {
	if len(d.refs) == 1 {
		return "1 tool"
	}
	return fmt.Sprintf("%d tools", len(d.refs))
}

func (d *shellToolsDialog) countLabelWithRange() string {
	if len(d.refs) == 0 {
		return "no tools"
	}
	height := max(d.bodyHeight(), 1)
	start := min(d.offset+1, len(d.refs))
	end := min(d.offset+height, len(d.refs))
	rangeLabel := fmt.Sprintf("%d-%d", start, end)
	if len(d.refs) <= height {
		return d.countLabel()
	}
	return d.countLabel() + " · " + rangeLabel
}

func (d *shellToolsDialog) renderBody(width, height int) string {
	width = max(width, 1)
	height = max(height, 1)
	if len(d.refs) == 0 {
		empty := dialogHintStyle(d.theme).Render("No tool calls in this session.")
		return renderDialogPlainBlock(width, height, empty)
	}
	d.ensureCursorVisible()
	lines := make([]string, 0, height)
	end := min(d.offset+height, len(d.refs))
	for idx := d.offset; idx < end; idx++ {
		ref := d.refs[idx]
		_, call := sessionToolCall(d.state, ref)
		if call == nil {
			continue
		}
		lines = append(lines, d.renderRow(ref, call, idx == d.cursor, width))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (d *shellToolsDialog) renderRow(ref sessionToolCallRef, call *events.ToolCallState, selected bool, width int) string {
	width = max(width, 1)
	status := toolStatus(call)
	rightText := d.renderRowRightText(ref, status)
	left := d.renderRowLeft(call, status, max(width-lipgloss.Width(rightText)-1, 1))
	line := joinShellToolRow(left, rightText, width)
	if selected {
		return dialogSelectedItemStyle(d.theme).Width(width).Render(line)
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(shellToolLabelColor(d.theme, status))).
		Width(width).
		Render(line)
}

func (d *shellToolsDialog) renderRowLeft(call *events.ToolCallState, status string, width int) string {
	icon := d.icons.ToolStatusSymbol(status)
	kind := shellToolKind(call)
	prefix := icon + " " + padRight(truncateEnd(kind, 10), 10) + " "
	label := shellToolPrimaryLabel(d.state, call)
	if label == "" {
		label = kind
	}
	if detail := strings.TrimSpace(groupedToolItemResultDetail(call)); detail != "" && !strings.Contains(label, detail) {
		label += " · " + detail
	}
	return prefix + truncateEnd(label, max(width-lipgloss.Width(prefix), 1))
}

func (d *shellToolsDialog) renderRowRightText(ref sessionToolCallRef, status string) string {
	parts := make([]string, 0, 2)
	if ordinal := sessionToolTurnOrdinal(d.state, ref.TurnID); ordinal > 0 {
		parts = append(parts, fmt.Sprintf("t%d", ordinal))
	}
	parts = append(parts, shellToolStatusLabel(status))
	return strings.Join(parts, " · ")
}

func shellToolKind(call *events.ToolCallState) string {
	if call == nil {
		return "tool"
	}
	name := strings.TrimSpace(call.ToolName)
	if name == "" {
		return "tool"
	}
	return name
}

func shellToolPrimaryLabel(state events.SessionState, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if command := strings.TrimSpace(commandToolDisplayName(state.WorkspaceRoot, call)); command != "" && command != strings.TrimSpace(call.ToolName) {
		return command
	}
	return strings.TrimSpace(toolDisplayNameForSession(state, call))
}

func shellToolStatusLabel(status string) string {
	switch normalizeOutcomeStatus(status) {
	case "error":
		return "error"
	case "running", "preparing", "building":
		return "running"
	case "done":
		return "done"
	default:
		return strings.TrimSpace(status)
	}
}

func toolStatusColorHex(th *theme.Theme, status string) string {
	switch normalizeOutcomeStatus(status) {
	case "error":
		return colorFor(th, "error", "#ff9aa6")
	case "running", "preparing", "building", "declared":
		return colorFor(th, "warning", "#ffd28f")
	case "done":
		return colorFor(th, "success", "#90e5b4")
	default:
		return colorFor(th, "subtext", "#9da8ca")
	}
}

func shellToolLabelColor(th *theme.Theme, status string) string {
	switch normalizeOutcomeStatus(status) {
	case "error":
		return colorFor(th, "error", "#ff9aa6")
	case "running", "preparing", "building":
		return colorFor(th, "warning", "#ffd28f")
	default:
		return colorFor(th, "text", "#ecf0ff")
	}
}

func joinShellToolRow(left, right string, width int) string {
	width = max(width, 1)
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := max(width-leftWidth-rightWidth, 1)
	return left + strings.Repeat(" ", gap) + right
}

func padRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := lipgloss.Width(value)
	if visible >= width {
		return value
	}
	return value + strings.Repeat(" ", width-visible)
}
