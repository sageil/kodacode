package tui

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tool"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// Session is the full-screen session view: header + taskpanel + messages + statusbar + footer.
type Session struct {
	header       Header
	taskPanel    TaskPanel
	msgs         Messages
	footer       Footer
	statusBar    StatusBar
	keys         KeyMap
	width        int
	height       int
	msgsH        int
	inlinePanel  tea.Model
	inlinePanelH int
	panelQueue   []queuedPanel
}

func NewSession() Session {
	return Session{
		header:    NewHeader(),
		taskPanel: NewTaskPanel(),
		msgs:      NewMessages(),
		footer:    NewFooter(),
		statusBar: NewStatusBar(),
		keys:      DefaultKeyMap(),
	}
}

func (s *Session) ApplyTheme(t *theme.Theme) {
	s.header.ApplyTheme(t)
	s.taskPanel.ApplyTheme(t)
	s.msgs.ApplyTheme(t)
	s.footer.ApplyTheme(t)
	s.statusBar.ApplyTheme(t)
}

func (s *Session) SetThemeName(name string) {
	s.msgs.SetThemeName(name)
}

func (s *Session) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.layout()
}

func (s *Session) SetAgent(id, name string) { s.header.SetAgent(id, name) }

func (s *Session) SetCompacting(b bool) {
	s.footer.SetCompacting(b)
	s.statusBar.SetCompacting(b)
}
func (s *Session) SetModel(id string)          { s.header.SetModel(id) }
func (s *Session) SetProviderName(name string) { s.header.SetProviderName(name) }

func (s *Session) SetModelInfo(info string) { s.header.SetModelInfo(info) }

func (s *Session) SetSessionID(id string) { s.header.SetSessionID(id) }

func (s *Session) SetMessages(msgs []Message) { s.msgs.SetMessages(msgs) }

func (s *Session) AppendDelta(delta string)          { s.msgs.AppendDelta(delta) }
func (s *Session) AppendReasoningDelta(delta string) { s.msgs.AppendReasoningDelta(delta) }
func (s *Session) FinishReasoning()                  { s.msgs.FinishReasoning() }

func (s *Session) AppendUserMessage(text string) { s.msgs.AppendUserMessage(text) }

func (s *Session) AppendAssistantMessage(text string) { s.msgs.AppendAssistantMessage(text) }

func (s *Session) AppendSystemMessage(text string) { s.msgs.AppendSystemMessage(text) }

func (s *Session) AppendErrorMessage(text string) { s.msgs.AppendErrorMessage(text) }

func (s *Session) TrimToTurns(n int) { s.msgs.TrimToTurns(n) }

func (s *Session) FinishStreaming() { s.msgs.FinishStreaming() }

func (s *Session) AppendToolStart(name, input, callID string) {
	s.msgs.AppendToolStart(name, input, callID)
}

func (s *Session) UpdateToolInputDelta(name, delta, callID string) {
	s.msgs.UpdateToolInputDelta(name, delta, callID)
}

func (s *Session) UpdateToolOutput(name, chunk, callID string) {
	s.msgs.UpdateToolOutput(name, chunk, callID)
}

func (s *Session) UpdateToolEnd(name, output, toolErr, callID string) {
	s.msgs.UpdateToolEnd(name, output, toolErr, callID)
}

func (s *Session) AppendBackgroundTaskDone(input, output, toolErr string, elapsed time.Duration) {
	s.msgs.AppendBackgroundTaskDone(input, output, toolErr, elapsed)
}

func (s *Session) UpdateSubagentActivity(tool, input, output string, done, hasError bool) {
	s.msgs.UpdateSubagentActivity(tool, input, output, done, hasError)
}

func (s *Session) FlushMessagesRender()  { s.msgs.FlushRender() }
func (s *Session) ToggleAllCollapsed()   { s.msgs.ToggleAllCollapsed() }

func (s *Session) SetStreaming(streaming bool) {
	s.footer.SetStreaming(streaming)
	s.statusBar.SetStreaming(streaming)
}

func (s *Session) SetToolLoopStep(n int) {
	s.statusBar.SetToolLoopStep(n)
	s.footer.SetToolLoopStep(n)
}

