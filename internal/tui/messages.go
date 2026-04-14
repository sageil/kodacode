package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/logging"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// SubagentActivity tracks a child tool call within a running subagent.
type SubagentActivity struct {
	Tool      string
	Input     string
	Summary   string
	Args      string
	Output    string
	StartTime time.Time
	Elapsed   time.Duration
	Done      bool
	Error     bool
	Expanded  bool
}

type Message struct {
	Role         string // "user" | "assistant" | "system" | "tool_call"
	Content      string
	Streaming    bool
	Timestamp    time.Time
	Collapsed    bool
	UserExpanded bool

	ToolCallID         string
	ToolName           string
	ToolInput          string
	ToolOutput         string
	ToolError          string
	ToolDone           bool
	ToolStartTime      time.Time
	ToolEndTime        time.Time
	ToolElapsed        time.Duration
	SubagentActivities []SubagentActivity
}

// toolRegion maps a range of rendered lines to a message index for click handling.
type toolRegion struct {
	startLine int // inclusive, in content coordinates
	endLine   int // exclusive
	msgIndex  int // index into Messages.messages
}

type subagentActivityRegion struct {
	line          int
	msgIndex      int
	activityIndex int
}

// hunkRegion maps a single rendered line to a hunk header or action bar for click handling.
type hunkRegion struct {
	line      int // absolute content-line
	msgIndex  int
	hunkIndex int // index into diffReview.hunks; -1 = action bar
}

// Messages is the scrollable chat viewport.
type Messages struct {
	vp                      viewport.Model
	messages                []Message
	mdWidth                 int
	width                   int
	height                  int
	screenY                 int // vertical offset of this viewport on screen (for mouse hit-testing)
	theme                   *theme.Theme
	themeName               string                   // bare name of the active theme, for syntax highlighting
	autoScroll              bool                     // track if we should auto-scroll to bottom
	userScrolled            bool                     // true when user has manually scrolled away from bottom; suppresses autoScroll
	focusedTool             int                      // index into messages of the focused tool_call block; -1 = none
	toolRegions             []toolRegion             // click regions for tool blocks, rebuilt on render
	subagentActivityRegions []subagentActivityRegion // click regions for subagent activities, rebuilt on render
	hunkRegions             []hunkRegion             // click regions for diff hunk headers/action bar, rebuilt on render
	diffReviews             map[int]*diffReview      // msgIndex → per-hunk acceptance state
	diffReviewMetas         map[int]*diffReviewMeta  // msgIndex → persistent hunk region offsets
	pendingMetas            []diffReviewMeta         // collected during renderMessageAt, consumed in render()

	renderCache []string
	dirtyFrom   int
	mdCache     *lruCache
	hlCache     *lruCache

	styles     *toolStyles
	codeBgAnsi string

	searchQuery  string
	searchActive bool

	needsRender bool
	lastRender  time.Time
}

func (m *Messages) SetThemeName(name string) {
	m.themeName = name
}

func (m *Messages) ApplyTheme(t *theme.Theme) {
	m.theme = t
	m.styles = nil // rebuild styles from new theme
	m.codeBgAnsi = resolveCodeBgAnsi(t)
	m.invalidateFrom(0)
	m.hlCache = nil // theme change invalidates syntax highlighting
	m.mdCache = nil // theme change invalidates markdown rendering
	m.needsRender = true
}

// resolveCodeBgAnsi returns an ANSI background escape from the theme's "surface" color.
func resolveCodeBgAnsi(t *theme.Theme) string {
	c := colorFrom(t, "surface", lipgloss.Color("236"))
	styled := lipgloss.NewStyle().Background(c).Render(" ")
	if before, _, ok := strings.Cut(styled, " "); ok {
		return before
	}
	return ansiCodeBg // fallback
}

func NewMessages() Messages {
	vp := viewport.New()
	vp.SoftWrap = true
	vp.MouseWheelEnabled = true
	vp.KeyMap = viewport.KeyMap{}
	return Messages{vp: vp, focusedTool: -1, diffReviews: make(map[int]*diffReview), diffReviewMetas: make(map[int]*diffReviewMeta)}
}

