package tui

import (
	"context"
	"errors"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/tui/theme"
)

var errLocalShellCommandUsage = errors.New("usage: !<command>")

const (
	composerMinHeight = 2
	composerMaxHeight = 6
)

func newComposer(th *theme.Theme) textarea.Model {
	composer := textarea.New()
	composer.SetVirtualCursor(false)
	composer.Placeholder = "Ask kodacode…"
	composer.Prompt = "  "
	composer.ShowLineNumbers = false
	composer.DynamicHeight = false
	composer.MinHeight = composerMinHeight
	composer.MaxHeight = composerMaxHeight
	composer.SetHeight(composerMinHeight)
	composer.SetWidth(1)
	composer.CharLimit = 0

	keyMap := textarea.DefaultKeyMap()
	keyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter"),
		key.WithHelp("shift+enter", "new line"),
	)
	composer.KeyMap = keyMap

	styles := composer.Styles()
	textColor := lipgloss.Color(colorFor(th, "text", "#cdd6f4"))
	subtle := lipgloss.Color(colorFor(th, "subtext", "#a6adc8"))
	styles.Focused.Text = lipgloss.NewStyle().Foreground(textColor)
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(subtle)
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(th, "primary", "#cba6f7"))).Bold(true)
	styles.Focused.EndOfBuffer = lipgloss.NewStyle().Foreground(subtle)
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Blurred.Text = lipgloss.NewStyle().Foreground(textColor)
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(subtle)
	styles.Blurred.Prompt = lipgloss.NewStyle().Foreground(subtle)
	styles.Blurred.EndOfBuffer = lipgloss.NewStyle().Foreground(subtle)
	styles.Blurred.CursorLine = lipgloss.NewStyle()
	composer.SetStyles(styles)

	return composer
}

func (m *Model) syncComposerFocus() tea.Cmd {
	if m.chrome.focus == focusComposer && m.composerInputEnabled() {
		return m.composer.Focus()
	}
	if !m.composerInputEnabled() {
		m.dismissComposerPopup()
	}
	m.composer.Blur()
	return nil
}

func (m *Model) setComposerWidth(width int) {
	innerWidth := max(width-6, 1)
	if shellLayoutEnabled(*m) {
		innerWidth = max(width-2, 1)
	} else if isWideShell(*m) {
		innerWidth = max(width-2, 1)
	}
	m.composer.SetWidth(innerWidth)
}

func (m *Model) submitComposer() (tea.Model, tea.Cmd) {
	if !m.composerInputEnabled() {
		return *m, m.openComposerSetupDialog()
	}
	text := strings.TrimSpace(m.expandComposerFocusPathTokens(m.expandComposerAttachmentTokens(m.expandComposerPastedText(m.composer.Value()))))
	if text == "" && len(m.composerState.pendingAttachments) == 0 && len(m.orderedPendingFocusPaths()) == 0 {
		return *m, nil
	}
	if invocation, ok, err := parseComposerSlashCommand(text, availableComposerCommands(*m)); ok {
		if err != nil {
			m.setComposerError(err.Error())
			return *m, nil
		}
		m.clearComposerError()
		return m.runComposerCommand(invocation)
	}
	if command, ok, err := parseLocalShellCommand(text); ok {
		if err != nil {
			m.setComposerError(err.Error())
			return *m, nil
		}
		m.clearComposerError()
		if m.busy || m.hasPendingInteraction() {
			return *m, nil
		}

		m.busy = true
		m.armLiveTurn()
		m.userText = ""
		m.turnID = app.NewTurnID()
		m.selection.detailTurnID = m.turnID
		m.selection.callSessionID = ""
		m.selection.callTurnID = ""
		m.selection.callID = ""
		m.selection.handoffID = ""
		m.inspector.tab = 1
		m.clearComposerDraft()
		m.chrome.focus = focusTranscript
		m.syncViewportLayout()
		m.messages.GotoBottom()
		m.syncInspectorBody(true)
		if strings.TrimSpace(m.sessionID) == "" {
			return *m, tea.Batch(
				m.syncComposerFocus(),
				openWorkspaceSessionCmd(m.ctx, m.backend, workspaceSessionOpenRequest{
					WorkspaceRoot:     m.workspace,
					LocalShellCommand: command,
					TurnID:            m.turnID,
					AgentID:           m.agentID,
					StartTurnAgentID:  m.agentID,
					ThinkingEnabled:   m.thinkingEnabled,
					ReasoningVariant:  m.reasoningVariant,
					SkillIDs:          append([]string(nil), m.skillIDs...),
					InspectorOpen:     m.chrome.inspectorOpen,
					WideSidebarOpen:   m.chrome.wideSidebarOpen,
					WatchID:           m.nextWatch,
				}),
			)
		}
		return *m, tea.Batch(
			m.syncComposerFocus(),
			runLocalShellCommandCmd(m.ctx, m.controller, m.sessionID, m.turnID, command),
			m.ensureAnimTicking(),
		)
	}
	if m.composerHasIncompleteSkillMention() {
		m.clearComposerError()
		return *m, m.refreshComposerPopup()
	}
	if m.busy || m.hasPendingInteraction() {
		return *m, nil
	}
	return m.submitAgentTurn(m.userTextWithFocusPaths(text), m.pendingAttachmentInputs(), m.agentID, true)
}