func (s *Session) AdvanceFooterAnim()      { s.footer.AdvanceAnim() }
func (s *Session) InvalidateRunningTools() { s.msgs.invalidateRunningTools() }

func (s *Session) SetTokens(inputTokens, outputTokens, contextSize, maxInputTokens, maxOutputTokens int) {
	s.statusBar.SetTokens(inputTokens, outputTokens, contextSize, maxInputTokens, maxOutputTokens)
	s.layout()
}

func (s *Session) SetSessionCost(cost, subagentCost float64) {
	s.statusBar.SetSessionCost(cost, subagentCost)
}

func (s *Session) SetTokenBreakdown(reasoning, cacheRead, cacheWrite int) {
	s.statusBar.SetTokenBreakdown(reasoning, cacheRead, cacheWrite)
}

func (s *Session) SetBudgetWarning(v bool) {
	s.statusBar.SetBudgetWarning(v)
}

func (s *Session) SetCostSnapshot(snap *CostSnapshotPayload) {
	s.statusBar.SetCostSnapshot(snap)
}

func (s Session) Title() string {
	t := s.header.title
	if t == "" {
		return "kodacode"
	}
	return "kodacode – " + t
}

func (s *Session) SetTitle(title string)       { s.header.SetTitle(title) }
func (s *Session) SetActiveModel(model string) { s.header.SetActiveModel(model) }

func (s *Session) SetToolCount(n int) { s.statusBar.SetToolCount(n) }

func (s *Session) SetPinCount(n int) { s.statusBar.SetPinCount(n) }

func (s *Session) SetMCPServers(servers []MCPServerStatus) { s.statusBar.SetMCPServers(servers) }

func (s *Session) SetGitBranch(branch string) { s.statusBar.SetGitBranch(branch) }

func (s *Session) SetSearch(query string) { s.msgs.SetSearch(query) }

type queuedPanel struct {
	panel  tea.Model
	height int
}

func (s *Session) SetInlinePanel(panel tea.Model, height int) {
	if s.inlinePanel != nil {
		s.panelQueue = append(s.panelQueue, queuedPanel{panel: panel, height: height})
		return
	}
	s.inlinePanel = panel
	s.inlinePanelH = height
	s.msgs.ScrollToBottom()
	s.footer.SetBlocked(true)
	s.layout()
}

func (s *Session) InlinePanelQuestionPrompt() string {
	if p, ok := s.inlinePanel.(InlineQuestionPanel); ok {
		return p.Prompt()
	}
	return ""
}

func (s *Session) ClearInlinePanel() {
	s.inlinePanel = nil
	s.inlinePanelH = 0
	if len(s.panelQueue) > 0 {
		next := s.panelQueue[0]
		s.panelQueue = s.panelQueue[1:]
		s.inlinePanel = next.panel
		s.inlinePanelH = next.height
		s.msgs.ScrollToBottom()
		s.footer.SetBlocked(true)
		s.layout()
		return
	}
	s.footer.SetBlocked(false)
	s.layout()
}

func (s Session) HasInlinePanel() bool { return s.inlinePanel != nil }

func (s *Session) SetTasks(tasks []*tool.Task) {
	prevH := s.taskPanel.Height()
	s.taskPanel.SetTasks(tasks)
	if s.taskPanel.Height() != prevH {
		s.layout()
	}
}

func (s *Session) ToggleTaskPanel() {
	s.taskPanel.Toggle()
	s.layout()
}

func (s *Session) CancelTaskSpinners() { s.taskPanel.Cancel() }

func (s *Session) SetLoopDetected(v bool) { s.statusBar.SetLoopDetected(v) }