func (m *Messages) SetSize(w, h int) {
	widthChanged := m.width != w
	m.width = w
	m.height = h
	m.mdWidth = w - 4 // 4 cols for left indent
	scrollbarWidth := 3
	m.vp.SetWidth(w - scrollbarWidth)
	m.vp.SetHeight(h)
	if widthChanged {
		m.invalidateFrom(0) // width change affects all rendered output
		m.needsRender = true
	}
}

const (
	ansiBold          = "\x1b[1m"
	ansiItalic        = "\x1b[3m"
	ansiReset         = "\x1b[0m"
	ansiDim           = "\x1b[2m"
	ansiCodeBg        = "\x1b[48;5;236m"   // dark gray background for inline code
	ansiHeadingClr    = "\x1b[1;38;5;75m"  // bold + blue for headings
	ansiLinkClr       = "\x1b[4;38;5;75m"  // underline + blue for links
	ansiBqClr         = "\x1b[38;5;241m"   // gray for blockquotes
	ansiBqBar         = "\x1b[38;5;241m▎ " // left bar for blockquotes
	ansiStrikethrough = "\x1b[9m"
	ansiGreen         = "\x1b[32m"
	ansiRed           = "\x1b[31m"
	ansiCyan          = "\x1b[36m"
	ansiReverse       = "\x1b[7m"
)

// maxStreamingLines caps streamed tool output in the TUI to the last N lines.
// The final Result.Output replaces it when the tool completes.
const maxStreamingLines = 15

var tableAvailWidth = 80

var pulseTick int64

func AdvanceSpinner() { pulseTick++ }

func (m *Messages) invalidateFrom(idx int) {
	if idx < m.dirtyFrom {
		m.dirtyFrom = idx
	}
}

func (m *Messages) invalidateRunningTools() {
	earliest := -1
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.Role == "tool_call" && !msg.ToolDone {
			earliest = i
		}
		if msg.Role == "tool_call" && msg.ToolDone && !msg.UserExpanded &&
			isReadOnlyToolCall(msg.ToolName, msg.ToolInput) &&
			!msg.ToolEndTime.IsZero() && time.Since(msg.ToolEndTime) <= readOnlyHideDelay+time.Second {
			if earliest < 0 || i < earliest {
				earliest = i
			}
		}
	}
	if earliest >= 0 {
		m.invalidateFrom(earliest)
		m.needsRender = true
	}
}

func (m *Messages) render() {
	dirtyFrom := m.dirtyFrom

	for len(m.renderCache) < len(m.messages) {
		m.renderCache = append(m.renderCache, "")
	}
	if len(m.renderCache) > len(m.messages) {
		m.renderCache = m.renderCache[:len(m.messages)]
	}

	for i := dirtyFrom; i < len(m.messages); i++ {
		m.renderCache[i] = m.renderMessageAt(i, m.messages[i])
	}
	m.dirtyFrom = len(m.messages) // all clean now

	var sb strings.Builder
	m.toolRegions = m.toolRegions[:0]
	m.subagentActivityRegions = m.subagentActivityRegions[:0]
	m.hunkRegions = m.hunkRegions[:0]
	m.pendingMetas = m.pendingMetas[:0]
	lineCount := 0
	wroteFirst := false
	prevRole := ""
	for i := range len(m.messages) {
		if m.messages[i].Role == "assistant" && strings.TrimSpace(m.messages[i].Content) == "" {
			logging.Debugf("[8-render] SKIP empty assistant msg[%d]", i)
			continue
		}

		if wroteFirst {
			isToolCall := m.messages[i].Role == "tool_call"
			prevIsToolCall := prevRole == "tool_call"
			if !isToolCall && !prevIsToolCall {
				sb.WriteString("\n")
				lineCount++
			}
		}
		wroteFirst = true
		prevRole = m.messages[i].Role

		startLine := lineCount
		sb.WriteString(m.renderCache[i])
		lineCount += strings.Count(m.renderCache[i], "\n")

		if m.messages[i].Role == "tool_call" && lineCount > startLine {
			m.toolRegions = append(m.toolRegions, toolRegion{
				startLine: startLine,
				endLine:   lineCount,
				msgIndex:  i,
			})
			panelContentStart := startLine + 2 // header + newline before body
			var pm *diffReviewMeta
			for mi := range m.pendingMetas {
				if m.pendingMetas[mi].msgIndex == i {
					pm = &m.pendingMetas[mi]
					break
				}
			}
			if pm == nil {
				pm = m.diffReviewMetas[i]
			}
			if pm != nil {
				for hi, off := range pm.hunkOffsets {
					m.hunkRegions = append(m.hunkRegions, hunkRegion{
						line:      panelContentStart + off,
						msgIndex:  i,
						hunkIndex: hi,
					})
				}
				if pm.actionOffset >= 0 {
					m.hunkRegions = append(m.hunkRegions, hunkRegion{
						line:      panelContentStart + pm.actionOffset,
						msgIndex:  i,
						hunkIndex: -1, // action bar
					})
				}
			}
			if m.messages[i].ToolName == "subagent" {
				m.buildSubagentActivityRegions(i, startLine)
			}
		}

	}
	content := sb.String()

	yOffset := m.vp.YOffset()

	m.vp.SetContent(content)

	if m.autoScroll && !m.userScrolled {
		m.vp.GotoBottom()
		m.autoScroll = false
	} else {
		m.vp.SetYOffset(yOffset)
	}
}

