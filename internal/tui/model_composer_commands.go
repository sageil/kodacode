package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
)

type composerCommand struct {
	ID            string
	Name          string
	Description   string
	Usage         string
	FreeformArg   bool
	StageOnSelect bool
}

type composerCommandInvocation struct {
	Command  composerCommand
	Argument string
}

var composerCommands = []composerCommand{
	{ID: "palette", Name: "/palette", Description: "open command palette"},
	{ID: "sessions", Name: "/sessions", Description: "manage sessions"},
	{ID: "init", Name: "/init", Description: "initialize workspace instruction files"},
	{ID: "model", Name: "/model", Description: "switch model"},
	{ID: "variant", Name: "/variant", Description: "set provider reasoning variant", Usage: "/variant [value]"},
	{ID: "thinking", Name: "/thinking", Description: "toggle provider thinking output"},
	{ID: "theme", Name: "/theme", Description: "switch theme"},
	{ID: "connect", Name: "/connect", Description: "connect provider"},
	{ID: "utility-model", Name: "/utility-model", Description: "select utility model"},
	{ID: "reviewer-model", Name: "/reviewer-model", Description: "select reviewer model"},
	{ID: "trust", Name: "/trust", Description: "manage trust for this workspace"},
	{ID: "cost", Name: "/cost", Description: "inspect session cost"},
	{ID: "timeline", Name: "/timeline", Description: "branch from a previous turn"},
	{ID: "trace", Name: "/trace", Description: "inspect one turn", Usage: "/trace [turn-number]", StageOnSelect: true},
	{ID: "restore", Name: "/restore", Description: "restore writes from one turn in this session", Usage: "/restore [turn-number]"},
	{ID: "new", Name: "/new", Description: "start new session"},
	{ID: "history", Name: "/history", Description: "search recent prompts"},
	{ID: "review", Name: "/review", Description: "review current changes", Usage: "/review [instructions]", FreeformArg: true, StageOnSelect: true},
	{ID: "compact", Name: "/compact", Description: "rebuild saved history summary now"},
	{ID: "compress", Name: "/compress", Description: "compress AGENTS.md and project memory", StageOnSelect: true},
	{ID: "quit", Name: "/quit", Description: "exit kodacode"},
}

const newSessionBlockedMessage = "Finish the active turn before starting a new session"
const restoreBlockedMessage = "Finish the active turn before restoring files"
const timelineBlockedMessage = "Finish the active turn before opening timeline"
const timelineUnavailableMessage = "Start a session before opening timeline"
const compactBlockedMessage = "Finish the active turn before rebuilding history summary"
const compactUnavailableMessage = "Start a session before rebuilding history summary"
const initBlockedMessage = "Finish the active turn before initializing workspace instructions"
const compressBlockedMessage = "Finish the active turn before compressing workspace instructions"
const reviewBlockedMessage = "Finish the active turn before starting a review"
const reasoningVariantUnavailableMessage = "/variant is unavailable for the current model and tool setup"
const thinkingUnavailableMessage = "/thinking is unavailable for the current model and tool setup"

func availableComposerCommands(m Model) []composerCommand {
	filtered := make([]composerCommand, 0, len(composerCommands))
	for _, command := range composerCommands {
		switch command.ID {
		case "variant":
			if !currentSessionSupportsReasoningVariants(m) {
				continue
			}
		case "thinking":
			if !currentSessionSupportsThinkingOutput(m) {
				continue
			}
		}
		filtered = append(filtered, command)
	}
	return filtered
}

func lookupComposerCommand(commands []composerCommand, id string) (composerCommand, bool) {
	for _, command := range commands {
		if command.ID == id {
			return command, true
		}
	}
	return composerCommand{}, false
}

func lookupComposerCommandByName(commands []composerCommand, name string) (composerCommand, bool) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return composerCommand{}, false
	}
	for _, command := range commands {
		if strings.TrimPrefix(command.Name, "/") == name {
			return command, true
		}
	}
	return composerCommand{}, false
}

