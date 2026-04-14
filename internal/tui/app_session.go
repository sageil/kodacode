package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sageil/kodacode/v1/internal/message"
)

type sessionCreatedMsg struct {
	session  APISession
	text     string
	messages []APIMessage // non-nil when switching to an existing session
}

type messageSentMsg struct {
	sessionID string
	text      string
}

type cancelTurnResultMsg struct {
	sessionID string
	err       error
}

func (a *App) cycleVariant() {
	if len(a.cfg.VariantNames) == 0 {
		return
	}
	idx := 0
	for i, v := range a.cfg.VariantNames {
		if v == a.cfg.Variant {
			idx = (i + 1) % len(a.cfg.VariantNames)
			break
		}
	}
	a.cfg.Variant = a.cfg.VariantNames[idx]
	display := variantDisplayLabel(a.cfg.Variant, a.cfg.Model)
	a.session.header.SetVariant(display)
	a.home.SetVariant(display)
}

func (a *App) setVariant(name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	provID, _, _ := strings.Cut(a.cfg.Model, "/")

	switch provID {
	case "openai", "google":
		// Users type provider-native labels: low/medium/high.
		switch name {
		case "adaptive":
			a.cfg.Variant = "adaptive"
		case "low":
			a.cfg.Variant = "low"
		case "medium":
			a.cfg.Variant = "high" // provider "medium" = kodacode "high" (10K)
		case "high":
			a.cfg.Variant = "max" // provider "high" = kodacode "max" (32K)
		default:
			return
		}
	default:
		// Anthropic and others: use kodacode variant names directly.
		switch name {
		case "adaptive", "low", "high", "max":
			a.cfg.Variant = name
		default:
			return
		}
	}

	display := variantDisplayLabel(a.cfg.Variant, a.cfg.Model)
	a.session.header.SetVariant(display)
	a.home.SetVariant(display)
}

func (a *App) cycleAgent() tea.Cmd {
	current := a.cfg.Agent
	idx := 0
	for i, id := range a.cfg.PrimaryAgentIDs {
		if id == current {
			idx = (i + 1) % len(a.cfg.PrimaryAgentIDs)
			break
		}
	}
	if !a.switchAgent(a.cfg.PrimaryAgentIDs[idx]) {
		return nil
	}
	return a.scheduleAgentPersistence(a.cfg.Agent)
}

func (a *App) switchAgent(agentID string) bool {
	if agentID == "" || agentID == a.cfg.Agent {
		return false
	}
	name := a.cfg.AgentNames[agentID]

	// Remember the previous agent when entering a direct planner session.
	if agentID == "planner" && a.cfg.Agent != "planner" {
		a.cfg.PreplanAgent = a.cfg.Agent
	} else if agentID != "planner" {
		a.cfg.PreplanAgent = ""
	}

	a.cfg.Agent = agentID
	a.cfg.AgentName = name
	a.session.SetAgent(agentID, name)
	a.home.SetAgent(agentID, name)
	return true
}

func (a *App) resetSession(agentID, modelID string) {
	a.stepTraces = nil
	a.pendingUndoFile = ""
	// Preserve history before replacing the session.
	history := a.home.footer.History()
	a.session = NewSession()
	if a.width > 0 {
		a.session.SetSize(a.innerWidth(), a.innerHeight())
	}
	if a.theme != nil {
		a.session.ApplyTheme(a.theme)
	}
	a.session.SetProjectDir(a.projectDir)
	a.session.SetAgent(agentID, a.cfg.AgentNames[agentID])
	a.session.SetModel(modelID)
	a.session.SetProviderName(a.lookupProviderName(modelID))
	a.session.SetModelInfo(a.lookupModelInfo(modelID))
	if a.cfg.HasReasoning && a.cfg.Variant != "adaptive" {
		a.session.header.SetVariant(a.cfg.Variant)
	}
	// Carry slash commands and input history to the new session footer.
	a.session.footer.SetSlashCommands(a.buildSlashCommands())
	a.session.footer.LoadHistory(history)
	a.applyStatusBar()
}

func (a *App) lookupModelInfo(modelID string) string {
	if item, ok := a.cfg.Models[modelID]; ok {
		return modelInfoString(item)
	}
	return a.cfg.ModelInfo
}