func (m *Model) submitAgentTurn(userText string, attachments []app.AttachmentInput, turnAgentID string, recordPromptHistory bool) (tea.Model, tea.Cmd) {
	if m.busy || m.hasPendingInteraction() {
		return *m, nil
	}
	userText = strings.TrimSpace(userText)
	turnAgentID = strings.TrimSpace(turnAgentID)
	if userText == "" && len(attachments) == 0 {
		return *m, nil
	}
	if turnAgentID == "" {
		turnAgentID = m.agentID
	}

	m.clearComposerError()
	m.busy = true
	m.armLiveTurn()
	m.userText = userText
	m.turnID = app.NewTurnID()
	if recordPromptHistory {
		m.recordPromptHistoryPrompt(m.userText, m.turnID)
		m.composerState.promptHistoryLoaded = false
	}
	m.selection.detailTurnID = m.turnID
	m.selection.callSessionID = ""
	m.selection.callTurnID = ""
	m.selection.callID = ""
	m.selection.handoffID = ""
	m.inspector.tab = 1
	m.clearComposerDraft()
	m.clearPendingFocusPaths()
	m.chrome.focus = focusTranscript
	m.syncViewportLayout()
	m.messages.GotoBottom()
	m.syncInspectorBody(true)
	if strings.TrimSpace(m.sessionID) == "" {
		return *m, tea.Batch(
			m.syncComposerFocus(),
			openWorkspaceSessionCmd(m.ctx, m.backend, workspaceSessionOpenRequest{
				WorkspaceRoot:    m.workspace,
				UserText:         m.userText,
				Attachments:      append([]app.AttachmentInput(nil), attachments...),
				TurnID:           m.turnID,
				AgentID:          m.agentID,
				StartTurnAgentID: turnAgentID,
				ThinkingEnabled:  m.thinkingEnabled,
				ReasoningVariant: m.reasoningVariant,
				SkillIDs:         append([]string(nil), m.skillIDs...),
				InspectorOpen:    m.chrome.inspectorOpen,
				WideSidebarOpen:  m.chrome.wideSidebarOpen,
				WatchID:          m.nextWatch,
			}),
		)
	}
	return *m, tea.Batch(
		m.syncComposerFocus(),
		startTurnCmd(m.ctx, m.controller, m.sessionID, m.turnID, m.userText, append([]app.AttachmentInput(nil), attachments...), turnAgentID, m.thinkingEnabled, m.reasoningVariant, m.skillIDs),
		m.ensureAnimTicking(),
	)
}

func (m *Model) submitComposerReview(instructions string) (tea.Model, tea.Cmd) {
	if m.busy || m.hasPendingInteraction() {
		return *m, nil
	}

	m.clearComposerError()
	m.busy = true
	m.armLiveTurn()
	m.userText = strings.TrimSpace(instructions)
	m.turnID = app.NewTurnID()
	m.selection.detailTurnID = m.turnID
	m.selection.callSessionID = ""
	m.selection.callTurnID = ""
	m.selection.callID = ""
	m.selection.handoffID = ""
	m.inspector.tab = 1
	m.clearComposerDraft()
	m.chrome.focus = focusTranscript
	m.syncViewportLayout()
	m.messages.GotoBottom()
	m.syncInspectorBody(true)
	if strings.TrimSpace(m.sessionID) == "" {
		return *m, tea.Batch(
			m.syncComposerFocus(),
			openWorkspaceSessionForReviewCmd(m.ctx, m.backend, workspaceSessionReviewRequest{
				WorkspaceRoot:    m.workspace,
				TurnID:           m.turnID,
				Instructions:     strings.TrimSpace(instructions),
				AgentID:          m.agentID,
				ThinkingEnabled:  m.thinkingEnabled,
				ReasoningVariant: m.reasoningVariant,
				SkillIDs:         append([]string(nil), m.skillIDs...),
				InspectorOpen:    m.chrome.inspectorOpen,
				WideSidebarOpen:  m.chrome.wideSidebarOpen,
				WatchID:          m.nextWatch,
			}),
		)
	}
	return *m, tea.Batch(
		m.syncComposerFocus(),
		startReviewCmd(
			m.ctx,
			m.controller,
			m.sessionID,
			m.turnID,
			strings.TrimSpace(instructions),
			m.thinkingEnabled,
			m.reasoningVariant,
			append([]string(nil), m.skillIDs...),
		),
		m.ensureAnimTicking(),
	)
}