func parseComposerSlashCommand(text string, commands []composerCommand) (composerCommandInvocation, bool, error) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, "\n") {
		return composerCommandInvocation{}, false, nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return composerCommandInvocation{}, true, fmt.Errorf("unknown command /")
	}
	query := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(fields[0], "/")))
	if query == "" {
		return composerCommandInvocation{}, true, fmt.Errorf("unknown command /")
	}
	command, ok := lookupComposerCommandByName(commands, query)
	if !ok {
		if hidden, exists := lookupComposerCommandByName(composerCommands, query); exists {
			switch hidden.ID {
			case "variant":
				return composerCommandInvocation{}, true, fmt.Errorf("%s", reasoningVariantUnavailableMessage)
			case "thinking":
				return composerCommandInvocation{}, true, fmt.Errorf("%s", thinkingUnavailableMessage)
			}
		}
		return composerCommandInvocation{}, true, fmt.Errorf("unknown command /%s", query)
	}
	args := fields[1:]
	rawArgs := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
	switch {
	case len(args) == 0:
		return composerCommandInvocation{Command: command}, true, nil
	case command.Usage == "":
		return composerCommandInvocation{}, true, fmt.Errorf("%s does not take arguments", command.Name)
	case command.FreeformArg:
		return composerCommandInvocation{Command: command, Argument: rawArgs}, true, nil
	case len(args) > 1:
		return composerCommandInvocation{}, true, fmt.Errorf("usage: %s", command.Usage)
	default:
		return composerCommandInvocation{Command: command, Argument: strings.TrimSpace(args[0])}, true, nil
	}
}

func composerSlashQuery(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, "\n") {
		return "", false
	}
	fields := strings.Fields(trimmed)
	if len(fields) != 1 {
		return "", false
	}
	query := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(fields[0], "/")))
	if query == "" {
		return "", true
	}
	return query, true
}