// variantDisplayLabel returns the provider-appropriate label for the variant badge.
// Anthropic uses its own names (adaptive/low/high/max), while OpenAI and Google
// use standard effort levels (low/medium/high).
func variantDisplayLabel(variant, modelID string) string {
	if variant == "adaptive" {
		return ""
	}
	provID, _, _ := strings.Cut(modelID, "/")
	switch provID {
	case "openai", "google":
		// Map kodacode variant names to provider-native effort levels.
		switch variant {
		case "low":
			return "low"
		case "high":
			return "medium"
		case "max":
			return "high"
		}
	}
	// Anthropic and others: use variant name as-is.
	return variant
}

// openExternalEditor launches $EDITOR/$VISUAL with a temp .md file containing
// the current input. When the editor exits, the file content replaces the input.
func (a App) openExternalEditor(currentText string) tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}

	// Write current input to temp file.
	tmpFile, err := os.CreateTemp("", "kodacode-*.md")
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	tmpPath := tmpFile.Name()
	if currentText != "" {
		_, _ = tmpFile.WriteString(currentText)
	}
	_ = tmpFile.Close()

	// Split editor command in case of args (e.g. "code --wait").
	parts := strings.Fields(editor)
	args := append(parts[1:], tmpPath)
	cmd := exec.Command(parts[0], args...)

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer func() { _ = os.Remove(tmpPath) }()
		if err != nil {
			return editorDoneMsg{err: err}
		}
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return editorDoneMsg{err: readErr}
		}
		return editorDoneMsg{text: strings.TrimRight(string(data), "\n")}
	})
}

// pushAndSaveHistory adds text to both footers' history and persists to DB.
func (a *App) pushAndSaveHistory(text string) {
	a.home.footer.PushHistory(text)
	a.session.footer.PushHistory(text)
	// Persist asynchronously so the UI does not block.
	entries := a.home.footer.History()
	go func() {
		if data, err := json.Marshal(entries); err == nil {
			_ = a.api.SetSetting(a.ctx, "input_history", string(data))
		}
	}()
}

func (a *App) lookupProviderName(modelID string) string {
	if item, ok := a.cfg.Models[modelID]; ok {
		return item.ProviderName
	}
	return ""
}

// startSession creates a new session via the API, posts the first message, and
// switches to the session route. Returns a tea.Cmd; the actual API calls run
// off the update loop.
func (a App) startSession(text string, attachments []Attachment) tea.Cmd {
	api := a.api
	ctx := a.ctx
	agentID := a.cfg.Agent
	modelID := a.cfg.Model
	variant := a.cfg.Variant
	return func() tea.Msg {
		sess, err := api.CreateSession(ctx, agentID, modelID)
		if err != nil {
			return SSEErrorMsg{Err: fmt.Errorf("create session: %w", err)}
		}

		if err := api.SendMessage(ctx, sess.ID, text, attachments, agentID, variant); err != nil {
			return SSEErrorMsg{SessionID: sess.ID, Err: fmt.Errorf("send message: %w", err)}
		}

		return sessionCreatedMsg{session: sess, text: text}
	}
}

