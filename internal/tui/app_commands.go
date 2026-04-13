package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sageil/kodacode/v1/internal/config"
)

func (a App) buildSlashCommands() []SlashCommand {
	cmds := []SlashCommand{
		{Name: "/agents", Desc: "Open agent picker", Category: "Navigation"},
		{Name: "/models", Desc: "Open model picker", Category: "Navigation"},
		{Name: "/sessions", Desc: "Open session list", Category: "Navigation"},
		{Name: "/theme", Desc: "Open theme picker", Category: "Navigation"},
		{Name: "/connect", Desc: "Connect to a provider", Category: "Navigation"},
		{Name: "/new", Desc: "Start a new session", Category: "Navigation"},
		{Name: "/help", Desc: "Open help dialog", Category: "Navigation"},
		{Name: "/config", Desc: "View effective configuration", Category: "Navigation"},
		{Name: "/exit", Desc: "Exit kodacode", Category: "Navigation"},
		{Name: "/pin", Desc: "Pin a sticky instruction for this session", Category: "Session", HelpText: "/pin <text>"},
		{Name: "/pins", Desc: "List pinned instructions", Category: "Session"},
		{Name: "/unpin", Desc: "Remove a pinned instruction", Category: "Session", HelpText: "/unpin <n>"},
		{Name: "/diff", Desc: "Show current branch diff", Category: "Session"},
		{Name: "/undo", Desc: "Preview and confirm revert of file changes", Category: "Session"},
		{Name: "/cost", Desc: "Session cost, tokens, and per-turn trace", Category: "Session"},
		{Name: "/trace", Desc: "Turn detail with step breakdown", Category: "Session", HelpText: "/trace <N>"},
		{Name: "/export", Desc: "Export session as markdown", Category: "Session"},
		{Name: "/search", Desc: "Search in session", Category: "Session", HelpText: "/search <text>"},
		{Name: "/init", Desc: "Generate project instruction file", Category: "Session", HelpText: "/init [file]"},
		{Name: "/reload", Desc: "Check for externally modified files", Category: "Session"},
		{Name: "/replay", Desc: "Navigate session snapshots", Category: "Session"},
		{Name: "/remember", Desc: "Save a project memory", Category: "Memory", HelpText: "/remember <text>"},
		{Name: "/memories", Desc: "List saved memories", Category: "Memory"},
		{Name: "/forget", Desc: "Remove a saved memory", Category: "Memory", HelpText: "/forget <id>"},
		{Name: "/palette", Desc: "Open command palette (Ctrl+P)", Category: "Navigation"},
		{Name: "/refresh", Desc: "Refresh MCP tools", Category: "Debug"},
		{Name: "/logs", Desc: "Open log file (use /logs clear to empty)", Category: "Debug"},
		{Name: "/heap", Desc: "Dump memory profile and open report", Category: "Debug"},
	}
	// Only show variant command if the current model supports reasoning.
	if a.cfg.HasReasoning {
		provID, _, _ := strings.Cut(a.cfg.Model, "/")
		var desc string
		switch provID {
		case "anthropic":
			desc = "Cycle: adaptive/low(3K)/high(10K)/max(32K)"
		case "openai":
			desc = "Cycle: low/medium/high reasoning effort"
		case "google":
			desc = "Cycle: low/medium/high thinking level"
		default:
			desc = "Cycle: low/high/max thinking"
		}
		cmds = append(cmds, SlashCommand{Name: "/variant", Desc: desc, Category: "Navigation"})
	}
	for _, id := range a.cfg.AgentIDs {
		if name, ok := a.cfg.AgentNames[id]; ok {
			cmds = append(cmds, SlashCommand{
				Name:     "/" + id,
				Desc:     "Delegate to " + name + " agent",
				Category: "Agents",
			})
		}
	}
	return cmds
}