func (m Model) handleComposerInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if !m.composerInputEnabled() {
		return m.enterInsertMode()
	}
	if updated, cmd, handled := m.handleComposerPopupInput(msg); handled {
		return updated, cmd
	}
	if msg.Key().Code == tea.KeyBackspace {
		if token, ok := m.composerBackspaceTokenTarget(); ok {
			m.clearComposerError()
			m.removeComposerProtectedToken(token)
			m.resetComposerHistoryRecall()
			return m, m.refreshComposerPopup()
		}
		if m.composerCursorOffset() == 0 && len(m.composerState.pendingAttachments) > 0 {
			m.clearComposerError()
			m.removePendingAttachmentAt(len(m.composerState.pendingAttachments) - 1)
			m.resetComposerHistoryRecall()
			return m, m.refreshComposerPopup()
		}
	}
	switch msg.String() {
	case "up":
		if updated, cmd, handled := m.handleComposerHistoryNavigation(-1); handled {
			return updated, cmd
		}
	case "down":
		if updated, cmd, handled := m.handleComposerHistoryNavigation(1); handled {
			return updated, cmd
		}
	case "enter":
		return m.submitComposer()
	}
	switch msg.String() {
	case "ctrl+r":
		if m.busy || m.hasPendingInteraction() {
			return m, nil
		}
		m.clearComposerError()
		return m, m.openComposerHistory()
	case "ctrl+e":
		if m.busy || m.hasPendingInteraction() {
			return m, nil
		}
		m.clearComposerError()
		return m, m.openComposerExternalEditor()
	case "pgup":
		focusCmd := m.enterTranscriptScrollMode()
		m.messages.PageUp()
		m.syncTranscriptCursorToViewport()
		return m, focusCmd
	case "pgdown":
		focusCmd := m.enterTranscriptScrollMode()
		m.messages.PageDown()
		m.syncTranscriptCursorToViewport()
		return m, focusCmd
	case "ctrl+p":
		return m, m.openCommandPalette()
	case "ctrl+c":
		m.beginShutdown()
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		before := m.composer.Value()
		m.clearComposerError()
		m.composer, cmd = m.composer.Update(msg)
		if m.composer.Value() != before {
			m.resetComposerHistoryRecall()
		}
		switch msg.Key().Code {
		case tea.KeyLeft:
			m.snapComposerCursorAcrossToken(-1)
		case tea.KeyRight:
			m.snapComposerCursorAcrossToken(1)
		default:
			m.snapComposerCursorOutOfToken()
		}
		return m, tea.Batch(cmd, m.refreshComposerPopup())
	}
}

func (m Model) handleComposerPopupInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "up":
		if m.composerState.popupMode == composerPopupNone {
			return m, nil, false
		}
		if m.composerState.popupMode == composerPopupHistory {
			m.moveComposerHistoryPopup(-1)
		} else {
			m.moveComposerPopup(-1)
		}
		return m, nil, true
	case "down":
		if m.composerState.popupMode == composerPopupNone {
			return m, nil, false
		}
		if m.composerState.popupMode == composerPopupHistory {
			m.moveComposerHistoryPopup(1)
		} else {
			m.moveComposerPopup(1)
		}
		return m, nil, true
	case "esc":
		if m.composerState.popupMode == composerPopupNone {
			return m, nil, false
		}
		m.dismissComposerPopup()
		return m, nil, true
	case "enter":
		if m.composerState.popupMode == composerPopupSlash {
			if invocation, ok, err := parseComposerSlashCommand(m.composer.Value(), availableComposerCommands(m)); ok && err == nil {
				if m.shouldStageComposerSlashCommand(invocation) {
					return m.stageComposerSlashCommand(invocation.Command, invocation.Argument), nil, true
				}
				next, cmd := m.runComposerCommand(invocation)
				return next, cmd, true
			}
		}
		item, ok := m.selectedComposerPopupItem()
		if !ok {
			if m.composerState.popupMode == composerPopupSkills {
				return m, m.ensureComposerSkillsLoaded(), true
			}
			if m.composerState.popupMode == composerPopupPaths {
				return m, m.ensureComposerWorkspacePathsLoaded(), true
			}
			return m, nil, false
		}
		switch m.composerState.popupMode {
		case composerPopupHistory:
			m.resetComposerHistoryRecall()
			m.clearComposerPastedText()
			m.clearPendingFocusPaths()
			m.composer.SetValue(item.Value)
			m.clearComposerError()
			m.dismissComposerPopup()
			m.syncViewportLayout()
			return m, nil, true
		case composerPopupSlash:
			command, ok := lookupComposerCommand(availableComposerCommands(m), item.ID)
			if !ok {
				return m, nil, false
			}
			if command.StageOnSelect {
				return m.stageComposerSlashCommand(command, item.Arg), nil, true
			}
			next, cmd := m.runComposerCommand(composerCommandInvocation{Command: command, Argument: item.Arg})
			return next, cmd, true
		case composerPopupSkills:
			if strings.TrimSpace(item.ID) == "" {
				return m, nil, false
			}
			m.insertComposerSkillMention(item.ID)
			m.clearComposerError()
			m.dismissComposerPopup()
			m.syncViewportLayout()
			return m, nil, true
		case composerPopupPaths:
			if strings.TrimSpace(item.ID) == "" {
				return m, nil, false
			}
			m.insertComposerFocusPath(item.ID)
			m.clearComposerError()
			m.dismissComposerPopup()
			m.syncViewportLayout()
			return m, nil, true
		default:
			return m, nil, false
		}
	default:
		return m, nil, false
	}
}