func (m *Model) runComposerCommand(invocation composerCommandInvocation) (tea.Model, tea.Cmd) {
	switch invocation.Command.ID {
	case "palette":
		m.clearComposerDraft()
		return *m, m.openCommandPalette()
	case "sessions":
		m.clearComposerDraft()
		return *m, m.openSessionsDialog()
	case "compact":
		activeTurnCompaction := m.busy && m.hasPendingInteraction()
		if m.busy && !activeTurnCompaction {
			m.clearFooterError()
			m.setComposerError(compactBlockedMessage)
			return *m, nil
		}
		if !m.busy && m.hasPendingInteraction() {
			m.clearFooterError()
			m.setComposerError(compactBlockedMessage)
			return *m, nil
		}
		if strings.TrimSpace(m.sessionID) == "" {
			m.clearFooterError()
			m.setComposerError(compactUnavailableMessage)
			return *m, nil
		}
		m.clearComposerError()
		m.clearFooterError()
		m.userText = ""
		compactionTurnID := strings.TrimSpace(m.turnID)
		if !activeTurnCompaction {
			m.busy = true
			m.armLiveTurn()
			compactionTurnID = app.NewTurnID()
			m.turnID = compactionTurnID
			m.selection.detailTurnID = m.turnID
			m.selection.callSessionID = ""
			m.selection.callTurnID = ""
			m.selection.callID = ""
			m.clearExpandedToolCall()
			m.selection.handoffID = ""
			m.inspector.tab = 1
		}
		m.clearComposerDraft()
		if !m.hasPendingInteraction() {
			m.chrome.focus = focusTranscript
			m.syncViewportLayout()
			m.messages.GotoBottom()
			m.syncInspectorBody(true)
		}
		return *m, tea.Batch(
			m.syncComposerFocus(),
			compactSessionHistoryCmd(m.ctx, m.controller, m.sessionID, compactionTurnID),
			func() tea.Msg {
				if activeTurnCompaction {
					return nil
				}
				return m.ensureAnimTicking()()
			},
		)
	case "model":
		m.clearComposerDraft()
		return *m, m.openModelDialog()
	case "utility-model":
		m.clearComposerDraft()
		return *m, m.openUtilityModelDialog()
	case "reviewer-model":
		m.clearComposerDraft()
		return *m, m.openReviewerModelDialog()
	case "init":
		if m.busy || m.hasPendingInteraction() {
			m.clearFooterError()
			m.setComposerError(initBlockedMessage)
			return *m, nil
		}
		includeClaude := false
		modelRoute, routeOK := effectiveSelectedAgentModelRoute(*m, m.projector.CurrentState())
		if ref, ok := effectiveSelectedAgentModelRef(*m, m.projector.CurrentState()); ok {
			includeClaude = provider.CanonicalProviderID(ref.ProviderID) == "anthropic"
		}
		m.clearComposerError()
		m.clearFooterError()
		m.clearComposerDraft()
		if !routeOK {
			modelRoute = provider.ModelRoute{}
		}
		return *m, initializeWorkspaceInstructionsCmd(m.ctx, m.backend, app.InitializeWorkspaceInstructionsInput{
			WorkspaceRoot: m.workspace,
			IncludeClaude: includeClaude,
			ModelRoute:    modelRoute,
		})
	case "compress":
		if m.busy || m.hasPendingInteraction() {
			m.clearFooterError()
			m.setComposerError(compressBlockedMessage)
			return *m, nil
		}
		modelRoute, routeOK := effectiveSelectedAgentModelRoute(*m, m.projector.CurrentState())
		m.clearComposerError()
		m.clearFooterError()
		m.clearComposerDraft()
		if !routeOK {
			modelRoute = provider.ModelRoute{}
		}
		return *m, compressWorkspacePromptSourcesCmd(m.ctx, m.backend, app.CompressWorkspacePromptSourcesInput{
			WorkspaceRoot: m.workspace,
			ModelRoute:    modelRoute,
		})
	case "variant":
		if !currentSessionSupportsReasoningVariants(*m) {
			m.setComposerError(reasoningVariantUnavailableMessage)
			return *m, nil
		}
		state := m.projector.CurrentState()
		ref, ok := effectiveSelectedAgentModelRef(*m, state)
		if !ok {
			m.setComposerError(reasoningVariantUnavailableMessage)
			return *m, nil
		}
		model, ok := selectedAvailableModelForState(*m, state)
		if !ok || len(model.SupportedReasoningVariants) == 0 {
			m.setComposerError(reasoningVariantUnavailableMessage)
			return *m, nil
		}
		m.clearComposerError()
		m.clearFooterError()
		m.clearComposerDraft()
		if variant := strings.TrimSpace(invocation.Argument); variant != "" {
			effective := normalizedAvailableModelReasoningVariant(model, variant)
			if effective == "" {
				m.setComposerError(fmt.Sprintf("unsupported reasoning variant %q for %s", variant, ref.String()))
				return *m, nil
			}
			m.reasoningVariant = effective
			return *m, m.showFooterActivity(reasoningVariantFooterLabel(m.reasoningVariant), footerActivityToneInfo, "")
		}
		return *m, m.openReasoningVariantDialog(ref, model.SupportedReasoningVariants, false, m.currentView())
	case "thinking":
		if !currentSessionSupportsThinkingOutput(*m) {
			m.setComposerError(thinkingUnavailableMessage)
			return *m, nil
		}
		m.clearComposerError()
		m.clearFooterError()
		m.thinkingEnabled = !m.thinkingEnabled
		m.clearComposerDraft()
		return *m, m.showFooterActivity(thinkingFooterLabel(m.thinkingEnabled), footerActivityToneInfo, "")
	case "theme":
		m.clearComposerDraft()
		return *m, m.openThemeDialog()
	case "connect":
		m.clearComposerDraft()
		return *m, m.openConnectDialog()
	case "trust":
		m.clearComposerDraft()
		return *m, m.openTrustDialog()
	case "cost":
		m.clearComposerDraft()
		return *m, m.openCostDialog()
	case "timeline":
		if m.busy || m.hasPendingInteraction() {
			m.clearFooterError()
			m.setComposerError(timelineBlockedMessage)
			return *m, nil
		}
		if strings.TrimSpace(m.sessionID) == "" {
			m.clearFooterError()
			m.setComposerError(timelineUnavailableMessage)
			return *m, nil
		}
		m.clearComposerDraft()
		return *m, m.openTimelineDialog()
	case "trace":
		cmd := m.openTraceDialog(invocation.Argument)
		if cmd == nil {
			return *m, nil
		}
		m.clearComposerDraft()
		return *m, cmd
	case "restore":
		if m.busy || m.hasPendingInteraction() {
			m.clearFooterError()
			m.setComposerError(restoreBlockedMessage)
			return *m, nil
		}
		state := m.projector.Snapshot()
		turnID, err := resolveSessionTurnIDArg(*m, state, invocation.Argument)
		if err != nil {
			m.setComposerError(err.Error())
			return *m, nil
		}
		m.clearComposerError()
		m.clearFooterError()
		m.busy = true
		m.clearComposerDraft()
		return *m, restoreTurnWritesCmd(m.ctx, m.controller, m.sessionID, turnID)
	case "new":
		return m.startNewWorkspaceSession(true, true)
	case "quit":
		m.clearComposerDraft()
		m.beginShutdown()
		return *m, tea.Quit
	case "history":
		m.clearComposerDraft()
		return *m, m.openComposerHistory()
	case "review":
		if m.busy || m.hasPendingInteraction() {
			m.clearFooterError()
			m.setComposerError(reviewBlockedMessage)
			return *m, nil
		}
		return m.submitComposerReview(invocation.Argument)
	default:
		return *m, nil
	}
}