// handleSlashCommand checks if text is a slash command and returns the
// modified App, corresponding Cmd, and whether it was handled.
func (a App) handleSlashCommand(text string) (App, tea.Cmd, bool) {
	// Legacy colon command: :theme
	if rest, ok := strings.CutPrefix(text, ":theme"); ok {
		name := strings.TrimSpace(rest)
		if name == "" {
			name = "default"
		}
		return a, a.applyThemeByName(name), true
	}

	// Only process /commands.
	if !strings.HasPrefix(text, "/") {
		return a, nil, false
	}

	parts := strings.SplitN(text, " ", 2)
	cmd := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch cmd {
	case "/agents", "/agent":
		return a, a.openAgentPicker(), true
	case "/models", "/model":
		return a, a.openModelPicker(), true
	case "/sessions", "/session":
		return a, a.openSessionDialog(), true
	case "/theme", "/themes":
		if arg != "" {
			return a, a.applyThemeByName(arg), true
		}
		return a, a.openThemePicker(), true
	case "/connect":
		if a.providerSyncBlocked() {
			return a, a.showErrorToast("Finish active turns before connecting a new provider. Restart is required for provider updates or removals."), true
		}
		// Collect configured provider IDs from the model lookup map.
		seen := make(map[string]bool)
		var configuredProviders []string
		for key := range a.cfg.Models {
			provID, _, _ := strings.Cut(key, "/")
			if !seen[provID] {
				seen[provID] = true
				configuredProviders = append(configuredProviders, provID)
			}
		}
		return a, func() tea.Msg {
			return openDialogMsg{dialog: NewProviderConnectDialog(dialogIDProvider, configuredProviders, a.theme)}
		}, true
	case "/new":
		a.sessionID = ""
		a.queuedTurns = 0
		a.resetSession(a.cfg.Agent, a.cfg.Model)
		a.session.SetQueuedTurns(0)
		a.route = routeSession
		return a, nil, true
	case "/help":
		return a, func() tea.Msg {
			return openDialogMsg{dialog: NewHelpDialog(dialogIDHelp, a.theme, a.buildSlashCommands())}
		}, true
	case "/config":
		projectDir := a.projectDir
		th := a.theme
		return a, func() tea.Msg {
			layered := config.LoadLayered(projectDir)
			return openDialogMsg{dialog: NewConfigViewerDialog(dialogIDConfig, th, layered, projectDir)}
		}, true
	case "/palette":
		return a, a.openPalette(), true
	case "/logs":
		logPath := filepath.Join(config.DataDir(), "kodacode.log")
		if arg == "clear" {
			_ = os.Truncate(logPath, 0)
			if a.route == routeSession {
				a.session.AppendSystemMessage("Log file cleared.")
				a.session.FlushMessagesRender()
			}
			return a, nil, true
		}
		if _, err := os.Stat(logPath); err != nil {
			return a, a.showErrorToast("No log file found. Set debug: true in config."), true
		}
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", logPath)
		default:
			cmd = exec.Command("xdg-open", logPath)
		}
		_ = cmd.Start()
		return a, nil, true
	case "/heap":
		return a, func() tea.Msg {
			return runHeapProfile()
		}, true
	case "/refresh":
		api := a.api
		ctx := a.ctx
		return a, func() tea.Msg {
			n, err := api.RefreshMCPTools(ctx)
			if err != nil {
				return mcpRefreshResultMsg{err: err}
			}
			return mcpRefreshResultMsg{tools: n}
		}, true

	case "/exit", "/quit", "/q":
		if a.cancel != nil {
			a.cancel()
		}
		a.quitting = true
		return a, tea.Quit, true
	case "/variant":
		if a.cfg.HasReasoning {
			if arg != "" {
				a.setVariant(arg)
			} else {
				a.cycleVariant()
			}
		}
		return a, nil, true

	case "/init":
		return a, a.handleInitCommand(arg), true

	case "/pin":
		if arg == "" {
			return a, a.showErrorToast("Usage: /pin <instruction>"), true
		}
		if !a.hasActivePinnedSession() {
			return a, a.showErrorToast("Pins are session-scoped. Start or open a session first."), true
		}
		nextPins := append(append([]string(nil), a.pins...), arg)
		return a, persistPinsCmd(a.api, a.ctx, a.sessionID, nextPins, "📌 Pinned: "+arg), true
	case "/pins":
		if !a.hasActivePinnedSession() {
			return a, a.showErrorToast("Pins are session-scoped. Start or open a session first."), true
		}
		if len(a.pins) == 0 {
			a.session.AppendSystemMessage("📌 No pinned instructions.")
		} else {
			var sb strings.Builder
			sb.WriteString("📌 Pinned instructions:\n")
			for i, p := range a.pins {
				fmt.Fprintf(&sb, "  %d. %s\n", i+1, p)
			}
			a.session.AppendSystemMessage(sb.String())
		}
		a.session.FlushMessagesRender()
		return a, nil, true
	case "/unpin":
		if arg == "" {
			return a, a.showErrorToast("Usage: /unpin <number>"), true
		}
		if !a.hasActivePinnedSession() {
			return a, a.showErrorToast("Pins are session-scoped. Start or open a session first."), true
		}
		idx := 0
		if _, err := fmt.Sscanf(arg, "%d", &idx); err != nil || idx < 1 || idx > len(a.pins) {
			return a, a.showErrorToast(fmt.Sprintf("Invalid pin number. Use 1-%d.", len(a.pins))), true
		}
		removed := a.pins[idx-1]
		nextPins := append(append([]string(nil), a.pins[:idx-1]...), a.pins[idx:]...)
		return a, persistPinsCmd(a.api, a.ctx, a.sessionID, nextPins, "📌 Unpinned: "+removed), true
	case "/remember":
		if arg == "" {
			return a, a.showErrorToast("Usage: /remember <instruction>"), true
		}
		if a.memoryStore == nil {
			return a, a.showErrorToast("Memory store not available"), true
		}
		mem, err := a.memoryStore.Save(arg)
		if err != nil {
			return a, a.showErrorToast("Failed to save: " + err.Error()), true
		}
		a.showCommandFeedback("Remembered: " + mem.Content + " [" + mem.ID + "]")
		return a, nil, true
	case "/memories":
		if a.memoryStore == nil {
			return a, a.showErrorToast("Memory store not available"), true
		}
		memories, err := a.memoryStore.List()
		if err != nil {
			return a, a.showErrorToast("Failed to list: " + err.Error()), true
		}
		if len(memories) == 0 {
			a.showCommandFeedback("No saved memories.")
		} else {
			var sb strings.Builder
			sb.WriteString("Saved memories:\n")
			for _, m := range memories {
				fmt.Fprintf(&sb, "  %s: %s\n", m.ID, m.Content)
			}
			a.showCommandFeedback(sb.String())
		}
		return a, nil, true
	case "/forget":
		if arg == "" {
			return a, a.showErrorToast("Usage: /forget <id>"), true
		}
		if a.memoryStore == nil {
			return a, a.showErrorToast("Memory store not available"), true
		}
		if err := a.memoryStore.Delete(arg); err != nil {
			return a, a.showErrorToast(err.Error()), true
		}
		a.showCommandFeedback("Forgot memory: " + arg)
		return a, nil, true
	case "/reload":
		if a.route == routeSession {
			a.session.AppendUserMessage("/reload")
		}
		return a, runReloadCommand(), true
	case "/diff":
		return a, runDiffCommand(arg), true
	case "/undo":
		if a.route == routeSession {
			display := "/undo"
			if arg != "" {
				display += " " + arg
			}
			a.session.AppendUserMessage(display)
		}
		return a.handleUndoCommand(arg)
	case "/cost":
		if a.route == routeSession {
			msg := a.renderCostMessage(a.session.statusBar)
			a.session.AppendSystemMessage(msg)
			a.session.FlushMessagesRender()
		}
		return a, nil, true
	case "/trace":
		if a.route == routeSession {
			if arg != "" {
				a.session.AppendSystemMessage(a.renderTraceDetail(arg))
			} else {
				a.session.AppendSystemMessage(a.renderCostMessage(a.session.statusBar))
			}
			a.session.FlushMessagesRender()
		}
		return a, nil, true
	case "/export":
		if a.route != routeSession || a.sessionID == "" {
			return a, nil, true
		}
		api := a.api
		ctx := a.ctx
		sessionID := a.sessionID
		title := a.session.header.title
		outPath := arg
		return a, func() tea.Msg {
			return exportSession(ctx, api, sessionID, title, outPath)
		}, true
	case "/search":
		if a.route == routeSession {
			a.session.SetSearch(arg)
		}
		return a, nil, true
	case "/replay":
		if a.route != routeSession || a.sessionID == "" {
			return a, a.showErrorToast("No active session"), true
		}
		api := a.api
		ctx := a.ctx
		sessionID := a.sessionID
		return a, func() tea.Msg {
			snapshots, err := api.ListSnapshots(ctx, sessionID)
			if err != nil || len(snapshots) == 0 {
				return snapshotsLoadedMsg{err: err}
			}
			return snapshotsLoadedMsg{snapshots: snapshots}
		}, true

	default:
		// Dynamic subagent commands: /insight, /polish, /planner, etc.
		agentID := cmd[1:] // strip leading /
		for _, id := range a.cfg.AgentIDs {
			if id == agentID {
				task := arg
				if task == "" {
					task = defaultSubagentTask(agentID)
				}
				display := "/" + agentID
				if arg != "" {
					display += " " + arg
				}
				if a.route == routeSession && a.sessionID != "" {
					a.session.AppendUserMessage(display)
					cmds := []tea.Cmd{spawnSubagentCmd(a.api, a.ctx, a.sessionID, agentID, task)}
					if !a.sse.IsConnected() {
						cmds = append(cmds, a.startSSE(a.sessionID))
					}
					return a, tea.Batch(cmds...), true
				}
				// There is no active session, so create one before spawning.
				return a, a.startSessionThenSubagent(agentID, task), true
			}
		}
		return a, a.showErrorToast("Unknown command: " + cmd), true
	}
}

