package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type Messages struct {
	vp       viewport.Model
	width    int
	height   int
	raw      string
	virtual  *messagesVirtualContent
	version  int64
	bgTone   string
	softWrap bool
	lockX    bool

	rawLinesCache *messagesRawLinesCache
	viewCache     *messagesViewCache
}

type messagesRawLinesCache struct {
	version int64
	valid   bool
	lines   []string
}

type messagesViewState struct {
	version  int64
	yOffset  int
	xOffset  int
	width    int
	height   int
	softWrap bool
}

type messagesViewCache struct {
	state    messagesViewState
	valid    bool
	rendered string
	lines    []string
	linesSet bool
}

type messagesVirtualChunk struct {
	content    string
	blankLines int
}

type messagesVirtualContent struct {
	chunks     []messagesVirtualChunk
	startLines []int
	totalLines int
}

func newMessagesVirtualContent(chunks []messagesVirtualChunk) *messagesVirtualContent {
	out := &messagesVirtualContent{
		chunks:     make([]messagesVirtualChunk, 0, len(chunks)),
		startLines: make([]int, 0, len(chunks)),
	}
	line := 0
	for _, chunk := range chunks {
		content := strings.TrimRight(chunk.content, "\n")
		lineCount := virtualContentLineCount(content)
		if chunk.blankLines > 0 {
			content = ""
			lineCount = chunk.blankLines
		}
		if lineCount <= 0 {
			continue
		}
		out.startLines = append(out.startLines, line)
		out.chunks = append(out.chunks, messagesVirtualChunk{content: content, blankLines: chunk.blankLines})
		line += lineCount
	}
	out.totalLines = line
	return out
}

func (v *messagesVirtualContent) equal(other *messagesVirtualContent) bool {
	if v == nil || other == nil {
		return v == other
	}
	if v.totalLines != other.totalLines || len(v.chunks) != len(other.chunks) {
		return false
	}
	for i := range v.chunks {
		if v.chunks[i].content != other.chunks[i].content || v.chunks[i].blankLines != other.chunks[i].blankLines {
			return false
		}
	}
	return true
}

func (v *messagesVirtualContent) placeholderContent() string {
	if v == nil || v.totalLines <= 0 {
		return ""
	}
	return strings.Repeat("\n", max(v.totalLines-1, 0))
}

func (v *messagesVirtualContent) allLines() []string {
	if v == nil || v.totalLines <= 0 {
		return nil
	}
	return v.visibleLines(0, v.totalLines)
}

