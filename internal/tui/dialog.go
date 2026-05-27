package tui

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/sageil/kodacode/internal/tui/theme"
)

const (
	dialogIDCommandPalette    = "command-palette"
	dialogIDTheme             = "theme"
	dialogIDSessions          = "sessions"
	dialogIDSkills            = "skills"
	dialogIDConnect           = "connect"
	dialogIDReasoningVariant  = "reasoning-variant"
	dialogIDOpenAIAuth        = "openai-auth"
	dialogIDGitHubCopilotAuth = "github-copilot-auth"
	dialogIDCost              = "cost"
	dialogIDTrace             = "trace"
	dialogIDShellTools        = "shell-tools"
	dialogIDToolDetail        = "tool-detail"
	dialogIDHandoffDetail     = "handoff-detail"
	dialogIDTaskDetail        = "task-detail"
	dialogIDTrust             = "trust"
)

type dialogModel interface {
	ID() string
	ApplyTheme(*theme.Theme)
	SetFrame(width, height int)
	Update(msg tea.Msg) (dialogModel, tea.Cmd)
	Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor
}

type dialogInitialCommander interface {
	InitialCmd() tea.Cmd
}

type dialogOverlayCacheKeyer interface {
	OverlayCacheKey() uint64
}

type wheelBoundaryDialog interface {
	ignoreWheel(tea.MouseWheelMsg) bool
	wheelState() (int, bool)
}

type dialogPaletteFrame struct {
	Title  string
	Prompt string
	Body   string
	Hint   string
}

type dialogStandaloneFrame struct {
	Title string
	Body  string
	Hint  string
}

type dialogClosedMsg struct {
	id     string
	result any
}

type dialogRenderArea struct {
	x      int
	y      int
	width  int
	height int
}

type dialogSurface interface {
	Width() int
	Height() int
	Cell(x, y int) *cellbuf.Cell
	SetCell(x, y int, cell *cellbuf.Cell) bool
}

type dialogOpenedMsg struct {
	dialog dialogModel
	err    error
}

const dialogFrameInset = 1

func dialogTitleStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorFor(th, "primary", "#7aa2f7")))
}

func dialogItemStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(th, "subtext", "#a9b1d6")))
}

func dialogSelectedItemStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(dialogSurfaceTone(th))).
		Background(lipgloss.Color(colorFor(th, "primary", "#7aa2f7")))
}

func dialogHintStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(th, "soft", "#565f89")))
}

func dialogSectionStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorFor(th, "soft", "#565f89")))
}

func dialogSurfaceTone(th *theme.Theme) string {
	return toneValue(th, toneBGAlt)
}

func renderDialogPlainBlock(width, height int, content string) string {
	return placeBlock(max(width, 1), max(height, 1), "", content)
}

func renderDialogSpacer(width int) string {
	return renderDialogPlainBlock(width, 1, "")
}

func renderDialogFooterHint(th *theme.Theme, width int, hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ""
	}
	hint = ansi.Truncate(hint, max(width, 1), "…")
	centered := lipgloss.NewStyle().
		Width(max(width, 1)).
		Align(lipgloss.Center).
		Render(dialogHintStyle(th).Render(hint))
	return renderDialogPlainBlock(width, 1, centered)
}

func renderStandaloneDialogContent(th *theme.Theme, contentWidth int, frame dialogStandaloneFrame) string {
	parts := make([]string, 0, 3)
	if title := strings.TrimSpace(frame.Title); title != "" {
		title = truncateEnd(title, max(contentWidth-1, 1))
		parts = append(parts, renderDialogPlainBlock(contentWidth, 1, " "+dialogTitleStyle(th).Render(title)))
	}

	body := strings.TrimRight(strings.TrimSpace(frame.Body), "\n")
	if body != "" {
		if len(parts) > 0 {
			parts = append(parts, renderDialogSpacer(contentWidth))
		}
		parts = append(parts, renderDialogPlainBlock(contentWidth, max(lipgloss.Height(body), 1), body))
	}

	if hint := strings.TrimSpace(frame.Hint); hint != "" {
		if len(parts) > 0 {
			parts = append(parts, renderDialogSpacer(contentWidth))
		}
		parts = append(parts, renderDialogFooterHint(th, contentWidth, hint))
	}

	content := strings.Join(parts, "\n")
	contentHeight := max(lipgloss.Height(content), 1)
	return renderDialogPlainBlock(contentWidth, contentHeight, content)
}