type subagentSpawnedMsg struct {
	agentID string
	err     error
}

type pinsPersistedMsg struct {
	sessionID string
	pins      []string
	message   string
	err       error
}

// subagentWithSessionMsg signals that a session was created and a subagent was
// spawned on it (for the no-active-session case).
type subagentWithSessionMsg struct {
	session APISession
	agentID string
	task    string // deferred: spawn after SSE is connected
	err     error
}

func (a App) hasActivePinnedSession() bool {
	return a.route == routeSession && a.sessionID != ""
}

func (a *App) showCommandFeedback(text string) {
	if a.route == routeSession {
		a.session.AppendSystemMessage(text)
		a.session.FlushMessagesRender()
		return
	}
	a.infoBanner = text
	a.errorBanner = ""
}

func persistPinsCmd(api Backend, ctx context.Context, sessionID string, pins []string, message string) tea.Cmd {
	return func() tea.Msg {
		data, err := json.Marshal(pins)
		if err != nil {
			return pinsPersistedMsg{
				sessionID: sessionID,
				err:       fmt.Errorf("persist pins: %w", err),
			}
		}
		if err := api.SetSetting(ctx, "pins:"+sessionID, string(data)); err != nil {
			return pinsPersistedMsg{
				sessionID: sessionID,
				err:       fmt.Errorf("persist pins: %w", err),
			}
		}
		return pinsPersistedMsg{
			sessionID: sessionID,
			pins:      pins,
			message:   message,
		}
	}
}

