package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type mouseWheelTarget int

const (
	mouseWheelTargetNone mouseWheelTarget = iota
	mouseWheelTargetTranscript
	mouseWheelTargetInspector
	mouseWheelTargetSessions
)

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.dialog != nil {
		return m, nil
	}
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	target := m.mouseTargetAt(msg.Mouse().X, msg.Mouse().Y)
	if target != mouseWheelTargetTranscript && (m.transcriptView.visualActive || m.transcriptView.mouseSelecting) {
		m.clearTranscriptMouseSelection()
		m.clearTranscriptVisualSelection()
	}
	switch target {
	case mouseWheelTargetTranscript:
		if m.transcriptView.visualActive {
			m.clearTranscriptVisualSelection()
		}
		if pos, ok := m.transcriptCursorPositionAtMouse(msg.Mouse().X, msg.Mouse().Y); ok {
			m.setTranscriptCursorPosition(pos)
			if m.transcriptView.cursorInitialized {
				m.transcriptView.mouseSelecting = true
				m.transcriptView.mouseAnchorLine = m.transcriptView.cursorLine
				m.transcriptView.mouseAnchorColumn = m.transcriptView.cursorColumn
			}
		}
		if m.chrome.focus != focusTranscript {
			m.chrome.focus = focusTranscript
			return m, m.syncComposerFocus()
		}
		return m, nil
	case mouseWheelTargetInspector:
		if !isWideShell(m) {
			if m.chrome.focus != focusInspector {
				m.chrome.focus = focusInspector
				return m, m.syncComposerFocus()
			}
			return m, nil
		}
	}
	if updated, cmd, handled := m.handleWideDrawerClick(msg); handled {
		return updated, cmd
	}
	if rect, ok := m.composerFocusMouseRect(); ok && rect.contains(msg.Mouse().X, msg.Mouse().Y) {
		return m.enterInsertMode()
	}
	return m, nil
}

func (m Model) handleMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.dialog != nil || !m.transcriptView.mouseSelecting {
		return m, nil
	}
	pos, ok := m.transcriptDragCursorPositionAtMouse(msg.Mouse().X, msg.Mouse().Y)
	if !ok {
		return m, nil
	}
	m.syncTranscriptMouseSelectionTo(pos)
	return m, nil
}

func (m Model) handleMouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if m.dialog != nil || !m.transcriptView.mouseSelecting {
		return m, nil
	}
	if pos, ok := m.transcriptDragCursorPositionAtMouse(msg.Mouse().X, msg.Mouse().Y); ok {
		m.syncTranscriptMouseSelectionTo(pos)
	}
	hasSelection := m.transcriptMouseHasSelection()
	if !hasSelection {
		m.clearTranscriptMouseSelection()
		m.clearTranscriptVisualSelection()
		return m, nil
	}
	cmd := m.copyTranscriptVisualSelectionCmd()
	m.clearTranscriptMouseSelection()
	m.clearTranscriptVisualSelection()
	return m, cmd
}

func (m *Model) syncTranscriptMouseSelectionTo(pos transcriptCursorPosition) {
	if m == nil || !m.transcriptView.mouseSelecting {
		return
	}
	anchor := m.transcriptMouseAnchorPosition()
	if compareTranscriptCursorPosition(anchor, pos) == 0 {
		m.setTranscriptCursorPosition(pos)
		m.transcriptView.visualActive = false
		return
	}
	m.transcriptView.visualActive = true
	m.transcriptView.visualAnchorLine = anchor.line
	m.transcriptView.visualAnchorColumn = anchor.column
	m.setTranscriptCursorPosition(pos)
}

func (m Model) transcriptMouseAnchorPosition() transcriptCursorPosition {
	return transcriptCursorPosition{
		line:   m.transcriptView.mouseAnchorLine,
		column: m.transcriptView.mouseAnchorColumn,
	}
}

