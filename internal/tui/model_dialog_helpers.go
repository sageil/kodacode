package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	tuitheme "github.com/sageil/kodacode/internal/tui/theme"
)

func (m *Model) focusComposerAfterDialogSelection() tea.Cmd {
	if m.hasPendingInteraction() {
		m.chrome.focus = focusTranscript
	} else if !m.composerInputEnabled() {
		m.chrome.focus = focusTranscript
	} else {
		m.chrome.focus = focusComposer
	}
	m.syncViewportLayout()
	return m.syncComposerFocus()
}

func (m *Model) applyTheme(th *tuitheme.Theme) {
	currentValue := m.composer.Value()
	keepFocus := m.chrome.focus == focusComposer

	m.theme = th
	m.themeRenderTheme = th
	m.themeRenderKey = renderThemeCacheKey(th)
	m.messages.ApplyTheme(th)
	m.inspector.body.ApplyTheme(th)
	m.composer = newComposer(th)
	m.composer.SetValue(currentValue)
	if keepFocus {
		_ = m.composer.Focus()
	} else {
		m.composer.Blur()
	}
	if m.dialog != nil {
		m.dialog.ApplyTheme(th)
	}
	m.syncViewportLayout()
}

func dialogRenderSize(m Model, state events.SessionState) (int, int) {
	if m.width <= 0 || m.height <= 0 {
		return m.width, m.height
	}
	area := resolveDialogRenderArea(m, state, resolveShellLayout(m, state))
	return area.width, area.height
}

func (m *Model) syncDialogFrameWithState(state events.SessionState) {
	if m == nil || m.dialog == nil {
		return
	}
	width, height := dialogRenderSize(*m, state)
	m.dialog.SetFrame(width, height)
}

func normalizedThemeSelection(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return tuitheme.StaticDefault().Name
	}
	return name
}

func currentDialogModelSelection(m Model) string {
	return currentDialogModelRef(m).String()
}

func currentDialogModelRef(m Model) provider.ModelRef {
	if route, ok := effectiveSelectedAgentModelRoute(m, m.projector.CurrentState()); ok {
		return route.Primary
	}
	return provider.ModelRef{}
}

func currentDialogUtilityModelSelection(state app.DialogState) string {
	return optionalModelRefString(state.UtilityModel)
}

func currentDialogReviewerModelSelection(state app.DialogState) string {
	return optionalModelRefString(state.ReviewModelRoute.Primary)
}

func skillSelectionFooterLabel(skillIDs []string) string {
	if len(skillIDs) == 0 {
		return "Skills cleared"
	}
	return "Skills: " + strings.Join(skillIDs, ", ")
}

func utilityModelFooterLabel(ref provider.ModelRef) string {
	if strings.TrimSpace(ref.ProviderID) == "" && strings.TrimSpace(ref.ModelID) == "" {
		return "Utility model: primary model fallback"
	}
	return "Utility model: " + ref.String()
}

func reviewerModelFooterLabel(ref provider.ModelRef) string {
	if strings.TrimSpace(ref.ProviderID) == "" && strings.TrimSpace(ref.ModelID) == "" {
		return "Reviewer model: session primary model fallback"
	}
	return "Reviewer model: " + ref.String()
}

func optionalModelRefString(ref provider.ModelRef) string {
	if strings.TrimSpace(ref.ProviderID) == "" && strings.TrimSpace(ref.ModelID) == "" {
		return ""
	}
	return ref.String()
}
