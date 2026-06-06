package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

func (m *Model) setTranscriptMessagesSize(width, height int) {
	m.messages.SetSize(max(width, 1), max(height, 1))
}

func (m *Model) setSessionsBodySize(width, height int) {
	m.sessionsBody.SetSize(max(width, 1), max(height, 1))
}

func (m *Model) setInspectorBodySize(width, height int) {
	m.inspector.body.SetSize(max(width, 1), max(height, 1))
}

func (m *Model) syncSessionsBody() {
	state := m.projector.CurrentState()
	content := renderSessionsPanelBody(*m, state, m.sessionsBody.Width())
	m.sessionsBody.Sync(content, false)
	m.sessionsBody.GotoTop()
}

func (m *Model) syncInspectorBody(reset bool) {
	m.syncInspectorTabAvailability()
	state := m.projector.CurrentState()
	contentWidth := max(m.inspector.body.Width(), 1)
	key := inspectorViewportKey(*m, state)
	keyChanged := key != m.inspector.key
	followBottom := inspectorAutoFollowBottom(*m)
	forceFollowBottom := followBottom && (reset || keyChanged)
	content := m.renderInspectorViewportContent(state, contentWidth)
	m.inspector.body.Sync(content, forceFollowBottom)
	if reset || keyChanged {
		if followBottom {
			m.inspector.body.GotoBottom()
		} else {
			m.inspector.body.GotoTop()
		}
	}
	m.inspector.key = key
}

func (m *Model) renderInspectorViewportContent(state events.SessionState, width int) string {
	width = max(width, 1)
	m.inspector.toolLines = nil
	m.inspector.taskLines = nil
	activeTab := effectiveInspectorTab(*m)
	pendingSubmission := m.pendingInteractionSubmissionInFlight()
	if isWideShell(*m) {
		if activeTab == inspectorTabTools {
			rendered := renderGroupedToolsInspector(*m, state, width)
			m.inspector.toolLines = rendered.LineActions
			return rendered.Content
		}
		if activeTab == inspectorTabTasks {
			rendered := renderTasksInspectorView(*m, state, width)
			m.inspector.taskLines = rendered.LineTaskIDs
			return rendered.Content
		}
		return renderSplitSidebarContent(*m, state, width)
	}
	if activeTab == inspectorTabTasks && !m.hasPendingApproval() && pendingSubmission {
		rendered := renderTasksInspectorView(*m, state, width)
		m.inspector.taskLines = rendered.LineTaskIDs
		return rendered.Content
	}
	return renderInspectorBody(*m, state, width)
}

func (m *Model) syncViewportLayout() {
	state := m.projector.CurrentState()
	layout := resolveShellLayout(*m, state)
	if shellLayoutEnabled(*m) {
		width := max(layout.totalWidth, 1)
		m.setComposerWidth(width)
		m.syncMouseRegions(state, layout)
		questionHeight := questionPromptPanelHeight(*m, width)
		permissionHeight := permissionPromptPanelHeight(*m, state, width)
		statusHeight := transcriptStatusBarHeight(*m, state, width)
		headerHeight := kodaShellHeaderHeight(*m, state, width)
		footerHeight := kodaShellFooterHeight(*m, state, width)
		messageHeight := max(m.height-headerHeight-footerHeight-questionHeight-permissionHeight-statusHeight, 1)
		m.setTranscriptMessagesSize(width, messageHeight)
		m.messages.ApplyTheme(m.theme)
		m.syncTranscriptStructureWithState(state)
		m.syncDialogFrameWithState(state)
		return
	}
	if isWideShell(*m) {
		m.setComposerWidth(layout.totalWidth)
		layout = normalizeWideShellLayout(*m, state, layout)
	}
	m.syncMouseRegions(state, layout)
	if isWideShell(*m) {
		panelHeight := splitWidePanelHeight(layout)
		transcriptGeometry := resolveTranscriptViewportGeometry(*m, state, layout)
		m.setTranscriptMessagesSize(transcriptGeometry.viewportRect.width, transcriptGeometry.viewportRect.height)
		m.messages.ApplyTheme(m.theme)
		m.syncTranscriptStructureWithState(state)
		if layout.showInspector {
			m.setInspectorBodySize(
				max(layout.rightWidth-2, 1),
				splitInspectorViewportHeight(*m, layout.rightWidth, panelHeight),
			)
			m.inspector.body.ApplyTheme(m.theme)
			m.syncInspectorBody(false)
		}
		m.syncDialogFrameWithState(state)
		return
	}
	if layout.showSidePanel && layout.sidePanelWidth > 0 {
		bodyHeight := sessionsBodyHeight(layout.mainHeight)
		m.setSessionsBodySize(max(layout.sidePanelWidth, 1), bodyHeight)
		m.sessionsBody.ApplyTheme(m.theme)
		m.syncSessionsBody()
	}
	transcriptGeometry := resolveTranscriptViewportGeometry(*m, state, layout)
	m.setTranscriptMessagesSize(transcriptGeometry.viewportRect.width, transcriptGeometry.viewportRect.height)
	m.messages.ApplyTheme(m.theme)
	m.setComposerWidth(layout.totalWidth)
	m.syncTranscriptStructureWithState(state)
	if layout.showInspector {
		m.setInspectorBodySize(max(layout.rightWidth, 1), inspectorBodyHeight(layout.inspectorHeight))
		m.inspector.body.ApplyTheme(m.theme)
		m.syncInspectorBody(false)
	}
	m.syncDialogFrameWithState(state)
}