func (m Model) composerHasIncompleteSkillMention() bool {
	value := strings.TrimSpace(m.composer.Value())
	if value != "$" {
		return false
	}
	query, _, _, ok := composerSkillQueryAtCursor(m.composer.Value(), m.composerCursorOffset())
	return ok && strings.TrimSpace(query) == ""
}

func (m *Model) insertComposerSkillMention(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	value := m.composer.Value()
	_, start, end, ok := composerSkillQueryAtCursor(value, m.composerCursorOffset())
	if !ok {
		m.composer.InsertString("$" + id)
		return
	}
	runes := []rune(value)
	mention := []rune("$" + id)
	replacement := make([]rune, 0, len(runes)-(end-start)+len(mention)+1)
	replacement = append(replacement, runes[:start]...)
	replacement = append(replacement, mention...)
	if end == len(runes) || !isWhitespaceRune(runes[end]) {
		replacement = append(replacement, ' ')
	}
	replacement = append(replacement, runes[end:]...)
	m.composer.SetValue(string(replacement))
	m.setComposerCursorOffset(start + len(mention) + 1)
}

func (m *Model) insertComposerFocusPath(id string) {
	id = cleanWorkspaceFocusPath(id)
	if id == "" {
		return
	}
	var selected app.WorkspacePath
	for _, path := range m.composerState.workspacePaths {
		if cleanWorkspaceFocusPath(path.Path) == id {
			selected = path
			selected.Path = id
			break
		}
	}
	if strings.TrimSpace(selected.Path) == "" {
		selected = app.WorkspacePath{Path: id}
	}
	focusPath, ok := m.appendPendingFocusPath(selected)
	if !ok {
		return
	}
	value := m.composer.Value()
	_, start, end, ok := composerPathQueryAtCursor(value, m.composerCursorOffset())
	if !ok {
		m.composer.InsertString(focusPath.Tag)
		return
	}
	runes := []rune(value)
	tag := []rune(focusPath.Tag)
	replacement := make([]rune, 0, len(runes)-(end-start)+len(tag)+1)
	replacement = append(replacement, runes[:start]...)
	replacement = append(replacement, tag...)
	if end == len(runes) || !isWhitespaceRune(runes[end]) {
		replacement = append(replacement, ' ')
	}
	replacement = append(replacement, runes[end:]...)
	m.composer.SetValue(string(replacement))
	m.setComposerCursorOffset(start + len(tag) + 1)
}

func isWhitespaceRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func (m Model) shouldStageComposerSlashCommand(invocation composerCommandInvocation) bool {
	return invocation.Command.StageOnSelect && strings.TrimSpace(invocation.Argument) == ""
}

func (m *Model) stageComposerSlashCommand(command composerCommand, argument string) tea.Model {
	m.resetComposerHistoryRecall()
	m.clearComposerPastedText()
	m.clearPendingFocusPaths()
	value := command.Name
	if arg := strings.TrimSpace(argument); arg != "" {
		value += " " + arg
	} else if command.FreeformArg || command.Usage != "" {
		value += " "
	}
	m.composer.SetValue(value)
	m.clearComposerError()
	m.dismissComposerPopup()
	m.syncViewportLayout()
	return *m
}

func parseLocalShellCommand(text string) (string, bool, error) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "!") {
		return "", false, nil
	}
	command := strings.TrimSpace(strings.TrimPrefix(trimmed, "!"))
	if command == "" || strings.Contains(command, "\n") {
		return "", true, errLocalShellCommandUsage
	}
	return command, true, nil
}

func runLocalShellCommandCmd(ctx context.Context, controller controller, sessionID, turnID, command string) tea.Cmd {
	return func() tea.Msg {
		return operationDoneMsg{err: controller.RunLocalShellCommand(ctx, sessionID, turnID, command)}
	}
}