func (m Model) transcriptMouseHasSelection() bool {
	if !m.transcriptView.mouseSelecting || !m.transcriptView.cursorInitialized {
		return false
	}
	current := transcriptCursorPosition{
		line:   m.transcriptView.cursorLine,
		column: m.transcriptView.cursorColumn,
	}
	return compareTranscriptCursorPosition(m.transcriptMouseAnchorPosition(), current) != 0
}

func (m Model) transcriptCursorPositionAtMouse(mouseX, mouseY int) (transcriptCursorPosition, bool) {
	rect, ok := m.transcriptViewportRect()
	if !ok || !rect.contains(mouseX, mouseY) {
		return transcriptCursorPosition{}, false
	}
	line := m.messages.YOffset() + (mouseY - rect.y)
	lineCount := m.transcriptLineCount()
	if lineCount <= 0 {
		return transcriptCursorPosition{}, false
	}
	line = clampTranscriptLineIndex(line, lineCount-1)
	if !m.transcriptLineSelectable(line) {
		if selectable, ok := m.nearestSelectableTranscriptLine(line, 1); ok {
			line = selectable
		} else if selectable, ok := m.nearestSelectableTranscriptLine(line, -1); ok {
			line = selectable
		} else {
			return transcriptCursorPosition{}, false
		}
	}
	visibleLine, ok := m.visibleTranscriptLine(line)
	if !ok {
		visibleLine = m.transcriptSelectionLineAt(line).text
	}
	rawColumn := transcriptRawColumnForDisplayX(ansi.Strip(visibleLine), mouseX-rect.x)
	selectionLine := m.transcriptSelectionLineAt(line)
	column := rawColumn - selectionLine.prefixGraphemes
	return transcriptCursorPosition{
		line:   line,
		column: clampTranscriptCursorColumn(column, selectionLine.graphemeCount),
	}, true
}

func (m *Model) transcriptDragCursorPositionAtMouse(mouseX, mouseY int) (transcriptCursorPosition, bool) {
	if m == nil {
		return transcriptCursorPosition{}, false
	}
	rect, ok := m.transcriptViewportRect()
	if !ok || rect.height <= 0 {
		return transcriptCursorPosition{}, false
	}
	lineCount := m.transcriptLineCount()
	if lineCount <= 0 {
		return transcriptCursorPosition{}, false
	}

	if mouseY < rect.y {
		m.messages.ScrollUp(rect.y - mouseY)
	} else if mouseY >= rect.y+rect.height {
		m.messages.ScrollDown(mouseY - (rect.y + rect.height) + 1)
	}

	localY := mouseY - rect.y
	if localY < 0 {
		localY = 0
	}
	if localY >= rect.height {
		localY = rect.height - 1
	}

	gutterWidth := transcriptScrollbarRenderWidth(transcriptPaneShowScrollbar(*m, true))
	localX := mouseX - rect.x
	if localX < 0 {
		localX = 0
	}
	maxLocalX := max(rect.width+gutterWidth-1, 0)
	if localX > maxLocalX {
		localX = maxLocalX
	}

	line := m.messages.YOffset() + localY
	line = clampTranscriptLineIndex(line, lineCount-1)
	if !m.transcriptLineSelectable(line) {
		if selectable, ok := m.nearestSelectableTranscriptLine(line, 1); ok {
			line = selectable
		} else if selectable, ok := m.nearestSelectableTranscriptLine(line, -1); ok {
			line = selectable
		} else {
			return transcriptCursorPosition{}, false
		}
	}
	visibleLine, ok := m.visibleTranscriptLine(line)
	if !ok {
		visibleLine = m.transcriptSelectionLineAt(line).text
	}
	rawColumn := transcriptRawColumnForDisplayX(ansi.Strip(visibleLine), localX)
	selectionLine := m.transcriptSelectionLineAt(line)
	column := rawColumn - selectionLine.prefixGraphemes
	return transcriptCursorPosition{
		line:   line,
		column: clampTranscriptCursorColumn(column, selectionLine.graphemeCount),
	}, true
}