func spawnSubagentCmd(api Backend, ctx context.Context, sessionID, agentID, task string) tea.Cmd {
	return func() tea.Msg {
		if err := api.SpawnSubagent(ctx, sessionID, agentID, task); err != nil {
			return subagentSpawnedMsg{agentID: agentID, err: err}
		}
		return subagentSpawnedMsg{agentID: agentID}
	}
}

func (a App) startSessionThenSubagent(agentID, task string) tea.Cmd {
	api := a.api
	ctx := a.ctx
	agent := a.cfg.Agent
	model := a.cfg.Model
	return func() tea.Msg {
		sess, err := api.CreateSession(ctx, agent, model)
		if err != nil {
			return subagentWithSessionMsg{agentID: agentID, err: fmt.Errorf("create session: %w", err)}
		}
		// Return the session first so the TUI can open an SSE stream
		// before spawning the subagent. The subagent is spawned in
		// the subagentWithSessionMsg handler after SSE is connected.
		return subagentWithSessionMsg{session: sess, agentID: agentID, task: task}
	}
}

func defaultSubagentTask(agentID string) string {
	switch agentID {
	case "explorer":
		return "Explore this project's codebase — summarize the architecture, key files, and how the main components connect."
	case "insight":
		return "Review this session and extract any non-obvious learnings into the appropriate AGENTS.md files."
	case "polish":
		return "Review the current branch diff and clean up any AI-generated code slop."
	case "planner":
		return "Analyze the current task context and create a structured implementation plan."
	case "refactor":
		return "Analyze the current codebase for refactoring opportunities and apply improvements."
	default:
		return "Perform your primary function based on your agent description."
	}
}

