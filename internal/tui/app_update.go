package tui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
		a.home.SetSize(a.innerWidth(), a.innerHeight())
		a.session.SetSize(a.innerWidth(), a.innerHeight())
		a.home.footer.SetExpandMax(a.innerHeight())
		a.session.footer.SetExpandMax(a.innerHeight())
		for i, d := range a.dialogs {
			updated, cmd := d.Update(msg)
			a.dialogs[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return a, tea.Batch(cmds...)

	case sessionCreatedMsg:
		return a.handleSessionCreated(msg)

	case sessionTitleRefreshMsg:
		return a.handleSessionTitleRefresh(msg)

	case agentPersistTickMsg:
		return a.handleAgentPersistTick(msg)

	case agentPersistResultMsg:
		return a.handleAgentPersistResult(msg)

	case messageSentMsg:
		return a.handleMessageSent(msg)

	case pinsPersistedMsg:
		if msg.err != nil {
			return a, a.showErrorToast(msg.err.Error())
		}
		if msg.sessionID != a.sessionID {
			return a, nil
		}
		a.pins = append([]string(nil), msg.pins...)
		a.session.SetPinCount(len(a.pins))
		if msg.message != "" {
			a.session.AppendSystemMessage(msg.message)
			a.session.FlushMessagesRender()
		}
		return a, nil

	case cancelTurnResultMsg:
		if msg.sessionID != a.sessionID {
			return a, nil
		}
		if msg.err != nil {
			a.cancelRequested = false
			a.infoBanner = ""
			a.errorBanner = "Cancel failed: " + msg.err.Error()
			return a, nil
		}
		a.infoBanner = "Cancelling turn..."
		a.errorBanner = ""
		return a, nil

	case gitBranchMsg:
		a.sbGitBranch = msg.branch
		a.home.SetGitBranch(msg.branch)
		a.applyStatusBar()
		return a, nil

	case configLoadedMsg:
		return a.handleConfigLoaded(msg)

	case agentsLoadedMsg:
		return a.handleAgentsLoaded(msg)

	case mcpStatusRefreshMsg:
		mcpStatuses := make([]MCPServerStatus, len(msg.servers))
		for i, s := range msg.servers {
			mcpStatuses[i] = MCPServerStatus(s)
		}
		a.sbMCPServers = mcpStatuses
		a.applyStatusBar()
		return a, nil

	case mcpStatusMsg:
		mcpStatuses := make([]MCPServerStatus, len(msg.servers))
		for i, s := range msg.servers {
			mcpStatuses[i] = MCPServerStatus(s)
		}
		a.sbMCPServers = mcpStatuses
		a.applyStatusBar()
		hasActive := false
		for _, s := range msg.servers {
			if s.Active {
				hasActive = true
				break
			}
		}
		if !hasActive && len(msg.servers) > 0 && msg.attempt < 5 {
			return a, a.mcpRefreshTick(msg.attempt + 1)
		}
		return a, nil

	case mcpRefreshResultMsg:
		if msg.err != nil {
			return a, a.showErrorToast("MCP refresh failed: " + msg.err.Error())
		}
		if a.route == routeSession {
			a.session.AppendSystemMessage(fmt.Sprintf("MCP tools refreshed: %d tools registered.", msg.tools))
			a.session.FlushMessagesRender()
		}
		return a, a.deferMCPRefresh()

	case homeOpenSessionMsg:
		return a, a.switchSession(msg.sessionID)

	case sseRepumpMsg:
		if a.sse.IsConnected() {
			return a, a.sse.ReadCmd()
		}
		return a, nil

	case SSEBatchMsg:
		return a.handleSSEBatch(msg)

	case SSEEventMsg:
		result, cmd := a.handleSSEEvent(msg)
		a = result.(App)
		a.session.FlushMessagesRender()
		var ecmds []tea.Cmd
		if cmd != nil {
			ecmds = append(ecmds, cmd)
		}
		if a.sse.IsConnected() {
			ecmds = append(ecmds, a.sse.ReadCmd())
		}
		return a, tea.Batch(ecmds...)

	case SSEErrorMsg:
		if msg.SessionID != "" && msg.SessionID != a.sessionID {
			return a, nil
		}
		if isCancellationError(msg.Err) {
			a.cancelRequested = false
			a.infoBanner = "Turn cancelled."
			a.errorBanner = ""
			if a.route == routeSession {
				a.session.AppendDelta("\n[cancelled]")
				a.session.FinishStreaming()
				a.sse.MarkDone()
				a.session.SetStreaming(false)
				a.session.FlushMessagesRender()
			}
			return a, nil
		}
		a.errorBanner = msg.Err.Error()
		if a.route == routeSession {
			a.session.FinishStreaming()
			a.sse.MarkDone()
			a.session.SetStreaming(false)
			a.session.FlushMessagesRender()
		}
		return a, nil

	case attachmentRemoveMsg:
		a.removePendingAttachmentAt(msg.index)
		return a, nil

	case providerConnectedMsg:
		a.infoBanner = msg.message
		return a, nil

	case dialogClosedMsg:
		return a.handleDialogClosed(msg)

	case openDialogMsg:
		if ws, ok := msg.dialog.(interface {
			SetWidth(int)
			Width() int
		}); ok {
			maxW := a.innerWidth() - 2
			if maxW > 0 && ws.Width() > maxW {
				ws.SetWidth(maxW)
			}
		}
		initCmd := msg.dialog.Init()
		a.dialogs = append(a.dialogs, msg.dialog)
		return a, initCmd

	case replayRestoreMsg:
		if msg.err != nil {
			return a, a.showErrorToast("Restore failed: " + msg.err.Error())
		}
		a.session.AppendSystemMessage(fmt.Sprintf("Workspace restored to turn %d.", msg.turn))
		a.session.FlushMessagesRender()
		return a, nil

	case snapshotsLoadedMsg:
		if msg.err != nil {
			return a, a.showErrorToast("Failed to load snapshots")
		}
		if len(msg.snapshots) == 0 {
			return a, a.showErrorToast("No snapshots available")
		}
		items := make([]ReplayItem, len(msg.snapshots))
		for i, s := range msg.snapshots {
			items[i] = ReplayItem{
				TurnIndex: s.TurnIndex,
				Summary:   s.Summary,
				Files:     s.Files,
				CreatedAt: s.CreatedAt,
			}
		}
		dialog := NewReplayDialog(dialogIDReplay, items, a.theme)
		return a, func() tea.Msg { return openDialogMsg{dialog: dialog} }

	case exportResultMsg:
		if msg.err != nil {
			return a, a.showErrorToast("Export failed: " + msg.err.Error())
		}
		a.session.AppendSystemMessage("Session exported to " + msg.path)
		a.session.FlushMessagesRender()
		return a, nil

	case ThemeChangedMsg:
		a.theme = msg.Theme
		if msg.Name != "" {
			a.themeName = msg.Name
			a.session.SetThemeName(msg.Name)
		}
		a.home.ApplyTheme(msg.Theme)
		a.session.ApplyTheme(msg.Theme)
		a.session.FlushMessagesRender()
		return a, nil

	case reopenSessionDialogMsg:
		a.refreshHomeRecentSessions()
		return a, a.openSessionDialog()

	case subagentSpawnedMsg:
		if msg.err != nil {
			return a, a.showErrorToast("Agent " + msg.agentID + " failed: " + msg.err.Error())
		}
		return a, nil

	case subagentWithSessionMsg:
		if msg.session.ID == "" && msg.err != nil {
			return a, a.showErrorToast("Agent " + msg.agentID + " failed: " + msg.err.Error())
		}
		model, cmd := a.handleSessionCreated(sessionCreatedMsg{session: msg.session})
		if msg.err != nil {
			a2 := model.(App)
			return a2, tea.Batch(cmd, a2.showErrorToast("Agent "+msg.agentID+" failed: "+msg.err.Error()))
		}
		// Spawn the subagent after SSE is connected so events are captured.
		if msg.task != "" {
			a2 := model.(App)
			a2.session.AppendUserMessage("/" + msg.agentID + " " + msg.task)
			return a2, tea.Batch(cmd, spawnSubagentCmd(a2.api, a2.ctx, msg.session.ID, msg.agentID, msg.task))
		}
		return model, cmd

	case heapResultMsg:
		if a.route == routeSession {
			if msg.err != "" {
				a.session.AppendSystemMessage(msg.err)
			} else {
				a.session.AppendSystemMessage(msg.output)
			}
			a.session.FlushMessagesRender()
		}
		return a, nil

	case undoResultMsg:
		if msg.clearPending {
			a.pendingUndoFile = ""
		}
		if msg.pendingFile != "" {
			a.pendingUndoFile = msg.pendingFile
		}
		if a.route == routeSession {
			if msg.err != "" {
				a.session.AppendSystemMessage(msg.err)
			} else {
				a.session.AppendSystemMessage("```\n" + msg.output + "\n```")
			}
			a.session.FlushMessagesRender()
		}
		return a, nil

	case reloadResultMsg:
		if a.route == routeSession {
			a.session.AppendSystemMessage("```\n" + msg.output + "\n```")
			a.session.FlushMessagesRender()
		}
		return a, nil

	case diffResultMsg:
		if a.route == routeSession {
			a.session.AppendSystemMessage("```\n" + msg.output + "\n```")
			a.session.FlushMessagesRender()
		}
		return a, nil

	case editorOpenMsg:
		return a, a.openExternalEditor(msg.currentText)

	case editorDoneMsg:
		if msg.err != nil {
			log.Printf("editor: %v", msg.err)
			return a, nil
		}
		if msg.text != "" {
			if a.route == routeHome {
				a.home.footer.input.SetValue(msg.text)
				a.home.footer.SetPendingAttachments(a.pendingAttachments)
			} else {
				a.session.footer.input.SetValue(msg.text)
				a.session.footer.SetPendingAttachments(a.pendingAttachments)
			}
		}
		return a, nil

	case shimTickMsg:
		h, _ := a.home.Update(msg)
		a.home = h

		if a.sse.IsConnected() {
			a.session.AdvanceFooterAnim()
		}
		AdvanceSpinner()
		a.session.InvalidateRunningTools()
		a.session.FlushMessagesRender()
		return a, shimTick()
	}

	if len(a.dialogs) > 0 {
		top := a.dialogs[len(a.dialogs)-1]
		updated, cmd := top.Update(msg)
		a.dialogs[len(a.dialogs)-1] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return a, tea.Batch(cmds...)
	}

	if pm, ok := msg.(tea.PasteMsg); ok {
		a.handlePastedText(pm.String())
		return a, nil
	}
	if pm, ok := msg.(clipboardPastedMsg); ok {
		a.handlePastedText(pm.text)
		return a, nil
	}

	if sub, ok := msg.(submitMsg); ok {
		a.errorBanner = ""
		if updated, cmd, handled := a.handleSlashCommand(sub.text); handled {
			return updated, cmd
		}

		a.pushAndSaveHistory(sub.text)

		if a.cfg.Model == "" {
			return a, a.showErrorToast("No model configured. Use /connect to add a provider or /models to select a model.")
		}
		atts := a.pendingAttachments
		a.pendingAttachments = nil
		if a.route == routeHome {
			a.home.footer.SetPendingAttachments(nil)
			return a, a.startSession(sub.text, atts)
		}
		if a.route == routeSession {
			a.session.footer.SetPendingAttachments(nil)
			if a.sessionID == "" {
				return a, a.startSession(sub.text, atts)
			}
			return a, a.sendMessage(sub.text, atts)
		}
	}

	if kp, ok := msg.(tea.KeyPressMsg); ok {
		if a.errorBanner != "" || a.infoBanner != "" {
			if key.Matches(kp, a.keys.PasteClipboard) {
				if a.errorBanner != "" {
					writeClipboard(a.errorBanner)
				} else {
					writeClipboard(a.infoBanner)
				}
			}
			a.errorBanner = ""
			a.infoBanner = ""
			return a, nil
		}

		switch {
		case key.Matches(kp, a.keys.Quit):
			a.sse.Stop()
			a.flushPendingAgentSelection()
			if a.cancel != nil {
				a.cancel()
			}
			a.quitting = true
			return a, tea.Quit

		case key.Matches(kp, a.keys.CancelStream):
			if (a.route == routeHome && (a.home.footer.slashActive || a.home.footer.historySearch)) ||
				(a.route == routeSession && (a.session.footer.slashActive || a.session.footer.historySearch)) {
				break
			}
			if a.route == routeHome && a.home.recentCursor >= 0 {
				a.home.recentCursor = -1
				return a, nil
			}
			if a.sse.IsConnected() {
				a.cancelRequested = true
				a.session.CancelTaskSpinners()
				a.infoBanner = "Cancelling turn..."
				a.errorBanner = ""
				a.session.FlushMessagesRender()
				return a, a.cancelTurn()
			}
			a.errorBanner = ""
			a.session.FlushMessagesRender()

		case key.Matches(kp, a.keys.CycleAgent):
			if len(a.cfg.PrimaryAgentIDs) > 1 {
				if cmd := a.cycleAgent(); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}

		case key.Matches(kp, a.keys.PasteClipboard):
			if a.route == routeSession {
				return a, pasteClipboardCmd()
			}
			return a, nil

		case key.Matches(kp, a.keys.OpenPalette):
			return a, a.openPalette()

		case key.Matches(kp, a.keys.ToggleCollapse):
			if a.route == routeSession {
				a.session.ToggleAllCollapsed()
				a.session.FlushMessagesRender()
			}
			return a, nil
		}
	}

	switch a.route {
	case routeHome:
		h, cmd := a.home.Update(msg)
		a.home = h
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case routeSession:
		s, cmd := a.session.Update(msg)
		a.session = s
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return a, tea.Batch(cmds...)
}

func isCancellationError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	return isCancellationMessage(err.Error())
}