func (m Model) transcriptViewportRect() (inputMouseRect, bool) {
	if m.width <= 0 || m.height <= 0 || m.messages.Height() <= 0 || m.messages.Width() <= 0 {
		return inputMouseRect{}, false
	}
	state := m.projector.Snapshot()
	viewport := viewportLayoutStateFor(m, state)
	rect := resolveTranscriptViewportGeometry(m, state, viewport.shell).viewportRect

	rect.width = max(m.messages.Width(), 1)
	rect.height = max(m.messages.Height(), 1)
	if strings.TrimSpace(m.messages.raw) == "" {
		rect.height = 0
	}
	return rect, rect.width > 0 && rect.height > 0
}

func (m Model) composerMouseRect() (inputMouseRect, bool) {
	if m.width <= 0 || m.height <= 0 {
		return inputMouseRect{}, false
	}

	state := m.projector.Snapshot()
	layout := resolveShellLayout(m, state)
	if isWideShell(m) {
		layout = normalizeWideShellLayout(m, state, layout)
	}
	composerFocus := resolveShellRects(m, state, layout).composerFocus
	width := max(composerFocus.width, 1)

	var composerHeight int
	if isWideShell(m) {
		composerHeight = lipgloss.Height(renderSplitComposerPane(m, state, width))
	} else {
		composerHeight = lipgloss.Height(renderComposerBar(m, state, width))
	}
	if composerHeight <= 0 {
		return inputMouseRect{}, false
	}

	rect := inputMouseRect{
		x:      0,
		y:      composerFocus.y,
		width:  width,
		height: composerHeight,
	}
	return rect, rect.width > 0 && rect.height > 0
}

func (m Model) composerFocusMouseRect() (inputMouseRect, bool) {
	if m.width <= 0 || m.height <= 0 {
		return inputMouseRect{}, false
	}

	state := m.projector.Snapshot()
	layout := resolveShellLayout(m, state)
	rect := resolveShellRects(m, state, layout).composerFocus
	return rect, rect.width > 0 && rect.height > 0
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	return m.handleMouseWheelSteps(msg, 1)
}

