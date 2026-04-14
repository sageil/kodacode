package tui

import (
	"encoding/json"
	"log"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type configLoadedMsg struct{ cfg APIConfig }

type mcpStatusRefreshMsg struct{ servers []APIMCPServer }

type mcpStatusMsg struct {
	servers []APIMCPServer
	attempt int
}

type mcpRefreshResultMsg struct {
	tools int
	err   error
}

type snapshotsLoadedMsg struct {
	snapshots []APISnapshot
	err       error
}

type replayRestoreMsg struct {
	turn int
	err  error
}

type agentsLoadedMsg struct {
	agents      []APIAgent
	sessions    []APISession
	lastAgentID string
	lastModelID string
	models      []APIProviderModels
}

func (a App) fetchInitialConfig() tea.Cmd {
	api := a.api
	ctx := a.ctx
	return tea.Batch(
		a.home.Init(),
		shimTick(),
		a.loadWorkspaceStatus(),
		func() tea.Msg {
			cfg, err := api.GetConfig(ctx)
			if err != nil {
				return nil
			}
			return configLoadedMsg{cfg: cfg}
		},
	)
}

func (a *App) handleConfigLoaded(msg configLoadedMsg) (App, tea.Cmd) {
	a.cfg.Agent = msg.cfg.DefaultAgent
	a.traceEnabled = msg.cfg.TraceEnabled
	if wd, err := os.Getwd(); err == nil {
		home, _ := os.UserHomeDir()
		if home != "" && strings.HasPrefix(wd, home) {
			wd = "~" + wd[len(home):]
		}
		a.home.SetProjectDir(wd)
	}
	if a.cfg.Variant != "adaptive" {
		a.home.SetVariant(a.cfg.Variant)
	}
	a.sbToolCount = msg.cfg.ToolCount
	mcpStatuses := make([]MCPServerStatus, len(msg.cfg.MCPServers))
	for i, s := range msg.cfg.MCPServers {
		mcpStatuses[i] = MCPServerStatus(s)
	}
	a.sbMCPServers = mcpStatuses
	a.applyStatusBar()

	api := a.api
	ctx := a.ctx
	return *a, func() tea.Msg {
		agents, _ := api.ListAgents(ctx)
		sessions, _ := api.ListSessions(ctx)
		models, _ := api.ListModels(ctx)
		var lastAgentID, lastModelID string
		if saved, err := api.GetSetting(ctx, "last_agent"); err == nil && saved != "" {
			lastAgentID = saved
		}
		if saved, err := api.GetSetting(ctx, "last_model"); err == nil && saved != "" {
			lastModelID = saved
		} else if len(sessions) > 0 {
			lastModelID = sessions[0].ModelID
		}
		return agentsLoadedMsg{agents: agents, sessions: sessions, lastAgentID: lastAgentID, lastModelID: lastModelID, models: models}
	}
}

func (a *App) handleAgentsLoaded(msg agentsLoadedMsg) (App, tea.Cmd) {
	a.cfg.AgentNames = make(map[string]string, len(msg.agents))
	a.cfg.AgentIDs = make([]string, 0, len(msg.agents))
	a.cfg.PrimaryAgentIDs = make([]string, 0, len(msg.agents))
	for _, ag := range msg.agents {
		agentID := ag.ID
		a.cfg.AgentNames[agentID] = ag.Name
		a.cfg.AgentIDs = append(a.cfg.AgentIDs, agentID)
		if ag.Mode != "subagent" {
			a.cfg.PrimaryAgentIDs = append(a.cfg.PrimaryAgentIDs, agentID)
		}
	}
	sort.Strings(a.cfg.AgentIDs)
	sort.Strings(a.cfg.PrimaryAgentIDs)
	if msg.lastAgentID != "" {
		a.cfg.Agent = msg.lastAgentID
	}
	a.agentPersistDirty = false
	a.agentPersistSeq = 0
	if name, ok := a.cfg.AgentNames[a.cfg.Agent]; ok {
		a.cfg.AgentName = name
	} else if a.cfg.Agent != "" {
		a.cfg.AgentName = formatAgentName(a.cfg.Agent)
	}
	items := a.replaceAvailableModels(msg.models)
	if msg.lastModelID != "" {
		if _, ok := a.cfg.Models[msg.lastModelID]; ok {
			a.cfg.Model = msg.lastModelID
		}
	}
	if a.cfg.Model == "" && len(items) > 0 {
		first := items[0]
		a.cfg.Model = first.ProviderID + "/" + first.ModelID
	}
	if a.cfg.Model != "" {
		a.home.SetModel(a.cfg.Model)
		a.cfg.ModelInfo = a.lookupModelInfo(a.cfg.Model)
		a.home.SetProviderName(a.lookupProviderName(a.cfg.Model))
		if item, ok := a.cfg.Models[a.cfg.Model]; ok {
			a.cfg.HasReasoning = item.Reasoning
			if !item.Reasoning {
				a.cfg.Variant = ""
				a.home.SetVariant("")
			}
		}
	}
	cmds := a.buildSlashCommands()
	a.home.footer.SetSlashCommands(cmds)
	a.session.footer.SetSlashCommands(cmds)

	if saved, err := a.api.GetSetting(a.ctx, "input_history"); err == nil && saved != "" {
		var entries []string
		if json.Unmarshal([]byte(saved), &entries) == nil {
			a.home.footer.LoadHistory(entries)
			a.session.footer.LoadHistory(entries)
		}
	}

	a.home.SetAgent(a.cfg.Agent, a.cfg.AgentName)

	var recent []RecentSession
	for _, s := range msg.sessions {
		if s.Title == "" {
			continue
		}
		recent = append(recent, RecentSession{
			ID:        s.ID,
			Title:     s.Title,
			UpdatedAt: s.UpdatedAt,
		})
		if len(recent) >= 5 {
			break
		}
	}
	a.home.SetRecentSessions(recent)

	if a.autoResume && len(recent) > 0 {
		a.autoResume = false
		log.Printf("init: auto-resuming session %s", recent[0].ID)
		return *a, tea.Batch(a.switchSession(recent[0].ID), a.deferMCPRefresh())
	}

	return *a, a.deferMCPRefresh()
}
