package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m Model) updateChromeMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.syncViewportLayout()
		return m, nil, true
	case dialogOpenedMsg:
		if typed.err != nil {
			m.setFooterError(typed.err.Error())
			return m, nil, true
		}
		m.clearFooterError()
		m.dialog = typed.dialog
		m.resetDialogRefreshState()
		if m.dialog != nil {
			m.dialog.ApplyTheme(m.theme)
			m.syncDialogFrameWithState(m.projector.CurrentState())
		}
		if initial, ok := m.dialog.(dialogInitialCommander); ok {
			return m, initial.InitialCmd(), true
		}
		return m, nil, true
	case dialogClosedMsg:
		next, cmd := m.handleDialogClosed(typed)
		return next.(Model), cmd, true
	case themeAppliedMsg:
		if typed.theme != nil {
			m.themeName = normalizedThemeSelection(typed.name)
			m.applyTheme(typed.theme)
			m.clearFooterError()
		}
		return m, nil, true
	case layoutPersistedMsg:
		if typed.err != nil {
			m.setFooterError(typed.err.Error())
			return m, nil, true
		}
		return m, nil, true
	case primaryModelSetMsg:
		if typed.err != nil {
			m.setFooterError(typed.err.Error())
			return m, nil, true
		}
		if err := m.refreshDialogState(); err != nil {
			m.setFooterError(err.Error())
			return m, nil, true
		}
		if !m.busy && !m.hasPendingInteraction() && m.composerInputEnabled() {
			m.chrome.focus = focusComposer
			m.syncViewportLayout()
		}
		m.clearFooterError()
		return m, m.syncComposerFocus(), true
	case utilityModelSetMsg:
		if typed.err != nil {
			m.setFooterError(typed.err.Error())
			return m, nil, true
		}
		m.cacheDialogState(typed.state)
		m.clearFooterError()
		return m, tea.Batch(
			m.syncComposerFocus(),
			m.showFooterActivity(utilityModelFooterLabel(typed.state.UtilityModel), footerActivityToneInfo, ""),
		), true
	case reviewerModelSetMsg:
		if typed.err != nil {
			m.setFooterError(typed.err.Error())
			return m, nil, true
		}
		m.cacheDialogState(typed.state)
		m.clearFooterError()
		return m, tea.Batch(
			m.syncComposerFocus(),
			m.showFooterActivity(reviewerModelFooterLabel(typed.state.ReviewModelRoute.Primary), footerActivityToneInfo, ""),
		), true
	case footerErrorMsg:
		if typed.err != nil {
			m.setFooterError(typed.err.Error())
			return m, nil, true
		}
		m.clearFooterError()
		return m, nil, true
	case footerActivityExpiredMsg:
		if m.footerNotice.activity != nil && typed.id == m.footerNotice.activity.id {
			m.clearFooterActivity()
			m.syncViewportLayout()
		}
		return m, nil, true
	case composerExternalEditorDoneMsg:
		if typed.err != nil {
			m.setComposerError(typed.err.Error())
			m.syncViewportLayout()
			return m, m.syncComposerFocus(), true
		}
		m.applyComposerExternalEditorText(typed.text)
		return m, tea.Batch(m.refreshComposerPopup(), m.syncComposerFocus()), true
	case modelCatalogRefreshRequestedMsg:
		if m.dialog == nil || m.dialog.ID() != dialogIDCommandPalette {
			return m, nil, true
		}
		return m, refreshModelCatalogCmd(m.ctx, m.backend, typed.query, typed.selected), true
	case modelCatalogRefreshedMsg:
		if typed.err != nil {
			m.setFooterError(typed.err.Error())
		} else {
			m.clearFooterError()
		}
		wasEnabled := m.composerInputEnabled()
		m.cacheDialogState(typed.state)
		m.syncFocusState()
		var nextCmd tea.Cmd
		switch {
		case !wasEnabled && !m.busy && !m.hasPendingInteraction() && m.dialog == nil && m.composerInputEnabled():
			m.chrome.focus = focusComposer
			m.syncViewportLayout()
		case m.shouldAutoOpenProviderDialog():
			nextCmd = m.openConnectDialog()
		}
		if m.dialog == nil {
			return m, tea.Batch(m.syncComposerFocus(), nextCmd), true
		}
		switch dialog := m.dialog.(type) {
		case *commandPaletteDialog:
			dialog.applyDialogStateRefresh(typed.state, typed.query, typed.selected.String())
			width, height := dialogRenderSize(m, m.projector.CurrentState())
			dialog.SetFrame(width, height)
			m.dialog = dialog
		case *connectDialog:
			dialog.applyDialogStateRefresh(typed.state)
			width, height := dialogRenderSize(m, m.projector.CurrentState())
			dialog.SetFrame(width, height)
			m.dialog = dialog
		default:
			return m, tea.Batch(m.syncComposerFocus(), nextCmd), true
		}
		return m, tea.Batch(m.syncComposerFocus(), nextCmd), true
	default:
		return m, nil, false
	}
}

func (m Model) updateInputAndTickMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch typed := msg.(type) {
	case animTickMsg:
		if !m.shouldAnimateTranscriptActivity() {
			m.animation.ticking = false
			return m, nil, true
		}
		m.animation.frame++
		return m, animTickCmd(), true
	case transcriptRefreshTickMsg:
		m.transcriptRefresh.ticking = false
		return m, m.flushPendingTranscriptRefresh(time.Now()), true
	case dialogRefreshTickMsg:
		m.dialogRefresh.ticking = false
		return m, m.flushPendingDialogRefresh(time.Now()), true
	case tea.KeyPressMsg:
		next, cmd := m.handleKeyPress(typed)
		return next.(Model), cmd, true
	case tea.PasteMsg:
		next, cmd := m.handlePaste(typed)
		return next.(Model), cmd, true
	case tea.MouseClickMsg:
		next, cmd := m.handleMouseClick(typed)
		return next.(Model), cmd, true
	case tea.MouseReleaseMsg:
		next, cmd := m.handleMouseRelease(typed)
		return next.(Model), cmd, true
	case tea.MouseMotionMsg:
		next, cmd := m.handleMouseMotion(typed)
		return next.(Model), cmd, true
	case tea.MouseWheelMsg:
		next, cmd := m.handleMouseWheel(typed)
		return next.(Model), cmd, true
	case coalescedWheelMsg:
		next, cmd := m.handleMouseWheelSteps(typed.msg, typed.steps)
		return next.(Model), cmd, true
	default:
		return m, nil, false
	}
}