func renderPaletteDialogContentSized(th *theme.Theme, contentWidth int, frame dialogPaletteFrame, bodyMinHeight int) string {
	parts := make([]string, 0, 3)
	if header := renderPaletteDialogHeader(th, contentWidth, frame); header != "" {
		parts = append(parts, header, renderPaletteDialogDivider(th, contentWidth))
	}
	if body := renderPaletteDialogBodySized(contentWidth, frame, bodyMinHeight); body != "" {
		parts = append(parts, body)
	}
	if hint := renderPaletteDialogHint(th, contentWidth, frame.Hint); hint != "" {
		if len(parts) > 0 {
			parts = append(parts, renderDialogSpacer(contentWidth))
		}
		parts = append(parts, hint)
	}
	content := strings.Join(parts, "\n")
	contentHeight := max(lipgloss.Height(content), 1)
	return renderDialogPlainBlock(contentWidth, contentHeight, content)
}

func renderPaletteDialogHeader(th *theme.Theme, width int, frame dialogPaletteFrame) string {
	if prompt := strings.TrimSpace(frame.Prompt); prompt != "" {
		if strings.HasPrefix(strings.TrimSpace(ansi.Strip(prompt)), "❯") {
			return renderDialogPlainBlock(width, 1, " "+prompt)
		}
		row := lipgloss.JoinHorizontal(
			lipgloss.Left,
			lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorFor(th, "primary", "#7aa2f7"))).
				Bold(true).
				Render("❯"),
			" ",
			prompt,
		)
		return renderDialogPlainBlock(width, 1, " "+row)
	}
	if title := strings.TrimSpace(frame.Title); title != "" {
		return renderDialogPlainBlock(width, 1, " "+dialogTitleStyle(th).Render(title))
	}
	return ""
}

func renderPaletteDialogDivider(th *theme.Theme, width int) string {
	return lipgloss.NewStyle().
		Width(max(width, 1)).
		Foreground(lipgloss.Color(dialogLineTone(th))).
		Render(strings.Repeat("─", max(width, 1)))
}

func renderPaletteDialogBodySized(width int, frame dialogPaletteFrame, minHeight int) string {
	body := strings.TrimRight(strings.TrimSpace(frame.Body), "\n")
	switch {
	case body == "" && minHeight <= 0:
		return ""
	}
	if body == "" {
		return renderDialogPlainBlock(width, max(minHeight, 1), "")
	}
	body = indentLines(body, "  ")
	if minHeight > 0 {
		return renderDialogPlainBlock(width, max(minHeight, 1), body)
	}
	return renderDialogPlainBlock(width, max(lipgloss.Height(body), 1), body)
}

func renderPaletteDialogHint(th *theme.Theme, width int, hint string) string {
	return renderDialogFooterHint(th, width, hint)
}