func (v *messagesVirtualContent) visibleLines(offset, height int) []string {
	if v == nil || height <= 0 {
		return nil
	}
	offset = max(offset, 0)
	end := min(offset+height, max(v.totalLines, 0))
	lines := make([]string, 0, max(end-offset, 0))
	for i, chunk := range v.chunks {
		start := v.startLines[i]
		chunkLineCount := chunk.blankLines
		if chunkLineCount <= 0 {
			chunkLineCount = virtualContentLineCount(chunk.content)
		}
		chunkEnd := start + chunkLineCount
		if chunkEnd <= offset {
			continue
		}
		if start >= end {
			break
		}
		from := max(offset-start, 0)
		to := min(end-start, chunkLineCount)
		if from < to {
			if chunk.blankLines > 0 {
				for i := 0; i < to-from; i++ {
					lines = append(lines, "")
				}
			} else {
				chunkLines := strings.Split(chunk.content, "\n")
				lines = append(lines, chunkLines[from:to]...)
			}
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func virtualContentLineCount(content string) int {
	if strings.TrimSpace(content) == "" {
		return 0
	}
	return strings.Count(strings.TrimRight(content, "\n"), "\n") + 1
}

func NewMessages(th *theme.Theme) Messages {
	msgs := NewMessagesWithTone(th, "bg-alt")
	msgs.softWrap = false
	msgs.lockX = true
	msgs.vp.SoftWrap = false
	msgs.vp.SetHorizontalStep(0)
	msgs.enforceHorizontalLock()
	return msgs
}

func NewMessagesWithTone(th *theme.Theme, bgTone string) Messages {
	vp := viewport.New()
	vp.SoftWrap = true
	vp.FillHeight = false
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 1
	vp.KeyMap = viewport.KeyMap{}

	msgs := Messages{
		vp:            vp,
		bgTone:        strings.TrimSpace(bgTone),
		softWrap:      true,
		rawLinesCache: &messagesRawLinesCache{},
		viewCache:     &messagesViewCache{},
	}
	msgs.ApplyTheme(th)
	return msgs
}

func (m *Messages) ApplyTheme(_ *theme.Theme) {}

func (m *Messages) SetSoftWrap(enabled bool) {
	followBottom := m.vp.AtBottom()
	m.softWrap = enabled
	m.vp.SoftWrap = enabled
	m.syncViewportContent(followBottom)
}

func (m *Messages) SetSize(width, height int) {
	followBottom := m.vp.AtBottom()
	m.width = max(width, 1)
	m.height = max(height, 1)
	m.vp.SetWidth(m.width)
	m.vp.SetHeight(m.height)
	m.syncViewportContent(followBottom)
}

func (m *Messages) Sync(content string, follow bool) {
	wasVirtual := m.virtual != nil
	m.virtual = nil
	if content == m.raw && !wasVirtual {
		if follow && !m.vp.AtBottom() {
			m.vp.GotoBottom()
		}
		return
	}
	m.raw = content
	m.version++
	m.syncViewportContent(follow)
}

func (m *Messages) SyncVirtualChunks(chunks []messagesVirtualChunk, follow bool) {
	virtual := newMessagesVirtualContent(chunks)
	if m.virtual != nil && m.virtual.equal(virtual) {
		if follow && !m.vp.AtBottom() {
			m.vp.GotoBottom()
		}
		return
	}
	m.raw = ""
	m.virtual = virtual
	m.version++
	m.syncViewportContent(follow)
}

func (m *Messages) Update(msg tea.Msg) tea.Cmd {
	updated, cmd := m.vp.Update(msg)
	m.vp = updated
	m.enforceHorizontalLock()
	return cmd
}

func (m Messages) View() string {
	return m.ensureRenderedViewState().rendered
}

func (m Messages) VisibleLines() []string {
	view := m.ensureRenderedViewState()
	if view.linesSet {
		return view.lines
	}
	lines := strings.Split(view.rendered, "\n")
	if m.viewCache != nil && m.viewCache.valid && m.viewCache.state == view.state {
		m.viewCache.lines = lines
		m.viewCache.linesSet = true
		return m.viewCache.lines
	}
	return lines
}

func (m Messages) RawLines() []string {
	if m.virtual != nil {
		return m.virtual.allLines()
	}
	if m.rawLinesCache != nil && m.rawLinesCache.valid && m.rawLinesCache.version == m.version {
		return m.rawLinesCache.lines
	}
	content := strings.TrimRight(m.raw, "\n")
	var lines []string
	if strings.TrimSpace(content) != "" {
		lines = strings.Split(content, "\n")
	}
	if m.rawLinesCache != nil {
		m.rawLinesCache.version = m.version
		m.rawLinesCache.valid = true
		m.rawLinesCache.lines = lines
	}
	return lines
}

func (m *Messages) ScrollUp(lines int) {
	m.vp.ScrollUp(lines)
}

func (m *Messages) ScrollDown(lines int) {
	m.vp.ScrollDown(lines)
}

func (m *Messages) PageUp() {
	m.vp.ScrollUp(max(m.height-1, 1))
}

func (m *Messages) PageDown() {
	m.vp.ScrollDown(max(m.height-1, 1))
}

func (m *Messages) GotoTop() {
	m.vp.GotoTop()
}

func (m *Messages) GotoBottom() {
	m.vp.GotoBottom()
}

func (m *Messages) GotoLine(line int) {
	m.vp.SetYOffset(max(line, 0))
}

func (m Messages) AtBottom() bool {
	return m.vp.AtBottom()
}

func (m Messages) YOffset() int {
	return m.vp.YOffset()
}

func (m Messages) Width() int {
	return max(m.vp.Width(), 1)
}

func (m Messages) Height() int {
	return m.vp.Height()
}

func (m Messages) ContentVersion() int64 {
	return m.version
}

func (m Messages) TotalLineCount() int {
	if m.virtual != nil {
		return m.virtual.totalLines
	}
	return m.vp.TotalLineCount()
}

func (m Messages) ScrollSummary() string {
	total := m.vp.TotalLineCount()
	height := m.vp.Height()
	if total <= height || height <= 0 {
		return ""
	}
	denom := max(total-height, 1)
	pct := (m.vp.YOffset() * 100) / denom
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("↑ ▓▒░ %d%%", pct)
}

func (m *Messages) syncViewportContent(follow bool) {
	atBottom := m.vp.AtBottom()
	m.vp.SoftWrap = m.softWrap
	m.vp.SetWidth(max(m.width, 1))
	m.vp.SetHeight(max(m.height, 1))
	if m.virtual != nil {
		m.vp.SetContent(m.virtual.placeholderContent())
	} else {
		m.vp.SetContent(m.raw)
	}
	m.enforceHorizontalLock()
	if follow || atBottom {
		m.vp.GotoBottom()
	}
}

func (m *Messages) enforceHorizontalLock() {
	if m == nil || !m.lockX {
		return
	}
	m.vp.SetHorizontalStep(0)
	m.vp.SetXOffset(0)
}

func (m Messages) ensureRenderedViewState() messagesViewCache {
	state := messagesViewState{
		version:  m.version,
		yOffset:  m.vp.YOffset(),
		xOffset:  m.vp.XOffset(),
		width:    m.vp.Width(),
		height:   m.vp.Height(),
		softWrap: m.softWrap,
	}
	if m.viewCache != nil && m.viewCache.valid && m.viewCache.state == state {
		return *m.viewCache
	}

	rendered := m.vp.View()
	linesSet := false
	var lines []string
	if m.virtual != nil {
		lines = m.virtual.visibleLines(m.vp.YOffset(), max(m.vp.Height(), 1))
		rendered = strings.Join(lines, "\n")
		linesSet = true
	}
	if m.viewCache != nil {
		m.viewCache.state = state
		m.viewCache.valid = true
		m.viewCache.rendered = rendered
		m.viewCache.lines = lines
		m.viewCache.linesSet = linesSet
		return *m.viewCache
	}
	return messagesViewCache{
		state:    state,
		valid:    true,
		rendered: rendered,
		lines:    lines,
		linesSet: linesSet,
	}
}

func (m Messages) surfaceTone() string {
	switch m.bgTone {
	case "none":
		return "none"
	case tonePanel:
		return tonePanel
	case tonePanelAlt:
		return tonePanelAlt
	default:
		return toneBGAlt
	}
}
