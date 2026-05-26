package tui

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type toolDetailDialog struct {
	id                string
	sessionID         string
	ref               sessionToolCallRef
	frameWidth        int
	frameHeight       int
	theme             *theme.Theme
	title             string
	subtitle          string
	body              Messages
	renderBody        func(width int) string
	markdownStreams   *streamingMarkdownSurfaceCache
	ctx               context.Context
	clipboard         clipboardWriter
	copyText          string
	copyLabel         string
	isMutation        bool
	mutationDiffStyle mutationToolDetailDiffStyle
}

const (
	toolDetailDialogNaturalWidth              = 156
	toolDetailDialogMinWidth                  = 72
	toolDetailDialogDefaultBodyHeight         = 24
	toolDetailDialogMaxBodyHeight             = 28
	mutationToolDetailDialogNaturalWidth      = 220
	mutationToolDetailDialogDefaultBodyHeight = 28
	mutationToolDetailDialogMaxBodyHeight     = 36
	scrollableDetailDialogCardTone            = toneBGAlt
	scrollableDetailDialogBodyTone            = tonePanelAlt
)

func newToolDetailDialog(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) *toolDetailDialog {
	return newToolDetailDialogForSession(m, m.sessionID, state, ref, call)
}

func newToolDetailDialogForSession(m Model, sessionID string, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) *toolDetailDialog {
	body := NewMessagesWithTone(m.theme, scrollableDetailDialogBodyTone)
	body.SetSoftWrap(false)
	dialog := &toolDetailDialog{
		id:              dialogIDToolDetail,
		sessionID:       strings.TrimSpace(sessionID),
		ref:             ref,
		frameWidth:      108,
		frameHeight:     32,
		theme:           m.theme,
		body:            body,
		markdownStreams: newStreamingMarkdownSurfaceCache(16),
		ctx:             m.ctx,
		clipboard:       m.clipboard,
	}
	width, height := dialogRenderSize(m, state)
	dialog.SetFrame(width, height)
	dialog.Sync(m, sessionID, state, ref, call)
	return dialog
}

func (d *toolDetailDialog) ID() string { return d.id }

func (d *toolDetailDialog) ignoreWheel(msg tea.MouseWheelMsg) bool {
	return shouldDropVerticalWheel(d.body, msg)
}

func (d *toolDetailDialog) wheelState() (int, bool) {
	return d.body.YOffset(), d.body.AtBottom()
}

func (d *toolDetailDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	d.body.ApplyTheme(th)
}

func (d *toolDetailDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	dialogWidth := d.dialogWidth()
	bodyWidth := toolDetailDialogBodyWidth(dialogWidth)
	bodyHeight := d.bodyHeight()
	prevBodyWidth := max(d.body.Width(), 1)
	d.body.SetSize(bodyWidth, bodyHeight)
	if d.renderBody != nil && bodyWidth != prevBodyWidth {
		d.body.Sync(d.renderBody(bodyWidth), false)
	}
}

func (d *toolDetailDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		switch typed.String() {
		case "q", "esc", "ctrl+c":
			return d, closeDialog(d.id, nil)
		case "c", "y":
			return d, d.copyCmd()
		case "tab":
			if d.isMutation {
				d.mutationDiffStyle = d.mutationDiffStyle.next()
				d.syncBody()
				return d, nil
			}
		case "up", "k":
			d.body.ScrollUp(1)
			return d, nil
		case "down", "j":
			d.body.ScrollDown(1)
			return d, nil
		case "pgup", "ctrl+u":
			d.body.PageUp()
			return d, nil
		case "pgdown", "ctrl+d":
			d.body.PageDown()
			return d, nil
		case "home", "g":
			d.body.GotoTop()
			return d, nil
		case "end", "G":
			d.body.GotoBottom()
			return d, nil
		}
	case tea.MouseWheelMsg:
		cmd := d.body.Update(typed)
		return d, cmd
	}
	return d, nil
}

func (d *toolDetailDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	summary := strings.TrimSpace(d.title)
	if meta := strings.TrimSpace(d.subtitle); meta != "" {
		if summary != "" {
			summary += " • " + meta
		} else {
			summary = meta
		}
	}
	width := d.dialogWidth()
	content := renderScrollableDetailDialogContent(
		d.theme,
		max(width-dialogFrameInset*2, 1),
		summary,
		renderToolDetailDialogViewport(d.theme, d.body, toolDetailDialogContentWidth(width)),
		d.hint(),
	)
	return drawDialogFrameOnSurfaceWithTone(surface, area, d.theme, width, content, nil, scrollableDetailDialogCardTone)
}

