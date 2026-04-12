package tui

import (
	"context"
	"log"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type reopenSessionDialogMsg struct{}

func (a App) handlePaletteResult(result any) (tea.Model, tea.Cmd) {
	switch r := result.(type) {
	case PaletteFileResult:
		att, err := ValidateAttachment(r.Path, a.maxAttachmentSize)
		if err != nil {
			return a, a.showErrorToast(err.Error())
		}
		a.appendPendingAttachment(att)
		return a, nil
	case PaletteSymbolResult:
		att, err := ValidateAttachment(r.File, a.maxAttachmentSize)
		if err != nil {
			return a, a.showErrorToast(err.Error())
		}
		a.appendPendingAttachment(att)
		return a, nil
	case SessionDialogResult:
		return a.handleSessionDialogResult(r)
	case PaletteCommandResult:
		updated, cmd, _ := a.handleSlashCommand(r.Command)
		return updated, cmd
	default:
		return a, nil
	}
}

func (a App) handleAgentDialogResult(result any) (tea.Model, tea.Cmd) {
	item, ok := result.(AgentItem)
	if !ok {
		return a, nil
	}
	persistCmd := tea.Cmd(nil)
	if a.switchAgent(item.ID) {
		persistCmd = a.scheduleAgentPersistence(item.ID)
	}
	api := a.api
	ctx := a.ctx
	agentID := item.ID
	sessionID := a.sessionID
	updateCmd := func() tea.Msg {
		if sessionID != "" {
			_ = api.UpdateSessionAgent(ctx, sessionID, agentID)
		}
		return nil
	}
	return a, tea.Batch(updateCmd, persistCmd)
}

func (a App) handleSessionDialogResult(result any) (tea.Model, tea.Cmd) {
	r, ok := result.(SessionDialogResult)
	if !ok {
		return a, nil
	}
	if r.Delete {
		api, appCtx := a.api, a.ctx
		return a, func() tea.Msg {
			ctx, cancel := context.WithTimeout(appCtx, 5*time.Second)
			defer cancel()
			_ = api.DeleteSession(ctx, r.Session.ID)
			return reopenSessionDialogMsg{}
		}
	}
	if len(r.PurgeIDs) > 0 {
		api, appCtx := a.api, a.ctx
		return a, func() tea.Msg {
			ctx, cancel := context.WithTimeout(appCtx, 10*time.Second)
			defer cancel()
			for _, id := range r.PurgeIDs {
				_ = api.DeleteSession(ctx, id)
			}
			return reopenSessionDialogMsg{}
		}
	}
	if r.New {
		text := r.Title
		if text == "" {
			text = "New session"
		}
		return a, a.startSession(text, nil)
	}
	return a, a.switchSession(r.Session.ID)
}

func (a App) handleReplayDialogResult(result any) (tea.Model, tea.Cmd) {
	item, ok := result.(ReplayItem)
	if !ok {
		return a, nil
	}
	api := a.api
	ctx := a.ctx
	sessionID := a.sessionID
	turn := item.TurnIndex
	return a, func() tea.Msg {
		if err := api.RestoreSnapshot(ctx, sessionID, turn); err != nil {
			return replayRestoreMsg{err: err}
		}
		return replayRestoreMsg{turn: turn}
	}
}

func (a App) handleModelDialogResult(result any) (tea.Model, tea.Cmd) {
	if _, ok := result.(modelRefreshRequest); ok {
		api := a.api
		ctx := a.ctx
		currentModelID := a.session.header.modelID
		if currentModelID == "" {
			currentModelID = a.cfg.Model
		}
		return a, func() tea.Msg {
			providers, _ := api.RefreshModels(ctx)
			items := buildModelItems(providers)
			return openDialogMsg{dialog: NewModelPickerDialog(dialogIDModel, items, currentModelID, a.theme)}
		}
	}
	item, ok := result.(ModelItem)
	if !ok {
		return a, nil
	}
	fullModelID := item.ProviderID + "/" + item.ModelID
	info := modelInfoString(item)
	a.session.SetModel(fullModelID)
	a.session.SetProviderName(item.ProviderName)
	a.session.SetModelInfo(info)
	a.home.SetModel(fullModelID)
	a.home.SetProviderName(item.ProviderName)
	a.cfg.Model = fullModelID
	a.cfg.ModelInfo = info
	a.cfg.HasReasoning = item.Reasoning
	a.cfg.Variant = "adaptive"
	a.session.header.SetVariant("")
	a.home.SetVariant("")
	slashCmds := a.buildSlashCommands()
	a.home.footer.SetSlashCommands(slashCmds)
	a.session.footer.SetSlashCommands(slashCmds)
	api := a.api
	ctx := a.ctx
	sessionID := a.sessionID
	return a, func() tea.Msg {
		if sessionID != "" {
			_ = api.UpdateSessionModel(ctx, sessionID, fullModelID)
		}
		_ = api.SetSetting(ctx, "last_model", fullModelID)
		return nil
	}
}

func (a App) handleThemeDialogResult(result any) (tea.Model, tea.Cmd) {
	if item, ok := result.(ThemeItem); ok {
		return a, a.applyThemeByName(item.Name)
	}
	return a, nil
}

func (a App) handleProviderDialogResult(result any) (tea.Model, tea.Cmd) {
	if r, ok := result.(ProviderConnectResult); ok {
		return a, a.applyProviderConnection(r)
	}
	return a, nil
}

func (a App) handleDynamicDialogResult(id string, result any, questionPrompt string) (tea.Model, tea.Cmd) {
	if rest, ok := strings.CutPrefix(id, dialogIDPermission+":"); ok {
		sessionID, questionID, ok := strings.Cut(rest, ":")
		if ok {
			var response string
			if action, ok := result.(PermissionAction); ok {
				switch action {
				case PermissionAllow:
					response = "once"
				case PermissionAllowAlways:
					response = "always"
				default:
					response = "reject"
				}
			} else {
				response = "reject"
			}
			api := a.api
			ctx := a.ctx
			return a, func() tea.Msg {
				if err := api.AnswerQuestion(ctx, sessionID, questionID, response); err != nil {
					log.Printf("tui: answer permission question %s failed: %v", questionID, err)
				}
				return nil
			}
		}
	}

	if sessionID, questionID, purpose, ok := parseUserQuestionDialogID(id); ok {
		var response string
		switch v := result.(type) {
		case string:
			response = v
		case []string:
			response = strings.Join(v, ", ")
		default:
			response = ""
		}
		// No AppendQuestionAnswer — the question tool_call message already
		// contains the question and answer in its ToolOutput field.
		if purpose == planApprovalPurpose && a.session.header.agentID == "planner" && a.cfg.PreplanAgent != "" {
			a.cfg.PlannerPending = true
			a.cfg.PlannerChoice = response
		}
		api := a.api
		ctx := a.ctx
		return a, func() tea.Msg {
			if err := api.AnswerQuestion(ctx, sessionID, questionID, response); err != nil {
				log.Printf("tui: answer user question %s failed: %v", questionID, err)
			}
			return nil
		}
	}
	return a, nil
}