// renderThrottle is the minimum interval between renders while streaming.
// This prevents per-delta re-renders from causing visible flickering.
const renderThrottle = 66 * time.Millisecond // ~15 fps

// ToggleAllCollapsed flips all tool call messages between collapsed and expanded.
// It detects the majority state and flips to the opposite.
func (m *Messages) ToggleAllCollapsed() {
	collapsed := 0
	total := 0
	for i := range m.messages {
		if m.messages[i].Role == "tool_call" {
			total++
			if m.messages[i].Collapsed {
				collapsed++
			}
		}
	}
	if total == 0 {
		return
	}
	// If majority collapsed, expand all; otherwise collapse all.
	target := collapsed > total/2
	for i := range m.messages {
		if m.messages[i].Role == "tool_call" {
			m.messages[i].Collapsed = !target
			m.messages[i].UserExpanded = target
		}
	}
	m.invalidateFrom(0)
	m.needsRender = true
}

// FlushRender performs a deferred render if needed. Call this once per update
// cycle (from Session.Update) to ensure the viewport is up-to-date before View().
func (m *Messages) FlushRender() {
	if !m.needsRender {
		return
	}
	if m.isStreaming() && time.Since(m.lastRender) < renderThrottle {
		return // keep needsRender=true; next flush will pick it up
	}
	m.needsRender = false
	m.lastRender = time.Now()
	m.render()
}

func (m *Messages) ScrollToBottom() {
	m.userScrolled = false
	m.autoScroll = true
}

func (m *Messages) isStreaming() bool {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Streaming {
			return true
		}
		if m.messages[i].Role != "assistant" {
			break
		}
	}
	return false
}

func (m Messages) Init() tea.Cmd { return m.vp.Init() }

func (m Messages) Update(msg tea.Msg) (Messages, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if m.handleMouseClick(msg) {
			return m, nil
		}
	}
	var cmd tea.Cmd
	prevOffset := m.vp.YOffset()
	m.vp, cmd = m.vp.Update(msg)
	if m.vp.YOffset() != prevOffset {
		if m.vp.AtBottom() {
			m.userScrolled = false
		} else {
			m.userScrolled = true
		}
	}
	return m, cmd
}

func (m Messages) View() string {
	content := m.vp.View()
	totalW := m.width

	lines := strings.Split(content, "\n")
	hasScrollbar := m.vp.TotalLineCount() > m.vp.Height()

	var scrollbar []string
	if hasScrollbar {
		scrollbar = m.renderScrollbar()
	}

	var sb strings.Builder
	for i, line := range lines {
		if hasScrollbar && i < len(scrollbar) {
			line = line + "  " + scrollbar[i]
		}
		w := lipgloss.Width(line)
		if w < totalW {
			line = line + strings.Repeat(" ", totalW-w)
		}
		sb.WriteString(line + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

// ThemeChangedMsg is broadcast when the active theme changes.
// Name is the bare theme name ("rose-pine-moon", "default" for system palette,
// or "" when the change came from the file-watcher and no name is known).
type ThemeChangedMsg struct {
	Theme *theme.Theme
	Name  string
}