// layout computes child dimensions from total width/height.
func (s *Session) layout() {
	s.header.SetSize(s.width)
	s.taskPanel.SetSize(s.width)
	s.statusBar.SetSize(s.width)
	footerH := s.footer.Height()
	taskH := s.taskPanel.Height()
	inlineH := 0
	if s.inlinePanel != nil {
		inlineH = s.inlinePanelH
	}
	// Subtract 3 for the "\n" separators between header/body, body/footer, and footer/statusbar in View().
	// Add 1 more for each visible panel ("\n" prefix before task panel and/or inline panel).
	separators := 3
	if taskH > 0 {
		separators++ // "\n" before task panel
	}
	if inlineH > 0 {
		separators++ // "\n" before inline panel
	}
	s.msgsH = s.height - headerHeight - taskH - inlineH - footerH - statusBarHeight - separators
	s.msgsH = max(s.msgsH, 1)
	s.msgs.SetSize(s.width, s.msgsH)
	// screenY is the absolute terminal row where the messages viewport starts.
	s.msgs.screenY = headerHeight + taskH
	s.footer.SetSize(s.width)
}

func (s Session) Init() tea.Cmd {
	return tea.Batch(s.msgs.Init(), s.footer.Focus())
}

func (s Session) Update(msg tea.Msg) (Session, tea.Cmd) {
	var cmds []tea.Cmd

	// The inline panel blocks keyboard input until the user responds.
	// Mouse clicks still pass through so users can expand/collapse tool calls.
	if s.inlinePanel != nil {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			updated, cmd := s.inlinePanel.Update(msg)
			s.inlinePanel = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return s, tea.Batch(cmds...)
		}
	}

	// When footer is in history search mode, let it handle all keys first.
	if _, isKey := msg.(tea.KeyPressMsg); isKey && s.footer.historySearch {
		prevFooterHeight := s.footer.Height()
		var footerCmd tea.Cmd
		s.footer, footerCmd = s.footer.Update(msg)
		if footerCmd != nil {
			cmds = append(cmds, footerCmd)
		}
		if s.footer.Height() != prevFooterHeight {
			s.layout()
		}
		return s, tea.Batch(cmds...)
	}

	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kp, s.keys.ScrollUp):
			s.msgs.vp.ScrollUp(1)
			s.msgs.userScrolled = true
			return s, nil
		case key.Matches(kp, s.keys.ScrollDown):
			s.msgs.vp.ScrollDown(1)
			// Re-enable auto-scroll when user scrolls back to the bottom.
			if s.msgs.vp.AtBottom() {
				s.msgs.userScrolled = false
			}
			return s, nil
		}
	}

	// Click on task panel toggles expand/collapse.
	if click, ok := msg.(tea.MouseClickMsg); ok && s.taskPanel.HasTasks() {
		taskStart := headerHeight // task panel starts right after header
		taskEnd := taskStart + s.taskPanel.Height()
		if click.Y >= taskStart && click.Y < taskEnd {
			s.ToggleTaskPanel()
			return s, nil
		}
	}

	// Footer handles Enter (submit) and all textarea input.
	prevFooterHeight := s.footer.Height()
	var footerCmd tea.Cmd
	s.footer, footerCmd = s.footer.Update(msg)
	if footerCmd != nil {
		cmds = append(cmds, footerCmd)
	}

	// After footer update, re-layout to adjust messages height when textarea grows.
	// Only re-layout when footer height actually changes to avoid flickering.
	if s.footer.Height() != prevFooterHeight {
		s.layout()
	}

	var msgsCmd tea.Cmd
	s.msgs, msgsCmd = s.msgs.Update(msg)
	if msgsCmd != nil {
		cmds = append(cmds, msgsCmd)
	}

	// Render once per update cycle. All mutations above only set needsRender.
	s.msgs.FlushRender()

	return s, tea.Batch(cmds...)
}

func (s Session) View() string {
	// Use the same height that layout() configured for the viewport.
	body := lipgloss.NewStyle().
		Width(s.width).
		Height(s.msgsH).
		Render(s.msgs.View())

	var taskView string
	if tv := s.taskPanel.View(); tv != "" {
		taskView = "\n" + tv
	}

	var inlineView string
	if s.inlinePanel != nil {
		if pv := s.inlinePanel.View().Content; pv != "" {
			inlineView = "\n" + pv
		}
	}

	full := s.header.View() + taskView + "\n" + body + inlineView + "\n" + s.footer.View() + "\n" + s.statusBar.View()
	return full
}
