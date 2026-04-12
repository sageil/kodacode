package tui

import (
	"os"
	"os/exec"
	"path/filepath"

	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/lsp"
)

type openDialogMsg struct{ dialog tea.Model }

func (a App) openPalette() tea.Cmd {
	api := a.api
	ctx := a.ctx
	commands := a.buildSlashCommands()
	th := a.theme
	lspProvider := a.lspManager
	return func() tea.Msg {

		var files []string
		out, err := exec.CommandContext(ctx, "git", "ls-files").Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if line != "" {
					files = append(files, line)
				}
			}
		}

		var symbols []lsp.SymbolResult
		if lspProvider != nil && lspProvider.HasRunningServers() {
			if syms, err := lspProvider.WorkspaceSymbolSearch(ctx, "."); err == nil {
				symbols = syms
			}
		}

		var sessionItems []SessionItem
		sessions, err := api.ListSessions(ctx)
		if err == nil {
			for _, s := range sessions {
				sessionItems = append(sessionItems, SessionItem{
					ID: s.ID, Title: s.Title, AgentID: s.AgentID,
				})
			}
		}

		return openDialogMsg{
			dialog: NewPaletteDialog(dialogIDPalette, files, symbols, sessionItems, commands, lspProvider, ctx, th),
		}
	}
}

func (a App) openAgentPicker() tea.Cmd {
	api := a.api
	ctx := a.ctx
	currentAgentID := a.session.header.agentID
	if currentAgentID == "" {
		currentAgentID = a.cfg.Agent
	}
	return func() tea.Msg {
		agents, err := api.ListAgents(ctx)
		if err != nil {
			// On failure, return an empty picker so the user still gets a dialog.
			agents = nil
		}
		var items []AgentItem
		for _, ag := range agents {
			// Only show primary agents in the picker; subagents are invoked via the subagent tool.
			if ag.Mode == "subagent" {
				continue
			}
			items = append(items, AgentItem{ID: ag.ID, Name: ag.Name, Description: ag.Description})
		}
		return openDialogMsg{dialog: NewAgentPickerDialog(dialogIDAgent, items, currentAgentID, a.theme)}
	}
}

func (a App) openModelPicker() tea.Cmd {
	api := a.api
	ctx := a.ctx
	currentModelID := a.session.header.modelID
	if currentModelID == "" {
		currentModelID = a.cfg.Model
	}
	return func() tea.Msg {
		providers, err := api.ListModels(ctx)
		if err != nil {
			providers = nil
		}
		items := buildModelItems(providers)
		return openDialogMsg{dialog: NewModelPickerDialog(dialogIDModel, items, currentModelID, a.theme)}
	}
}

func (a App) openSessionDialog() tea.Cmd {
	api := a.api
	ctx := a.ctx
	return func() tea.Msg {
		sessions, err := api.ListSessions(ctx)
		if err != nil {
			sessions = nil
		}
		var items []SessionItem
		for _, s := range sessions {
			if s.ID == a.sessionID {
				continue
			}
			items = append(items, SessionItem{ID: s.ID, Title: s.Title, AgentID: s.AgentID, CreatedAt: s.UpdatedAt})
		}
		return openDialogMsg{dialog: NewSessionDialog(dialogIDSession, items, a.theme)}
	}
}

// openThemePicker reads the themes directory and opens the theme picker dialog.
// A synthetic "System theme" entry is always prepended (name "default").
func (a App) openThemePicker() tea.Cmd {
	th := a.theme
	current := a.themeName
	return func() tea.Msg {
		items := []ThemeItem{
			{Name: "default", DisplayName: "System theme"},
		}
		entries, err := os.ReadDir(config.ThemesDir())
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if filepath.Ext(name) != ".yaml" {
					continue
				}
				bare := strings.TrimSuffix(name, ".yaml")
				items = append(items, ThemeItem{Name: bare, DisplayName: bare})
			}
		}
		return openDialogMsg{dialog: NewThemePickerDialog(dialogIDTheme, items, th, current)}
	}
}
