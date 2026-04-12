package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type sseRepumpMsg struct{}

type sseQuestionPayload struct {
	QuestionID string `json:"question_id"`
	Tool       string `json:"tool"`
	Input      string `json:"input"`
}

type sseAssistantMessagePayload struct {
	Content string `json:"content"`
}

type sseUserQuestionPayload struct {
	QuestionID string   `json:"question_id"`
	Question   string   `json:"question"`
	Options    []string `json:"options"`
	Multiple   bool     `json:"multiple"`
	Purpose    string   `json:"purpose"`
}

// startSSE cancels any existing SSE connection, opens a new one, and returns
// the first-read command. Enables the footer streaming animation.
func (a *App) startSSE(sessionID string) tea.Cmd {
	a.session.SetStreaming(true)
	return a.sse.Start(a.ctx, a.api, sessionID)
}

// handleSSEBatch processes a batch of SSE events that arrived between update cycles.
// This avoids per-event render overhead by processing all events then rendering once.
func (a App) handleSSEBatch(batch SSEBatchMsg) (tea.Model, tea.Cmd) {
	var batchCmds []tea.Cmd
	for _, ev := range batch.Events {
		result, cmd := a.handleSSEEvent(ev)
		a = result.(App)
		if cmd != nil {
			batchCmds = append(batchCmds, cmd)
		}
	}

	a.session.FlushMessagesRender()

	if batch.Done {
		a.session.FinishStreaming()
		a.sse.MarkDone()
		a.session.SetStreaming(false)
		a.session.SetToolLoopStep(0)
		a.session.FlushMessagesRender()
		return a, tea.Batch(batchCmds...)
	}

	// Re-issue the read command to pump the next batch.
	if a.sse.IsConnected() {
		batchCmds = append(batchCmds, a.sse.ReadCmd())
	}
	return a, tea.Batch(batchCmds...)
}