func (a App) renderCostMessage(sb StatusBar) string {
	snap := sb.costSnapshot

	costVal := sb.sessionCost
	subagentVal := sb.subagentCost
	if snap != nil {
		costVal = snap.TotalCost
		subagentVal = snap.SubagentCost
	}

	costStr := fmt.Sprintf("$%.4f", costVal)
	if costVal >= 0.01 {
		costStr = fmt.Sprintf("$%.2f", costVal)
	}
	msg := fmt.Sprintf("**Session cost: %s**", costStr)
	if subagentVal > 0 {
		agentStr := fmt.Sprintf("$%.4f", subagentVal)
		if subagentVal >= 0.01 {
			agentStr = fmt.Sprintf("$%.2f", subagentVal)
		}
		msg += fmt.Sprintf(" (agents: %s)", agentStr)
	}

	var stats []string
	stats = append(stats, fmt.Sprintf("Model: %s", a.cfg.Model))
	if len(a.stepTraces) > 0 {
		stats = append(stats, fmt.Sprintf("Turns: %d", len(a.stepTraces)))
		stats = append(stats, fmt.Sprintf("Steps: %d", countSteps(a.stepTraces)))
	}
	msg += "\n" + strings.Join(stats, "  ")

	if sb.inputTokens > 0 {
		inputStr := formatTokenCount(sb.inputTokens)
		if sb.maxInputTokens > 0 {
			inputStr += "/" + formatTokenCount(sb.maxInputTokens)
		}
		var tokenParts []string
		tokenParts = append(tokenParts, fmt.Sprintf("Input: %s↑", inputStr))
		if sb.outputTokens > 0 {
			outStr := formatTokenCount(sb.outputTokens)
			if sb.maxOutputTokens > 0 {
				outStr += "/" + formatTokenCount(sb.maxOutputTokens)
			}
			tokenParts = append(tokenParts, fmt.Sprintf("Output: %s↓", outStr))
		}
		if snap != nil && snap.CacheReadTokens > 0 {
			tokenParts = append(tokenParts, "Cache read: "+formatTokenCount(snap.CacheReadTokens))
		} else if sb.cacheReadTokens > 0 {
			tokenParts = append(tokenParts, "Cache read: "+formatTokenCount(sb.cacheReadTokens))
		}
		msg += "\n" + strings.Join(tokenParts, "  ")
	}

	var extras []string
	if snap != nil {
		if snap.ReasoningTokens > 0 {
			extras = append(extras, "Reasoning: "+formatTokenCount(snap.ReasoningTokens))
		}
		if snap.CacheWriteTokens > 0 {
			extras = append(extras, "Cache write: "+formatTokenCount(snap.CacheWriteTokens))
		}
	} else {
		if sb.reasoningTokens > 0 {
			extras = append(extras, "Reasoning: "+formatTokenCount(sb.reasoningTokens))
		}
		if sb.cacheWriteTokens > 0 {
			extras = append(extras, "Cache write: "+formatTokenCount(sb.cacheWriteTokens))
		}
	}
	if len(extras) > 0 {
		msg += "\n" + strings.Join(extras, "  ")
	}

	if a.traceEnabled && len(a.stepTraces) > 0 {
		msg += "\n" + a.renderTurnSummaryTable()
	}
	return msg
}

// handleInitCommand generates a project instruction file (AGENTS.md or CLAUDE.md).
// It sends a message asking the model to analyze the project and create the file.
func (a App) handleInitCommand(arg string) tea.Cmd {
	filename := "AGENTS.md"
	if arg != "" {
		filename = arg
	} else if strings.Contains(strings.ToLower(a.cfg.Model), "claude") ||
		strings.Contains(strings.ToLower(a.cfg.Model), "anthropic") {
		filename = "CLAUDE.md"
	}

	task := "Analyze this project's codebase and create a `" + filename + "` file in the project root. " +
		"This file provides instructions to AI agents working in this codebase.\n\n" +
		"Include:\n" +
		"- Project overview: language, framework, purpose (1-2 sentences)\n" +
		"- Build and test commands\n" +
		"- Key architectural patterns and conventions used in this project\n" +
		"- File organization: what lives where\n" +
		"- Non-obvious constraints or gotchas a developer should know\n\n" +
		"Do NOT include:\n" +
		"- Generic language/framework documentation\n" +
		"- Anything obvious from file names or standard project structure\n" +
		"- Long explanations — keep each entry to 1-3 lines\n\n" +
		"Read the project structure, key config files, and a few representative source files before writing. " +
		"The file should be concise (under 100 lines) and immediately useful."

	if a.route == routeSession && a.sessionID != "" {
		a.session.AppendUserMessage("/init " + filename)
		return a.sendMessage(task, nil)
	}
	return a.startSession(task, nil)
}

type exportResultMsg struct {
	path string
	err  error
}