func indentLines(text, prefix string) string {
	if text == "" || prefix == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for idx, line := range lines {
		lines[idx] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func dialogLineTone(th *theme.Theme) string {
	if th != nil {
		if resolved := th.ToneToken("line"); resolved != "" {
			return resolved
		}
	}
	return colorFor(th, "overlay", lineColor)
}

func dialogBorderTone(th *theme.Theme) string {
	if th != nil {
		if resolved := th.ToneToken("line-strong"); resolved != "" {
			return resolved
		}
	}
	return dialogLineTone(th)
}

func applyDialogInputTheme(input *textinput.Model, th *theme.Theme) {
	textFG := lipgloss.Color(colorFor(th, "text", "#c0caf5"))
	softFG := lipgloss.Color(colorFor(th, "subtext", "#a9b1d6"))
	placeholderFG := lipgloss.Color(colorFor(th, "soft", "#565f89"))
	promptFG := lipgloss.Color(colorFor(th, "primary", "#7aa2f7"))

	styles := input.Styles()
	styles.Focused.Text = lipgloss.NewStyle().Foreground(textFG)
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(placeholderFG)
	styles.Focused.Suggestion = lipgloss.NewStyle().Foreground(softFG)
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(promptFG)

	styles.Blurred.Text = lipgloss.NewStyle().Foreground(textFG)
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(placeholderFG)
	styles.Blurred.Suggestion = lipgloss.NewStyle().Foreground(softFG)
	styles.Blurred.Prompt = lipgloss.NewStyle().Foreground(promptFG)
	styles.Cursor.Color = promptFG
	input.SetStyles(styles)
}

func closeDialog(id string, result any) tea.Cmd {
	return func() tea.Msg {
		return dialogClosedMsg{id: id, result: result}
	}
}

func desiredDialogWidth(frameWidth, minWidth, naturalWidth int) int {
	if frameWidth <= 0 {
		return max(minWidth, naturalWidth)
	}
	maxWidth := max(frameWidth-6, minWidth)
	return min(max(naturalWidth, minWidth), maxWidth)
}

func ansiWidth(value string) int {
	return ansi.StringWidth(value)
}

func blockWidth(value string) int {
	width := 0
	for _, line := range strings.Split(strings.TrimRight(value, "\n"), "\n") {
		width = max(width, ansiWidth(line))
	}
	return width
}

func renderDialogOnSurface(base string, dialog dialogModel, area dialogRenderArea, width, height int) (string, *tea.Cursor) {
	width = max(width, 1)
	height = max(height, 1)

	baseBuffer, baseRows := cachedSurfaceBase(base, width, height)
	surface := newOverlaySurface(baseBuffer, baseRows)
	cursor := renderDialogOnBuffer(surface, dialog, area)
	return renderDialogSurface(surface), cursor
}

func renderDialogOnBuffer(surface dialogSurface, dialog dialogModel, area dialogRenderArea) *tea.Cursor {
	if surface == nil || dialog == nil {
		return nil
	}

	width := max(surface.Width(), 1)
	height := max(surface.Height(), 1)
	area = clampDialogRenderArea(area, width, height)
	dialog.SetFrame(area.width, area.height)
	return dialog.Draw(surface, area)
}

func clampDialogRenderArea(area dialogRenderArea, width, height int) dialogRenderArea {
	width = max(width, 1)
	height = max(height, 1)
	if area.width <= 0 {
		area.width = width
	}
	if area.height <= 0 {
		area.height = height
	}
	area.x = min(max(area.x, 0), width-1)
	area.y = min(max(area.y, 0), height-1)
	area.width = min(max(area.width, 1), width-area.x)
	area.height = min(max(area.height, 1), height-area.y)
	return area
}

func drawDialogFrameOnSurface(surface dialogSurface, area dialogRenderArea, th *theme.Theme, width int, content string, cursor *tea.Cursor) *tea.Cursor {
	return drawDialogFrameOnSurfaceWithTone(surface, area, th, width, content, cursor, "")
}

func drawDialogFrameOnSurfaceWithTone(surface dialogSurface, area dialogRenderArea, th *theme.Theme, width int, content string, cursor *tea.Cursor, fillTone string) *tea.Cursor {
	if surface == nil {
		return nil
	}
	horizontalInset := dialogFrameInset
	width = max(width, 1)
	contentHeight := max(lipgloss.Height(content), 1)
	boxWidth := width
	boxHeight := contentHeight + dialogFrameInset*2
	area = clampDialogRenderArea(area, surface.Width(), surface.Height())
	x := area.x + max((max(area.width, 1)-boxWidth)/2, 0)
	y := area.y + max((max(area.height, 1)-boxHeight)/2, 0)

	clearDialogFrameArea(surface, x, y, boxWidth, boxHeight)
	fillDialogFrameArea(surface, th, x, y, boxWidth, boxHeight, fillTone)
	drawDialogFrameBorder(surface, th, x, y, boxWidth, boxHeight)
	drawDialogBlockOnSurface(surface, content, x+horizontalInset, y+dialogFrameInset)
	if cursor == nil {
		return nil
	}
	result := *cursor
	result.X += x + horizontalInset
	result.Y += y + dialogFrameInset
	return &result
}

func fillDialogFrameArea(surface dialogSurface, th *theme.Theme, x, y, width, height int, tone string) {
	if surface == nil || width <= 0 || height <= 0 {
		return
	}
	bg := toneValue(th, tone)
	if strings.TrimSpace(bg) == "" {
		return
	}
	drawBlockOnSurface(surface, placeBlock(width, height, bg, ""), x, y)
}

func drawDialogFrameBorder(surface dialogSurface, th *theme.Theme, x, y, width, height int) {
	if surface == nil || width <= 0 || height <= 0 {
		return
	}
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(dialogBorderTone(th)))
	switch {
	case width == 1:
		side := border.Render("│")
		for row := y; row < y+height; row++ {
			drawDialogBlockOnSurface(surface, side, x, row)
		}
		return
	case height == 1:
		top := border.Render("┌" + strings.Repeat("─", max(width-2, 0)) + "┐")
		drawDialogBlockOnSurface(surface, top, x, y)
		return
	}
	top := border.Render("┌" + strings.Repeat("─", max(width-2, 0)) + "┐")
	bottom := border.Render("└" + strings.Repeat("─", max(width-2, 0)) + "┘")
	drawDialogBlockOnSurface(surface, top, x, y)
	drawDialogBlockOnSurface(surface, bottom, x, y+height-1)
	sideStyle := border.Render("│")
	for row := y + 1; row < y+height-1; row++ {
		drawDialogBlockOnSurface(surface, sideStyle, x, row)
		drawDialogBlockOnSurface(surface, sideStyle, x+width-1, row)
	}
}

func clearDialogFrameArea(surface dialogSurface, x, y, width, height int) {
	if surface == nil || width <= 0 || height <= 0 {
		return
	}
	for row := 0; row < height; row++ {
		targetY := y + row
		if targetY < 0 || targetY >= surface.Height() {
			continue
		}
		for col := 0; col < width; col++ {
			targetX := x + col
			if targetX < 0 || targetX >= surface.Width() {
				continue
			}
			_ = surface.SetCell(targetX, targetY, &cellbuf.BlankCell)
		}
	}
}

func drawBlockOnSurface(surface dialogSurface, overlay string, x, y int) {
	drawSurfaceBlockWithMode(surface, overlay, x, y, false)
}

func drawDialogBlockOnSurface(surface dialogSurface, overlay string, x, y int) {
	drawSurfaceBlockWithMode(surface, overlay, x, y, true)
}

func drawBlockBufferWithMode(surface dialogSurface, overlay *cellbuf.Buffer, x, y int, transparentBlanks bool) {
	if surface == nil || overlay == nil {
		return
	}
	width := max(surface.Width(), 1)
	height := max(surface.Height(), 1)
	for row := 0; row < overlay.Height(); row++ {
		targetY := y + row
		if targetY < 0 || targetY >= height {
			continue
		}
		line := overlay.Line(row)
		if line == nil {
			continue
		}
		for col, cell := range line {
			targetX := x + col
			if targetX < 0 || targetX >= width {
				continue
			}
			if cell == nil || cell.Width == 0 {
				continue
			}
			if transparentBlanks && cell.Equal(&cellbuf.BlankCell) {
				continue
			}
			_ = surface.SetCell(targetX, targetY, compositeSurfaceCell(surface, targetX, targetY, cell))
		}
	}
}

func compositeSurfaceCell(surface dialogSurface, x, y int, overlay *cellbuf.Cell) *cellbuf.Cell {
	if surface == nil || overlay == nil {
		return overlay
	}
	if overlay.Style.Bg != nil {
		return overlay
	}

	existing := surface.Cell(x, y)
	if existing == nil || existing.Style.Bg == nil {
		return overlay
	}

	composited := overlay.Clone()
	composited.Style.Bg = existing.Style.Bg
	return composited
}

func renderCellBuffer(buf *cellbuf.Buffer) string {
	if buf == nil || buf.Height() <= 0 || buf.Width() <= 0 {
		return ""
	}
	return strings.Join(renderCellBufferRows(buf), "\n")
}

func renderCellBufferRows(buf *cellbuf.Buffer) []string {
	if buf == nil || buf.Height() <= 0 || buf.Width() <= 0 {
		return nil
	}
	lines := make([]string, 0, buf.Height())
	for row := 0; row < buf.Height(); row++ {
		lines = append(lines, renderCellBufferLine(buf.Line(row), buf.Width()))
	}
	return lines
}

func renderDialogSurface(surface dialogSurface) string {
	if surface == nil || surface.Height() <= 0 || surface.Width() <= 0 {
		return ""
	}
	if overlay, ok := surface.(*overlaySurface); ok {
		return renderOverlaySurface(overlay)
	}
	lines := make([]string, 0, surface.Height())
	for row := 0; row < surface.Height(); row++ {
		lines = append(lines, renderDialogSurfaceLine(surface, row, surface.Width()))
	}
	return strings.Join(lines, "\n")
}

func renderOverlaySurface(surface *overlaySurface) string {
	if surface == nil || surface.Height() <= 0 || surface.Width() <= 0 {
		return ""
	}
	lines := make([]string, 0, surface.Height())
	for row := 0; row < surface.Height(); row++ {
		if !surface.rowDirty(row) {
			if baseRow, ok := surface.baseRow(row); ok {
				lines = append(lines, baseRow)
				continue
			}
		}
		lines = append(lines, renderDialogSurfaceLine(surface, row, surface.Width()))
	}
	return strings.Join(lines, "\n")
}

func renderDialogSurfaceLine(surface dialogSurface, row, width int) string {
	return renderSurfaceLine(width, func(col int) *cellbuf.Cell {
		if surface == nil {
			return nil
		}
		return surface.Cell(col, row)
	})
}

func renderCellBufferLine(line cellbuf.Line, width int) string {
	return renderSurfaceLine(width, func(col int) *cellbuf.Cell {
		if line == nil {
			return nil
		}
		return line.At(col)
	})
}

func renderSurfaceLine(width int, cellAt func(col int) *cellbuf.Cell) string {
	if width <= 0 {
		return ""
	}
	if cellAt == nil {
		return strings.Repeat(" ", width)
	}

	var (
		buf     strings.Builder
		pending strings.Builder
		pen     cellbuf.Style
		link    cellbuf.Link
	)

	writePending := func() {
		if pending.Len() == 0 {
			return
		}
		buf.WriteString(pending.String())
		pending.Reset()
	}

	for col := 0; col < width; col++ {
		cell := cellAt(col)
		if cell == nil || cell.Width == 0 {
			continue
		}
		if cell.Style.Empty() && !pen.Empty() {
			writePending()
			buf.WriteString(ansi.ResetStyle)
			pen.Reset()
		}
		if !cell.Style.Equal(&pen) {
			writePending()
			buf.WriteString(cell.Style.DiffSequence(pen))
			pen = cell.Style
		}
		if cell.Link != link && link.URL != "" {
			writePending()
			buf.WriteString(ansi.ResetHyperlink())
			link.Reset()
		}
		if cell.Link != link {
			writePending()
			buf.WriteString(ansi.SetHyperlink(cell.Link.URL, cell.Link.Params))
			link = cell.Link
		}
		if cell.Equal(&cellbuf.BlankCell) {
			pending.WriteString(cell.String())
			continue
		}
		writePending()
		buf.WriteString(cell.String())
	}

	writePending()
	if link.URL != "" {
		buf.WriteString(ansi.ResetHyperlink())
	}
	if !pen.Empty() {
		buf.WriteString(ansi.ResetStyle)
	}
	return buf.String()
}

func fuzzyScore(pattern, text string) (bool, int) {
	if pattern == "" {
		return true, 0
	}
	lp := strings.ToLower(pattern)
	lt := strings.ToLower(text)
	rp := []rune(lp)
	rt := []rune(lt)
	origRunes := []rune(text)

	pi := 0
	score := 0
	lastMatch := -1
	for ti := 0; ti < len(rt) && pi < len(rp); ti++ {
		if rt[ti] != rp[pi] {
			continue
		}
		if ti == lastMatch+1 {
			score += 8
		}
		if ti == 0 || rt[ti-1] == '/' || rt[ti-1] == '.' || rt[ti-1] == '-' || rt[ti-1] == '_' || rt[ti-1] == ' ' || rt[ti-1] == '(' {
			score += 6
		}
		if ti > 0 && unicode.IsUpper(origRunes[ti]) && unicode.IsLower(origRunes[ti-1]) {
			score += 4
		}
		if ti == pi {
			score += 3
		}
		score += 2
		lastMatch = ti
		pi++
	}
	if pi < len(rp) {
		return false, 0
	}
	if strings.Contains(lt, lp) {
		score += 100
	}
	return true, score
}