func (m Model) handleMouseWheelSteps(msg tea.MouseWheelMsg, steps int) (tea.Model, tea.Cmd) {
	steps = max(steps, 1)
	if m.dialog != nil {
		cmds := make([]tea.Cmd, 0, steps+1)
		for range steps {
			updated, cmd := m.dialog.Update(msg)
			m.dialog = updated
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, m.syncDeferredDialogIfNeeded())
		return m, tea.Batch(cmds...)
	}

	target := m.mouseWheelTarget(msg)
	switch target {
	case mouseWheelTargetInspector:
		cmds := make([]tea.Cmd, 0, steps)
		for range steps {
			updated, cmd := m.handleInspectorScroll(msg)
			m = updated.(Model)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case mouseWheelTargetSessions:
		cmds := make([]tea.Cmd, 0, steps)
		for range steps {
			cmds = append(cmds, m.sessionsBody.Update(msg))
		}
		return m, tea.Batch(cmds...)
	case mouseWheelTargetTranscript:
		var focusCmd tea.Cmd
		if m.chrome.focus != focusTranscript {
			m.chrome.focus = focusTranscript
			focusCmd = m.syncComposerFocus()
		}
		cmds := make([]tea.Cmd, 0, steps+1)
		for range steps {
			cmds = append(cmds, m.messages.Update(msg))
		}
		m.syncTranscriptCursorToViewport()
		m.syncDeferredTranscriptIfNeeded()
		cmds = append(cmds, focusCmd)
		return m, tea.Batch(cmds...)
	default:
		return m, nil
	}
}

func (m Model) mouseWheelTarget(msg tea.MouseWheelMsg) mouseWheelTarget {
	mouse := msg.Mouse()
	return m.mouseTargetAt(mouse.X, mouse.Y)
}

func (m Model) mouseTargetAt(mouseX, mouseY int) mouseWheelTarget {
	state := m.projector.Snapshot()
	layout := resolveShellLayout(m, state)
	return resolveShellRects(m, state, layout).mouseTargetAt(mouseX, mouseY)
}

func (m Model) handleWideDrawerClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd, bool) {
	if !isWideShell(m) {
		return m, nil, false
	}

	state := m.projector.Snapshot()
	layout := resolveShellLayout(m, state)
	rects := resolveShellRects(m, state, layout)
	layout = normalizeWideShellLayout(m, state, layout)
	if !layout.showInspector || layout.rightWidth <= 0 {
		return m, nil, false
	}

	mouse := msg.Mouse()
	if !rects.inspector.contains(mouse.X, mouse.Y) {
		return m, nil, false
	}
	localY := mouse.Y - rects.inspector.y

	m.chrome.wideSidebarOpen = true
	m.chrome.inspectorOpen = true
	m.chrome.focus = focusInspector

	tabsHeight := lipgloss.Height(renderSplitInspectorTabs(m, layout.rightWidth))
	localX := mouse.X - rects.inspector.x
	if localY < tabsHeight {
		if !m.hasPendingApproval() {
			if tab := splitInspectorTabIndexAt(layout.rightWidth, localX); tab >= 0 {
				if inspectorTabEnabled(m, tab) {
					m.inspector.tab = tab
					m.syncInspectorBody(true)
				}
			}
		}
		return m, tea.Batch(m.syncComposerFocus(), m.ensureRelevantDelegatedSessionSnapshotsLoadedCmd(m.projector.Snapshot())), true
	}

	if m.hasPendingApproval() {
		return m, tea.Batch(m.syncComposerFocus(), m.ensureRelevantDelegatedSessionSnapshotsLoadedCmd(m.projector.Snapshot())), true
	}

	bodyY := localY - tabsHeight
	if bodyY < 0 || bodyY >= m.inspector.body.height {
		return m, m.syncComposerFocus(), true
	}

	index := m.inspector.body.YOffset() + bodyY
	activeTab := effectiveInspectorTab(m)
	if activeTab == inspectorTabTools && len(m.inspector.toolLines) > 0 {
		action, ok := m.inspector.toolLines[index]
		if !ok {
			return m, tea.Batch(m.syncComposerFocus(), m.ensureRelevantDelegatedSessionSnapshotsLoadedCmd(m.projector.Snapshot())), true
		}
		switch action.Kind {
		case inspectorToolLineToggleGroup:
			m.toggleInspectorToolGroup(action.GroupID)
			m.syncInspectorBody(false)
			return m, tea.Batch(m.syncComposerFocus(), m.ensureRelevantDelegatedSessionSnapshotsLoadedCmd(m.projector.Snapshot())), true
		case inspectorToolLineOpenCall:
			return m, tea.Batch(m.syncComposerFocus(), m.selectInspectorToolTarget(action.Target), m.openInspectorToolTargetDialog(action.Target)), true
		case inspectorToolLineOpenHandoff:
			return m, tea.Batch(m.syncComposerFocus(), m.openHandoffDetailDialog(action.Handoff)), true
		default:
			return m, m.syncComposerFocus(), true
		}
	}
	if activeTab == inspectorTabTasks && len(m.inspector.taskLines) > 0 {
		taskID, ok := m.inspector.taskLines[index]
		if !ok {
			return m, m.syncComposerFocus(), true
		}
		m.selectTask(taskID)
		return m, tea.Batch(m.syncComposerFocus(), m.openTaskDialog(taskID)), true
	}
	if activeTab != inspectorTabTools {
		return m, m.syncComposerFocus(), true
	}
	refs := visibleToolSelectionRefs(m, state)
	if index < 0 || index >= len(refs) {
		return m, m.syncComposerFocus(), true
	}

	return m, tea.Batch(m.syncComposerFocus(), m.selectToolCall(refs[index]), m.openToolCallDialog(refs[index])), true
}