// handleSSEEvent processes one streaming event, updating session state.
// Does NOT flush render or re-issue the read command — callers are responsible.
//
// Events whose SessionID does not match the current session are silently
// discarded. This prevents stale events from a cancelled old session from
// appearing in the new session view.
func (a App) handleSSEEvent(msg SSEEventMsg) (tea.Model, tea.Cmd) {
	if msg.SessionID != a.sessionID {
		return a, nil
	}
	switch msg.Type {
	case "delta":
		var p SSEDeltaPayload
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			a.session.AppendDelta(p.Content)
		}
		a.errorBanner = ""

	case "assistant_message":
		var p sseAssistantMessagePayload
		if err := json.Unmarshal(msg.Data, &p); err == nil && p.Content != "" {
			a.session.AppendAssistantMessage(p.Content)
		}

	case "task_sync":
		a.refreshTaskPanel()

	case "system_message":
		var p sseAssistantMessagePayload
		if err := json.Unmarshal(msg.Data, &p); err == nil && p.Content != "" {
			a.session.AppendSystemMessage(p.Content)
		}

	case "reasoning_delta":
		var p SSEDeltaPayload
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			log.Printf("[6-tui] reasoning_delta: %d chars, content=%q", len(p.Content), p.Content)
			a.session.AppendReasoningDelta(p.Content)
		} else {
			log.Printf("[6-tui] reasoning_delta UNMARSHAL ERROR: %v, raw=%s", err, string(msg.Data))
		}

	case "reasoning_done":
		log.Printf("[6-tui] reasoning_done received")
		a.session.FinishReasoning()

	case "usage":
		var p SSEDonePayload
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			inputTokens := p.Usage.InputTokens + p.Usage.CacheReadTokens + p.Usage.CacheWriteTokens
			a.session.SetTokens(inputTokens, p.Usage.OutputTokens, p.ContextSize, p.MaxInputTokens, p.MaxOutputTokens)
			a.session.SetTokenBreakdown(p.Usage.ReasoningTokens, p.Usage.CacheReadTokens, p.Usage.CacheWriteTokens)
			a.session.SetBudgetWarning(p.BudgetWarn)
			if p.CostSnapshot != nil {
				a.session.SetCostSnapshot(p.CostSnapshot)
			}
			if p.ContextSize > 0 {
				if item, ok := a.cfg.Models[a.cfg.Model]; ok {
					if item.ContextSize == 0 || p.ContextSize > item.ContextSize {
						item.ContextSize = p.ContextSize
						a.cfg.Models[a.cfg.Model] = item
					}
					if p.MaxInputTokens > 0 && item.MaxInputTokens == 0 {
						item.MaxInputTokens = p.MaxInputTokens
						a.cfg.Models[a.cfg.Model] = item
					}
					a.session.SetModelInfo(modelInfoString(item))
				} else {
					a.session.SetModelInfo(formatContextSize(p.ContextSize))
				}
			}
			if p.SessionCost > 0 {
				a.session.SetSessionCost(p.SessionCost, p.SubagentCost)
			}
		}

	case "step_trace":
		var p struct {
			TurnIndex int            `json:"turn_index"`
			Steps     []stepTraceTUI `json:"steps"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil && len(p.Steps) > 0 {
			for len(a.stepTraces) <= p.TurnIndex {
				a.stepTraces = append(a.stepTraces, nil)
			}
			a.stepTraces[p.TurnIndex] = p.Steps
		}

	case "done":
		a.cancelRequested = false
		var p SSEDonePayload
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			inputTokens := p.Usage.InputTokens + p.Usage.CacheReadTokens + p.Usage.CacheWriteTokens
			a.session.SetTokens(inputTokens, p.Usage.OutputTokens, p.ContextSize, p.MaxInputTokens, p.MaxOutputTokens)
			a.session.SetTokenBreakdown(p.Usage.ReasoningTokens, p.Usage.CacheReadTokens, p.Usage.CacheWriteTokens)
			a.session.SetBudgetWarning(p.BudgetWarn)
			if p.CostSnapshot != nil {
				a.session.SetCostSnapshot(p.CostSnapshot)
			}
			if p.SessionCost > 0 {
				a.session.SetSessionCost(p.SessionCost, p.SubagentCost)
			}
		}
		a.session.FinishStreaming()
		a.session.SetActiveModel("")
		a.session.TrimToTurns(a.displayTurns)
		// Re-inject pin indicator after trim so it's always visible.
		if len(a.pins) > 0 {
			var pinText strings.Builder
			pinText.WriteString("📌 Active pins:")
			for _, p := range a.pins {
				pinText.WriteString("\n  - ")
				pinText.WriteString(p)
			}
			a.session.AppendSystemMessage(pinText.String())
		}
		a.sse.MarkDone()
		a.session.SetStreaming(false)
		a.session.SetToolLoopStep(0)
		if a.cfg.PlannerPending {
			return a.completePlannerApprovalAfterDone()
		}

	case "tool_start":
		var p struct {
			Tool   string `json:"tool"`
			Input  string `json:"input"`
			CallID string `json:"call_id"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			a.errorBanner = ""
			a.session.SetLoopDetected(false)
			a.session.AppendToolStart(p.Tool, p.Input, p.CallID)
			a.session.SetToolLoopStep(a.sse.IncrementToolStep())
		}

	case "tool_input_delta":
		var p struct {
			Tool   string `json:"tool"`
			Delta  string `json:"delta"`
			CallID string `json:"call_id"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			a.session.UpdateToolInputDelta(p.Tool, p.Delta, p.CallID)
		}

	case "tool_output":
		var p struct {
			Tool   string `json:"tool"`
			Chunk  string `json:"chunk"`
			CallID string `json:"call_id"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			a.session.UpdateToolOutput(p.Tool, p.Chunk, p.CallID)
		}

	case "tool_end":
		var p struct {
			Tool   string  `json:"tool"`
			Output string  `json:"output"`
			Error  *string `json:"error"`
			CallID string  `json:"call_id"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			toolErr := ""
			if p.Error != nil {
				toolErr = *p.Error
			}
			a.session.UpdateToolEnd(p.Tool, p.Output, toolErr, p.CallID)
			if p.Tool == "task" || p.Tool == "subagent" {
				a.refreshTaskPanel()
			}
		}

	case "subagent_activity":
		var p struct {
			Tool   string `json:"tool"`
			Input  string `json:"input"`
			Output string `json:"output"`
			Done   bool   `json:"done"`
			Error  bool   `json:"error"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			a.session.UpdateSubagentActivity(p.Tool, p.Input, p.Output, p.Done, p.Error)
			if p.Done {
				a.refreshTaskPanel()
			}
		}

	case "question":
		var p sseQuestionPayload
		if err := json.Unmarshal(msg.Data, &p); err == nil && p.QuestionID != "" {
			id := dialogIDPermission + ":" + msg.SessionID + ":" + p.QuestionID
			req := PermissionRequest{ToolName: p.Tool, Arguments: p.Input}
			panel := NewInlinePermissionPanel(id, req, a.session.width, a.theme)
			a.session.SetInlinePanel(panel, panel.PanelHeight())
		}

	case "user_question":
		var p sseUserQuestionPayload
		if err := json.Unmarshal(msg.Data, &p); err == nil && p.QuestionID != "" {
			id := dialogIDUserQuestion + ":" + msg.SessionID + ":" + p.QuestionID
			if p.Purpose != "" {
				id += ":" + p.Purpose
			}
			mode := QuestionSingle
			if p.Multiple {
				mode = QuestionMulti
			}
			opts := make([]QuestionOption, len(p.Options))
			for i, o := range p.Options {
				opts[i] = QuestionOption{Value: o, Label: o}
			}
			panel := NewInlineQuestionPanel(id, p.Question, opts, mode, a.session.width, a.theme)
			a.session.SetInlinePanel(panel, panel.PanelHeight())
		}

	case "title_updated":
		var p struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil && p.Title != "" {
			a.session.SetTitle(p.Title)
		}

	case "active_model":
		var p struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			a.session.SetActiveModel(p.Model)
		}

	case "loop_detected":
		a.session.SetLoopDetected(true)

	case "compaction_start":
		a.session.SetCompacting(true)

	case "compaction":
		a.session.SetCompacting(false)
		var p struct {
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil && p.Summary != "" {
			log.Printf("tui: received compaction event, summary=%d chars, appending system message", len(p.Summary))
			a.session.AppendSystemMessage("Context compacted. Summary:\n" + p.Summary)
			a.session.FlushMessagesRender()
		} else {
			log.Printf("tui: received compaction event but failed to parse: %v", err)
		}

	case "background_event":
		var p backgroundEventPayload
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			a.session.AppendSystemMessage(formatBackgroundEventMessage(p))
			a.session.FlushMessagesRender()
		}

	case "retry", "warning":
		type warnPayload struct{ Message string }
		var p warnPayload
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			a.errorBanner = p.Message
		}

	case "overflow":
		var p struct {
			Dropped  int  `json:"dropped"`
			Critical bool `json:"critical"`
		}
		if err := json.Unmarshal(msg.Data, &p); err == nil && p.Dropped > 0 {
			if p.Critical {
				a.errorBanner = fmt.Sprintf("Stream lag detected: dropped %d events and the view may need a manual refresh.", p.Dropped)
			} else {
				a.errorBanner = fmt.Sprintf("Stream lag detected: dropped %d transient updates.", p.Dropped)
			}
		}

	case "error":
		type errPayload struct{ Message string }
		var p errPayload
		if err := json.Unmarshal(msg.Data, &p); err == nil {
			if isCancellationMessage(p.Message) {
				a.cancelRequested = false
				a.infoBanner = "Turn cancelled."
				a.errorBanner = ""
				if a.route == routeSession {
					a.session.AppendDelta("\n[cancelled]")
				}
			} else {
				a.errorBanner = p.Message
				a.infoBanner = ""
			}
		}
		a.session.FinishStreaming()
		a.sse.MarkDone()
		a.session.SetStreaming(false)
		return a, nil
	}
	return a, nil
}
