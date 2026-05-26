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
	if content == m.raw {
		if follow && !m.vp.AtBottom() {
			m.vp.GotoBottom()
		}
		return
	}
	m.raw = content
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
	m.vp.SetContent(m.raw)
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
	if m.viewCache != nil {
		m.viewCache.state = state
		m.viewCache.valid = true
		m.viewCache.rendered = rendered
		m.viewCache.lines = nil
		m.viewCache.linesSet = false
		return *m.viewCache
	}
	return messagesViewCache{
		state:    state,
		valid:    true,
		rendered: rendered,
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