func isCancellationMessage(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "context cancelled") ||
		strings.Contains(msg, "turn cancelled")
}

func (a *App) handlePastedText(text string) {
	if text == "" {
		return
	}
	if att, ok := a.validatePastedAttachment(text); ok {
		a.appendPendingAttachment(att)
		return
	}
	switch a.route {
	case routeSession:
		a.session.footer.HandlePaste(text)
	case routeHome:
		a.home.footer.HandlePaste(text)
	}
}

func (a App) validatePastedAttachment(text string) (Attachment, bool) {
	path := strings.TrimSpace(text)
	if path == "" || strings.Contains(path, "\n") || strings.Contains(path, "\r") {
		return Attachment{}, false
	}
	if len(path) >= 2 {
		if (path[0] == '\'' && path[len(path)-1] == '\'') ||
			(path[0] == '"' && path[len(path)-1] == '"') {
			path = path[1 : len(path)-1]
		}
	}
	if path == "" {
		return Attachment{}, false
	}
	if _, err := os.Stat(path); err != nil {
		return Attachment{}, false
	}
	att, err := ValidateAttachment(path, a.maxAttachmentSize)
	if err != nil {
		return Attachment{}, false
	}
	return att, true
}

func (a App) View() tea.View {
	if !a.ready {
		v := tea.NewView("loading…")
		v.AltScreen = true
		return v
	}

	var banner string
	if a.errorBanner != "" {
		banner = renderBanner(a.errorBanner, "error", "✗ Error", a.innerWidth(), a.theme)
	} else if a.infoBanner != "" {
		banner = renderBanner(a.infoBanner, "success", "✓ Done", a.innerWidth(), a.theme)
	}

	var content string
	switch a.route {
	case routeHome:
		content = a.home.View()
	case routeSession:
		content = a.session.View()
	}

	if banner != "" {
		content = banner + "\n" + content
	}

	if len(a.dialogs) > 0 {
		dv := a.dialogs[len(a.dialogs)-1].View()
		content = center(dv.Content, a.innerWidth(), a.innerHeight())
	}

	framed := padView(content, a.innerWidth(), a.innerHeight())

	view := tea.NewView(framed)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	if a.quitting {
		view.WindowTitle = ""
	} else {
		view.WindowTitle = a.session.Title()
	}
	return view
}
