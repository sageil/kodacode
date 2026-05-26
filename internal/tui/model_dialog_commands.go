package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
	tuitheme "github.com/sageil/kodacode/internal/tui/theme"
)

func applyThemeCmd(ctx context.Context, backend Backend, name string) tea.Cmd {
	return func() tea.Msg {
		if err := backend.SetThemeName(ctx, name); err != nil {
			return footerErrorMsg{err: err}
		}
		loaded, err := tuitheme.Load(name)
		if err != nil {
			return footerErrorMsg{err: err}
		}
		return themeAppliedMsg{name: name, theme: loaded}
	}
}

func setPrimaryModelCmd(ctx context.Context, backend Backend, view sessionView, ref provider.ModelRef, watchID int) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(view.SessionID) == "" {
			return openWorkspaceSessionCmd(ctx, backend, workspaceSessionOpenRequest{
				WorkspaceRoot:    view.WorkspaceRoot,
				TurnID:           app.NewTurnID(),
				AgentID:          view.AgentID,
				StartTurnAgentID: view.AgentID,
				ThinkingEnabled:  view.ThinkingEnabled,
				ReasoningVariant: view.ReasoningVariant,
				SkillIDs:         append([]string(nil), view.SkillIDs...),
				InspectorOpen:    view.InspectorOpen,
				WideSidebarOpen:  view.WideSidebarOpen,
				WatchID:          watchID,
				AfterOpen: func(ctx context.Context, backend Backend, sessionID string) error {
					return backend.SetPrimaryModel(ctx, sessionID, ref)
				},
			})()
		}
		if err := backend.SetPrimaryModel(ctx, view.SessionID, ref); err != nil {
			return primaryModelSetMsg{err: err}
		}
		return primaryModelSetMsg{}
	}
}

func setUtilityModelCmd(ctx context.Context, backend Backend, ref provider.ModelRef) tea.Cmd {
	return func() tea.Msg {
		if err := backend.SetUtilityModel(ctx, ref); err != nil {
			return utilityModelSetMsg{err: err}
		}
		state, err := backend.DialogState(ctx)
		return utilityModelSetMsg{
			state: state,
			err:   err,
		}
	}
}

func setReviewerModelCmd(ctx context.Context, backend Backend, ref provider.ModelRef) tea.Cmd {
	return func() tea.Msg {
		if err := backend.SetReviewerModel(ctx, ref); err != nil {
			return reviewerModelSetMsg{err: err}
		}
		state, err := backend.DialogState(ctx)
		return reviewerModelSetMsg{
			state: state,
			err:   err,
		}
	}
}

func refreshModelCatalogCmd(ctx context.Context, backend Backend, query string, selected provider.ModelRef) tea.Cmd {
	return func() tea.Msg {
		state, err := backend.RefreshModelCatalog(ctx)
		if err != nil {
			return modelCatalogRefreshedMsg{query: query, selected: selected, err: err}
		}
		return modelCatalogRefreshedMsg{
			state:    state,
			query:    query,
			selected: selected,
			err:      err,
		}
	}
}

func saveProviderCmd(ctx context.Context, backend Backend, input app.ProviderConnectionInput) tea.Cmd {
	return func() tea.Msg {
		if err := backend.SaveProvider(ctx, input); err != nil {
			return footerErrorMsg{err: err}
		}
		state, err := backend.RefreshModelCatalog(ctx)
		if err != nil {
			return modelCatalogRefreshedMsg{err: err}
		}
		return modelCatalogRefreshedMsg{
			state:    state,
			selected: state.ModelRoute.Primary,
			err:      err,
		}
	}
}

func revokeTrustAndReopenDialogCmd(
	ctx context.Context,
	backend Backend,
	sessionID, workspaceRoot string,
	result trustDialogResult,
	th *tuitheme.Theme,
	width, height int,
) tea.Cmd {
	return func() tea.Msg {
		state, err := backend.RevokeTrust(ctx, app.RevokeTrustInput{
			SessionID:     sessionID,
			WorkspaceRoot: workspaceRoot,
			Scope:         result.Scope,
			Fingerprint:   strings.TrimSpace(result.Fingerprint),
		})
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newTrustDialog(state, th)
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func removeProviderCmd(ctx context.Context, backend Backend, providerID string) tea.Cmd {
	return func() tea.Msg {
		if err := backend.RemoveProvider(ctx, providerID); err != nil {
			return footerErrorMsg{err: err}
		}
		state, err := backend.DialogState(ctx)
		return modelCatalogRefreshedMsg{
			state:    state,
			selected: state.ModelRoute.Primary,
			err:      err,
		}
	}
}