func (a App) handleSessionCreated(msg sessionCreatedMsg) (tea.Model, tea.Cmd) {
	a.cancelRequested = false
	a.queuedTurns = 0
	a.sessionID = msg.session.ID
	a.route = routeSession
	a.resetSession(msg.session.AgentID, msg.session.ModelID)
	a.session.SetQueuedTurns(0)
	// Load pinned instructions for this session.
	a.pins = nil
	if saved, err := a.api.GetSetting(a.ctx, "pins:"+msg.session.ID); err != nil {
		log.Printf("tui: failed to load pins for session %s: %v", msg.session.ID, err)
	} else if saved != "" {
		if err := json.Unmarshal([]byte(saved), &a.pins); err != nil {
			log.Printf("tui: failed to parse pins for session %s: %v", msg.session.ID, err)
		}
	}
	a.session.SetPinCount(len(a.pins))
	if msg.session.Title != "" {
		a.session.SetTitle(msg.session.Title)
	}

	// Load persisted tasks for this session.
	a.taskStore.LoadTasks(a.ctx, msg.session.ID)
	a.refreshTaskPanel()

	// When switching to an existing session, populate history from the API.
	// Do not start SSE because the user is only viewing, not sending a message.
	if msg.messages != nil {
		tuiMsgs := make([]Message, 0, len(msg.messages))
		for _, m := range msg.messages {
			if m.Content != "" {
				switch m.Role {
				case "user", "assistant", "system":
					tuiMsgs = append(tuiMsgs, Message{Role: m.Role, Content: m.Content})
				}
			}
			for _, p := range m.Parts {
				switch p.Type {
				case "background_event":
					if event, err := parseBackgroundEvent(p.Content); err == nil {
						tuiMsgs = append(tuiMsgs, Message{
							Role:    "system",
							Content: formatBackgroundEventMessage(event),
						})
					}
				case "tool_call":
					name, input := parseToolCallPart(p.Content)
					tuiMsgs = append(tuiMsgs, Message{
						Role:      "tool_call",
						ToolName:  name,
						ToolInput: input,
						ToolDone:  true,
						Collapsed: true,
					})
				case "tool_result":
					output, toolErr := parseToolResultPart(p.Content)
					updateLastToolResult(tuiMsgs, output, toolErr)
				}
			}
		}
		a.session.SetMessages(tuiMsgs)
		a.session.FlushMessagesRender()
		// Estimate tokens from loaded history so the status bar shows a count.
		tokenEst := 0
		for _, m := range msg.messages {
			tokenEst += (len(m.Content) + 3) / 4
		}
		if tokenEst > 0 {
			a.session.SetTokens(tokenEst, 0, 0, 0, 0)
		}
		return a, nil
	}

	// New session flow. Add the user message and start SSE for the response.
	if msg.text != "" {
		a.session.AppendUserMessage(msg.text)
	}
	return a, a.startSSE(msg.session.ID)
}

func (a App) sendMessage(text string, attachments []Attachment) tea.Cmd {
	api := a.api
	ctx := a.ctx
	sessionID := a.sessionID
	agentID := a.cfg.Agent
	variant := a.cfg.Variant
	return func() tea.Msg {
		if err := api.SendMessage(ctx, sessionID, text, attachments, agentID, variant); err != nil {
			return SSEErrorMsg{SessionID: sessionID, Err: fmt.Errorf("send message: %w", err)}
		}
		return messageSentMsg{sessionID: sessionID, text: text}
	}
}

func (a App) cancelTurn() tea.Cmd {
	api := a.api
	ctx := a.ctx
	sessionID := a.sessionID
	return func() tea.Msg {
		return cancelTurnResultMsg{
			sessionID: sessionID,
			err:       api.CancelTurn(ctx, sessionID),
		}
	}
}

// handleMessageSent appends the user message to the view and restarts SSE only
// when there is no active connection for the session.
func (a App) handleMessageSent(msg messageSentMsg) (tea.Model, tea.Cmd) {
	a.cancelRequested = false
	if msg.text != "" {
		a.session.AppendUserMessage(msg.text)
	}

	// Only restart SSE if there's no active connection. When a next turn is
	// queued, the same stream stays open across the handoff.
	if !a.sse.IsConnected() {
		return a, a.startSSE(msg.sessionID)
	}
	return a, nil
}

func (a App) switchSession(sessionID string) tea.Cmd {
	api := a.api
	ctx := a.ctx
	return func() tea.Msg {
		sess, err := api.GetSession(ctx, sessionID)
		if err != nil {
			return SSEErrorMsg{SessionID: sessionID, Err: fmt.Errorf("get session: %w", err)}
		}
		msgs, err := api.ListMessages(ctx, sessionID)
		if err != nil {
			// Non-fatal: proceed without history rather than blocking the switch.
			msgs = nil
		}
		return sessionCreatedMsg{session: sess, messages: msgs}
	}
}

func parseToolCallPart(raw string) (name, input string) {
	c, err := message.UnmarshalContent("tool_call", raw)
	if err != nil {
		return "", ""
	}
	tc := c.(message.ToolCallContent)
	return tc.Name, tc.Arguments
}

func parseToolResultPart(raw string) (output string, toolErr string) {
	c, err := message.UnmarshalContent("tool_result", raw)
	if err != nil {
		return raw, ""
	}
	tr := c.(message.ToolResultContent)
	if tr.Error != nil {
		return tr.Output, *tr.Error
	}
	return tr.Output, ""
}

func updateLastToolResult(msgs []Message, output, toolErr string) {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "tool_call" && msgs[i].ToolOutput == "" {
			msgs[i].ToolOutput = output
			msgs[i].ToolError = toolErr
			return
		}
	}
}