func (d *toolDetailDialog) Sync(m Model, sessionID string, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) {
	d.sessionID = strings.TrimSpace(sessionID)
	d.ref = ref
	d.ctx = m.ctx
	d.clipboard = m.clipboard
	d.isMutation = toolDetailDialogUsesMutationLayout(call)
	d.title = toolDetailDialogTitle(state, ref, call)
	d.subtitle = toolDetailDialogSubtitle(state, ref, call)
	d.copyText, d.copyLabel = toolDetailDialogCopyPayload(call)
	d.renderBody = func(width int) string {
		return toolDetailDialogBodyWithStyle(m, d.markdownStreams, d.sessionID, state, ref, call, width, d.mutationDiffStyle)
	}
	wasEmpty := strings.TrimSpace(d.body.raw) == ""
	d.syncBody()
	if wasEmpty {
		d.body.GotoTop()
	}
}

func (d *toolDetailDialog) OverlayCacheKey() uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, d.title)
	writeTranscriptSignatureString(hasher, d.subtitle)
	writeTranscriptSignatureBool(hasher, d.isMutation)
	writeTranscriptSignatureInt(hasher, int(d.mutationDiffStyle))
	writeTranscriptSignatureString(hasher, d.hint())
	appendMessagesRenderCacheSignature(hasher, d.body)
	return hasher.Sum64()
}

func toolDetailDialogWidth(frameWidth int) int {
	return toolDetailDialogWidthForMutation(false, frameWidth)
}

func toolDetailDialogWidthForMutation(isMutation bool, frameWidth int) int {
	if frameWidth <= 0 {
		if isMutation {
			return mutationToolDetailDialogNaturalWidth
		}
		return toolDetailDialogNaturalWidth
	}
	available := max(frameWidth-4, 1)
	if available <= toolDetailDialogMinWidth {
		return available
	}
	naturalWidth := toolDetailDialogNaturalWidth
	if isMutation {
		naturalWidth = mutationToolDetailDialogNaturalWidth
	}
	return min(available, naturalWidth)
}

func toolDetailDialogBodyHeight(frameHeight int) int {
	return toolDetailDialogBodyHeightForMutation(false, frameHeight)
}

func toolDetailDialogBodyHeightForMutation(isMutation bool, frameHeight int) int {
	if frameHeight <= 0 {
		if isMutation {
			return mutationToolDetailDialogDefaultBodyHeight
		}
		return toolDetailDialogDefaultBodyHeight
	}
	available := max(frameHeight-8, 5)
	maxBodyHeight := toolDetailDialogMaxBodyHeight
	if isMutation {
		maxBodyHeight = mutationToolDetailDialogMaxBodyHeight
	}
	return min(available, maxBodyHeight)
}

func toolDetailDialogContentWidth(dialogWidth int) int {
	return max(dialogWidth-dialogFrameInset*2, 1)
}

func toolDetailDialogBodyWidth(dialogWidth int) int {
	return max(toolDetailDialogContentWidth(dialogWidth)-transcriptScrollbarWidth, 1)
}

func renderToolDetailDialogViewport(th *theme.Theme, body Messages, width int) string {
	height := min(max(body.TotalLineCount(), 1), max(body.Height(), 1))
	content := strings.TrimRight(body.View(), "\n")
	maxContentWidth := max(width-transcriptScrollbarWidth, 1)
	contentWidth := min(max(blockWidth(content), 1), maxContentWidth)
	lines := strings.Split(content, "\n")
	gutter := renderToolDetailDialogScrollbar(th, body, height)
	bg := toneValue(th, body.surfaceTone())
	viewportWidth := contentWidth + transcriptScrollbarWidth

	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", contentWidth))
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	for i := range lines {
		if i < len(gutter) {
			lines[i] += gutter[i]
		}
		if strings.TrimSpace(bg) != "" {
			lines[i] = fillBackground(viewportWidth, bg, lines[i])
		}
	}
	return strings.Join(lines, "\n")
}

func renderToolDetailDialogScrollbar(th *theme.Theme, body Messages, height int) []string {
	if height <= 0 {
		return nil
	}
	total := body.TotalLineCount()
	visible := max(body.Height(), 1)
	if total <= visible {
		return blankTranscriptScrollbar(height)
	}

	offset := body.YOffset()
	thumbHeight := max(1, height*height/max(total, 1))
	if thumbHeight > height {
		thumbHeight = height
	}
	trackSpan := max(height-thumbHeight, 0)
	scrollSpan := max(total-visible, 1)
	thumbStart := 0
	if trackSpan > 0 {
		thumbStart = offset * trackSpan / scrollSpan
	}

	thumbStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(th, "primary", "#7aa2f7")))

	lines := blankTranscriptScrollbar(height)
	for i := 0; i < height; i++ {
		if i >= thumbStart && i < thumbStart+thumbHeight {
			lines[i] = thumbStyle.Render(transcriptScrollbarThumbGlyph)
		}
	}
	return lines
}