func (m Model) handleInspectorScroll(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.inspector.body.Update(msg)
	return m, cmd
}

func inspectorBodyHeight(total int) int {
	return max(total-3, 1)
}

func transcriptBodyHeight(total int) int {
	return max(total-2, 1)
}

func sessionsBodyHeight(total int) int {
	return max(total-2, 1)
}

func inspectorAutoFollowBottom(m Model) bool {
	if m.pendingInteractionSubmissionInFlight() {
		return false
	}
	if m.pendingExecution() != nil || m.pendingPermission() != nil {
		return true
	}
	return effectiveInspectorTab(m) != inspectorTabDetails
}

func inspectorViewportKey(m Model, state events.SessionState) string {
	activeTab := effectiveInspectorTab(m)
	if m.pendingInteractionSubmissionInFlight() {
		switch activeTab {
		case inspectorTabTools:
			return "tools:" + m.sessionID
		case inspectorTabTasks:
			return "history:" + m.sessionID
		default:
			return "overview:" + effectiveDetailTurnID(m, state)
		}
	}
	switch {
	case m.pendingExecution() != nil:
		return "execution-approval:" + m.pendingExecution().RequestID
	case m.pendingPermission() != nil:
		return "permission:" + m.pendingPermission().RequestID
	case activeTab == inspectorTabTools:
		return "tools:" + m.sessionID
	case activeTab == inspectorTabTasks:
		return "history:" + m.sessionID
	default:
		return "overview:" + effectiveDetailTurnID(m, state)
	}
}

type viewportLayoutState struct {
	shell                  shellLayout
	questionPromptHeight   int
	permissionPromptHeight int
	transcriptStatusHeight int
	footerActivityVisible  bool
}

type transcriptViewportGeometry struct {
	viewportRect           inputMouseRect
	questionPromptHeight   int
	permissionPromptHeight int
	transcriptStatusHeight int
}

func resolveTranscriptViewportGeometry(m Model, state events.SessionState, layout shellLayout) transcriptViewportGeometry {
	if isWideShell(m) {
		layout = normalizeWideShellLayout(m, state, layout)
	}
	rect := resolveShellRects(m, state, layout).transcript

	if isWideShell(m) {
		borderless := !layout.showInspector
		paneWidth := max(layout.centerWidth-4, 1)
		panelHeight := splitWidePanelHeight(layout)
		if borderless {
			paneWidth = max(layout.centerWidth, 1)
		} else {
			rect.x += 2
			rect.y++
		}
		questionHeight := questionPromptPanelHeight(m, paneWidth)
		permissionHeight := permissionPromptPanelHeight(m, state, paneWidth)
		statusHeight := transcriptStatusBarHeight(m, state, paneWidth)
		viewportHeight := max(panelHeight-3-questionHeight-permissionHeight-statusHeight, 1)
		viewportWidth := transcriptViewportWidth(paneWidth)
		if borderless {
			viewportWidth = paneWidth
			viewportHeight = max(panelHeight-questionHeight-permissionHeight-statusHeight, 1)
		}
		rect.y += permissionHeight + questionHeight
		rect.width = viewportWidth
		rect.height = viewportHeight
		return transcriptViewportGeometry{
			viewportRect:           rect,
			questionPromptHeight:   questionHeight,
			permissionPromptHeight: permissionHeight,
			transcriptStatusHeight: statusHeight,
		}
	}

	paneWidth := max(layout.centerWidth, 1)
	questionHeight := questionPromptPanelHeight(m, paneWidth)
	permissionHeight := permissionPromptPanelHeight(m, state, paneWidth)
	statusHeight := transcriptStatusBarHeight(m, state, paneWidth)
	rect.y += 2 + permissionHeight + questionHeight
	rect.width = transcriptViewportWidth(paneWidth)
	rect.height = max(transcriptBodyHeight(layout.transcriptHeight)-questionHeight-permissionHeight-statusHeight, 1)
	return transcriptViewportGeometry{
		viewportRect:           rect,
		questionPromptHeight:   questionHeight,
		permissionPromptHeight: permissionHeight,
		transcriptStatusHeight: statusHeight,
	}
}

func viewportLayoutStateFor(m Model, state events.SessionState) viewportLayoutState {
	layout := resolveShellLayout(m, state)
	if isWideShell(m) {
		layout = normalizeWideShellLayout(m, state, layout)
	}
	transcriptGeometry := resolveTranscriptViewportGeometry(m, state, layout)
	return viewportLayoutState{
		shell:                  layout,
		questionPromptHeight:   transcriptGeometry.questionPromptHeight,
		permissionPromptHeight: transcriptGeometry.permissionPromptHeight,
		transcriptStatusHeight: transcriptGeometry.transcriptStatusHeight,
		footerActivityVisible:  m.footerNotice.activity != nil && m.footerNotice.activity.text != "",
	}
}