func intToString(value int) string {
	return fmt.Sprintf("%d", value)
}

func (m *Model) clearComposerDraft() {
	m.composer.Reset()
	m.clearPendingAttachments()
	m.clearPendingFocusPaths()
	m.clearComposerPastedText()
	m.clearComposerError()
	m.resetComposerHistoryRecall()
	m.dismissComposerPopup()
}

func (m *Model) startNewWorkspaceSession(useComposerError bool, clearComposerDraft bool) (tea.Model, tea.Cmd) {
	if m.busy || m.hasPendingInteraction() {
		if useComposerError {
			m.clearFooterError()
			m.setComposerError(newSessionBlockedMessage)
		} else {
			m.clearComposerError()
			m.setFooterError(newSessionBlockedMessage)
		}
		return *m, nil
	}

	m.clearComposerError()
	m.clearFooterError()
	if clearComposerDraft {
		m.clearComposerDraft()
	} else {
		m.dismissComposerPopup()
	}
	m.busy = true
	return *m, openWorkspaceSessionCmd(m.ctx, m.backend, workspaceSessionOpenRequest{
		WorkspaceRoot:    m.workspace,
		TurnID:           app.NewTurnID(),
		AgentID:          m.agentID,
		StartTurnAgentID: m.agentID,
		ThinkingEnabled:  m.thinkingEnabled,
		ReasoningVariant: m.reasoningVariant,
		SkillIDs:         append([]string(nil), m.skillIDs...),
		InspectorOpen:    m.chrome.inspectorOpen,
		WideSidebarOpen:  m.chrome.wideSidebarOpen,
		WatchID:          m.nextWatch,
	})
}

func reasoningVariantFooterLabel(mode string) string {
	normalized := strings.TrimSpace(strings.ToLower(mode))
	if normalized == "" {
		return "Variant default"
	}
	return "Variant " + normalized
}

func thinkingFooterLabel(enabled bool) string {
	if enabled {
		return "Thinking enabled"
	}
	return "Thinking disabled"
}

func initializeWorkspaceInstructionsCmd(ctx context.Context, backend Backend, input app.InitializeWorkspaceInstructionsInput) tea.Cmd {
	return func() tea.Msg {
		result, err := backend.InitializeWorkspaceInstructions(ctx, input)
		return workspaceInstructionsInitializedMsg{
			result: result,
			err:    err,
		}
	}
}

func compressWorkspacePromptSourcesCmd(ctx context.Context, backend Backend, input app.CompressWorkspacePromptSourcesInput) tea.Cmd {
	return func() tea.Msg {
		result, err := backend.CompressWorkspacePromptSources(ctx, input)
		return workspacePromptSourcesCompressedMsg{
			result: result,
			err:    err,
		}
	}
}