func renderScrollableDetailDialogContent(th *theme.Theme, contentWidth int, subtitle, viewport, hint string) string {
	parts := make([]string, 0, 5)
	if strings.TrimSpace(subtitle) != "" {
		parts = append(parts, renderDialogFooterHint(th, contentWidth, subtitle))
	}
	if strings.TrimSpace(viewport) != "" {
		if len(parts) > 0 {
			parts = append(parts, renderDialogSpacer(contentWidth))
		}
		parts = append(parts, viewport)
	}
	if strings.TrimSpace(hint) != "" {
		if len(parts) > 0 {
			parts = append(parts, renderDialogSpacer(contentWidth))
		}
		parts = append(parts, renderDialogFooterHint(th, contentWidth, hint))
	}

	content := strings.Join(parts, "\n")
	contentHeight := max(lipgloss.Height(content), 1)
	return renderDialogPlainBlock(contentWidth, contentHeight, content)
}

func toolDetailDialogTitle(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) string {
	if call == nil {
		return "Tool"
	}
	title := strings.TrimSpace(toolDisplayNameForSession(state, call))
	if title == "" {
		title = "tool"
	}
	return title
}

func toolDetailDialogSubtitle(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	parts := []string{toolStatus(call)}
	if ordinal := sessionToolTurnOrdinal(state, ref.TurnID); ordinal > 0 {
		parts = append(parts, fmt.Sprintf("turn %d", ordinal))
	}
	return strings.Join(parts, " • ")
}

func toolDetailDialogBody(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int) string {
	return toolDetailDialogBodyWithStyle(m, m.renderCache.transcriptMarkdown, m.sessionID, state, ref, call, width, mutationToolDetailDiffStyleAuto)
}

func toolDetailDialogBodyWithStyle(m Model, streamCache *streamingMarkdownSurfaceCache, sessionID string, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState, width int, diffStyle mutationToolDetailDiffStyle) string {
	if call == nil {
		return ""
	}
	width = max(width, 1)
	switch {
	case isMutationToolCall(call):
		if strings.TrimSpace(call.Error) != "" {
			body := renderGenericToolDetailMarkdownForSession(m, sessionID, ref, call)
			if strings.TrimSpace(body) == "" {
				return ""
			}
			return strings.Join(renderMarkdownBlockOnSurfaceWithStreamCache(m, body, width, "", streamCache, toolMarkdownStreamKey(sessionID, ref, "tool-detail-dialog")), "\n")
		}
		return renderMutationToolDetailSectionForSession(m, sessionID, state.WorkspaceRoot, ref, call, width, diffStyle)
	default:
		body := toolDetailDialogMarkdownBodyForSession(m, sessionID, state, ref, call)
		if strings.TrimSpace(body) == "" {
			return ""
		}
		return strings.Join(renderMarkdownBlockOnSurfaceWithStreamCache(m, body, width, "", streamCache, toolMarkdownStreamKey(sessionID, ref, "tool-detail-dialog")), "\n")
	}
}

func (d *toolDetailDialog) dialogWidth() int {
	return toolDetailDialogWidthForMutation(d.isMutation, d.frameWidth)
}

func (d *toolDetailDialog) bodyHeight() int {
	return toolDetailDialogBodyHeightForMutation(d.isMutation, d.frameHeight)
}

func (d *toolDetailDialog) hint() string {
	if d.isMutation {
		hint := "q close"
		if strings.TrimSpace(d.copyText) != "" {
			hint += " • c copy"
		}
		return hint + " • tab diff view (" + d.mutationDiffStyle.label() + ") • ↑/↓ scroll • pgup/pgdn page"
	}
	hint := "q close"
	if strings.TrimSpace(d.copyText) != "" {
		hint += " • c copy command"
	}
	return hint + " • ↑/↓ scroll • pgup/pgdn page"
}

func (d *toolDetailDialog) syncBody() {
	if d == nil || d.renderBody == nil {
		return
	}
	d.body.Sync(d.renderBody(max(d.body.Width(), 1)), false)
}

func toolDetailDialogUsesMutationLayout(call *events.ToolCallState) bool {
	return isMutationToolCall(call) && call.Completed && strings.TrimSpace(call.Error) == ""
}

func (d *toolDetailDialog) copyCmd() tea.Cmd {
	if d == nil || strings.TrimSpace(d.copyText) == "" {
		return nil
	}
	writer := d.clipboard
	if writer == nil {
		writer = systemClipboardWriter{}
	}
	text := normalizeTranscriptCopyText(d.copyText)
	if text == "" {
		return nil
	}
	label := strings.TrimSpace(d.copyLabel)
	if label == "" {
		label = "Copied tool detail"
	}
	return func() tea.Msg {
		return transcriptCopiedMsg{
			label: label,
			err:   writer.WriteText(d.ctx, text),
		}
	}
}

func toolDetailDialogCopyPayload(call *events.ToolCallState) (string, string) {
	command := strings.TrimSpace(toolDetailCommandText(call))
	if command != "" {
		return command, "Copied command"
	}
	return "", ""
}
