package tui

import (
	"fmt"
	"log"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func parseUserQuestionDialogID(id string) (sessionID, questionID, purpose string, ok bool) {
	rest, ok := strings.CutPrefix(id, dialogIDUserQuestion+":")
	if !ok {
		return "", "", "", false
	}
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) < 2 {
		return "", "", "", false
	}
	sessionID = parts[0]
	questionID = parts[1]
	if len(parts) == 3 {
		purpose = parts[2]
	}
	return sessionID, questionID, purpose, true
}

func (a App) completePlannerApprovalAfterDone() (tea.Model, tea.Cmd) {
	choice := a.cfg.PlannerChoice
	a.cfg.PlannerPending = false
	a.cfg.PlannerChoice = ""

	restoreID := a.cfg.PreplanAgent
	if a.session.header.agentID != "planner" || restoreID == "" {
		return a, nil
	}

	persistCmd := tea.Cmd(nil)
	if a.switchAgent(restoreID) {
		persistCmd = a.scheduleAgentPersistence(restoreID)
	}
	api := a.api
	ctx := a.ctx
	sessionID := a.sessionID

	switch choice {
	case planApprovalSaveLabel, planApprovalGoLabel:
		a.session.AppendSystemMessage("Plan approved. Executing with " + a.cfg.AgentNames[restoreID] + " agent...")
		a.session.FlushMessagesRender()
		execMsg := "The plan above has been approved. Execute it now, following each step in order. Complete one task at a time, but continue through the remaining tasks without asking the user for confirmation between tasks unless blocked. Report progress as you go."
		if choice == planApprovalSaveLabel {
			execMsg = "The plan above has been approved. Save the plan chunks to docs/kodacode/plans/{YYYY-MM-DD}-{plan-name}-part{N}.md, one chunk per file, then execute the approved task list. Complete one task at a time, but continue through the remaining tasks without asking the user for confirmation between tasks unless blocked. Report progress as you go."
		}
		execCmd := func() tea.Msg {
			if err := api.SendMessage(ctx, sessionID, execMsg, nil, restoreID); err != nil {
				return SSEErrorMsg{SessionID: sessionID, Err: fmt.Errorf("send plan execution: %w", err)}
			}
			return messageSentMsg{sessionID: sessionID, text: ""}
		}
		return a, tea.Batch(execCmd, persistCmd)
	case planApprovalRejectLabel:
		a.session.AppendSystemMessage("Plan rejected.")
	default:
		a.session.AppendSystemMessage("Planning cancelled.")
	}
	a.session.FlushMessagesRender()
	updateCmd := func() tea.Msg {
		_ = api.UpdateSessionAgent(ctx, sessionID, restoreID)
		return nil
	}
	return a, tea.Batch(updateCmd, persistCmd)
}

// modelInfoString builds a compact metadata string for the header.
// Omits "plan" for subscription models; always includes capabilities.
func modelInfoString(item ModelItem) string {
	var parts []string
	if item.ContextSize > 0 {
		ctx := formatContextSize(item.ContextSize)
		if item.MaxInputTokens > 0 && item.MaxInputTokens < item.ContextSize {
			outputTokens := item.ContextSize - item.MaxInputTokens
			parts = append(parts, ctx+" · "+formatContextSize(item.MaxInputTokens)+"↑ · "+formatContextSize(outputTokens)+"↓")
		} else {
			parts = append(parts, ctx)
		}
	}
	// Only show pricing if it's pay-per-token (not subscription/plan).
	if item.CostInput > 0 || item.CostOutput > 0 {
		parts = append(parts, formatPrice(item.CostInput, item.CostOutput))
	}
	var caps []string
	if item.Reasoning {
		caps = append(caps, "✓R")
	}
	if item.ToolCall {
		caps = append(caps, "✓T")
	}
	if item.Vision {
		caps = append(caps, "✓V")
	}
	if len(caps) > 0 {
		parts = append(parts, strings.Join(caps, " "))
	}
	return strings.Join(parts, "  ")
}

func buildModelItems(providers []APIProviderModels) []ModelItem {
	var items []ModelItem
	for _, p := range providers {
		for _, m := range p.Models {
			name := m.Name
			if name == "" {
				name = m.ID
			}
			items = append(items, ModelItem{
				ProviderID:     p.ProviderID,
				ProviderName:   p.ProviderName,
				ModelID:        m.ID,
				ModelName:      name,
				ContextSize:    m.ContextSize,
				MaxInputTokens: m.MaxInputTokens,
				Reasoning:      m.Reasoning,
				ToolCall:       m.ToolCall,
				Vision:         m.Vision,
				CostInput:      m.CostInput,
				CostOutput:     m.CostOutput,
			})
		}
	}
	return items
}

func (a App) handleDialogClosed(msg dialogClosedMsg) (tea.Model, tea.Cmd) {
	var questionPrompt string
	if strings.HasPrefix(msg.id, dialogIDUserQuestion+":") {
		questionPrompt = a.session.InlinePanelQuestionPrompt()
	}

	if strings.HasPrefix(msg.id, dialogIDPermission+":") || strings.HasPrefix(msg.id, dialogIDUserQuestion+":") {
		a.session.ClearInlinePanel()
	}
	if len(a.dialogs) > 0 {
		a.dialogs = a.dialogs[:len(a.dialogs)-1]
	}

	if msg.result == nil {
		// Cancelled. For most dialogs there is nothing to do, but blocking dialogs
		// (permission, user_question) must send a response to unblock the server,
		// and then re-pump the SSE connection so subsequent events are delivered.
		if rest, ok := strings.CutPrefix(msg.id, dialogIDPermission+":"); ok {
			if sessionID, questionID, ok := strings.Cut(rest, ":"); ok {
				api := a.api
				ctx := a.ctx
				return a, func() tea.Msg {
					if err := api.AnswerQuestion(ctx, sessionID, questionID, "reject"); err != nil {
						log.Printf("tui: answer permission question %s failed: %v", questionID, err)
					}
					return nil
				}
			}
		}
		if sessionID, questionID, purpose, ok := parseUserQuestionDialogID(msg.id); ok {
			if purpose == planApprovalPurpose && a.session.header.agentID == "planner" && a.cfg.PreplanAgent != "" {
				a.cfg.PlannerPending = true
				a.cfg.PlannerChoice = ""
			}
			api := a.api
			ctx := a.ctx
			return a, func() tea.Msg {
				if err := api.AnswerQuestion(ctx, sessionID, questionID, ""); err != nil {
					log.Printf("tui: answer user question %s (cancel) failed: %v", questionID, err)
				}
				return nil
			}
		}
		return a, nil
	}

	switch msg.id {
	case dialogIDAgent:
		return a.handleAgentDialogResult(msg.result)
	case dialogIDSession:
		return a.handleSessionDialogResult(msg.result)
	case dialogIDModel:
		return a.handleModelDialogResult(msg.result)
	case dialogIDTheme:
		return a.handleThemeDialogResult(msg.result)
	case dialogIDReplay:
		return a.handleReplayDialogResult(msg.result)
	case dialogIDPalette:
		return a.handlePaletteResult(msg.result)
	case dialogIDProvider:
		return a.handleProviderDialogResult(msg.result)
	default:
		return a.handleDynamicDialogResult(msg.id, msg.result, questionPrompt)
	}
}
