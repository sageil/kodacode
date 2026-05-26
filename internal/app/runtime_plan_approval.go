package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

const (
	plannerPlanApprovalQuestion   = "What should happen with the completed plan?"
	plannerPlanApprovalSave       = "Save plan"
	plannerPlanApprovalApply      = "Apply plan"
	plannerPlanApprovalRevise     = "Revise plan"
	plannerPlanApprovalStop       = "Stop"
	handoffKindImplementationPlan = "implementation_plan"
	handoffKindReviewFindings     = "review_findings"
)

func (e *ToolExecutor) requestPlannerPlanApprovalForDelegateResult(ctx context.Context, input ExecuteToolInput, output string) (ToolExecutionResult, bool, error) {
	record, ok := delegateRecordFromToolResult(input.ToolName, output)
	if !ok || record.Status != tool.DelegateStatusCompleted || strings.TrimSpace(record.ChildAgentID) != "planner" {
		return ToolExecutionResult{}, false, nil
	}
	state, err := e.sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return ToolExecutionResult{}, false, err
	}
	_, handoff := findHandoffState(state, record.HandoffID)
	if handoff == nil || strings.TrimSpace(handoff.ChildAgentID) != "planner" || strings.TrimSpace(handoff.AssistantText) == "" {
		return ToolExecutionResult{}, false, nil
	}
	if strings.TrimSpace(handoff.ToolCallID) != strings.TrimSpace(input.ToolCallID) {
		return ToolExecutionResult{}, false, nil
	}
	if !plannerHandoffRequiresPlanApproval(handoff) {
		return ToolExecutionResult{}, false, nil
	}
	if err := appendTextToParentTurn(ctx, e.sessions, input.SessionID, input.TurnID, handoff.AssistantText); err != nil {
		return ToolExecutionResult{}, false, err
	}
	planID, err := ensurePlannerPlanRecorded(ctx, e.sessions, input.SessionID, input.TurnID, handoff)
	if err != nil {
		return ToolExecutionResult{}, false, err
	}
	requestID, err := e.sessions.RequestQuestion(ctx, QuestionRequestInput{
		SessionID:  input.SessionID,
		TurnID:     input.TurnID,
		ToolCallID: input.ToolCallID,
		ToolName:   input.ToolName,
		PlanID:     planID,
		Question:   plannerPlanApprovalQuestion,
		Options:    plannerPlanApprovalOptions(state),
		Purpose:    events.QuestionPurposePlannerPlanDecision,
	})
	if err != nil {
		return ToolExecutionResult{}, false, err
	}
	return ToolExecutionResult{
		Status:             ToolExecutionStatusPending,
		CanonicalArguments: string(input.Arguments),
		PendingRequestID:   requestID,
	}, true, nil
}

func (e *ToolExecutor) preparePlannerSavePlanQuestion(ctx context.Context, state events.SessionState, input ExecuteToolInput) (ExecuteToolInput, error) {
	if !isPlannerSavePlanQuestion(input.ToolName, input.Arguments) {
		return input, nil
	}
	planID, err := ensurePrimaryPlannerPlanRecorded(ctx, e.sessions, state, input.SessionID, input.TurnID)
	if err != nil {
		return ExecuteToolInput{}, err
	}
	arguments, err := plannerPlanApprovalQuestionArguments(state)
	if err != nil {
		return ExecuteToolInput{}, err
	}
	input.PlanID = planID
	input.Arguments = arguments
	return input, nil
}

func plannerPlanApprovalQuestionArguments(state events.SessionState) (json.RawMessage, error) {
	encoded, err := json.Marshal(struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
		Purpose  string   `json:"purpose"`
	}{
		Question: plannerPlanApprovalQuestion,
		Options:  plannerPlanApprovalOptions(state),
		Purpose:  events.QuestionPurposePlannerPlanDecision,
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func appendTextToParentTurn(ctx context.Context, sessions *SessionService, sessionID, turnID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	state, err := sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	existing := ""
	if turn := state.Turns[turnID]; turn != nil {
		existing = strings.TrimSpace(turn.AssistantText)
	}
	next := text
	if existing != "" {
		if strings.Contains(existing, text) {
			return nil
		}
		next = strings.TrimRight(existing, "\n") + "\n\n" + text
	}
	_, err = sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeAssistantCommit,
		Payload: events.AssistantCommitPayload{
			Content: next,
		},
	})
	return err
}

func isPlannerPlanApprovalQuestion(request *events.QuestionRequestState) bool {
	return request != nil && strings.TrimSpace(request.Purpose) == events.QuestionPurposePlannerPlanDecision
}

func plannerHandoffRequiresPlanApproval(handoff *events.AgentHandoffState) bool {
	return handoffProvidesKind(handoff, handoffKindImplementationPlan)
}

func plannerPlanForHandoff(state events.SessionState, handoff *events.AgentHandoffState) *events.PlanState {
	if handoff == nil {
		return nil
	}
	for _, planID := range state.PlanOrder {
		plan := state.Plans[planID]
		if plan != nil && strings.TrimSpace(plan.SourceHandoffID) == strings.TrimSpace(handoff.HandoffID) {
			return plan
		}
	}
	return nil
}

func plannerPlanForTurn(state events.SessionState, turnID string) *events.PlanState {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return nil
	}
	for _, planID := range state.PlanOrder {
		plan := state.Plans[planID]
		if plan != nil && strings.TrimSpace(plan.SourceTurnID) == turnID {
			return plan
		}
	}
	return nil
}

func planStateFromHandoff(handoff *events.AgentHandoffState) *events.PlanState {
	if handoff == nil {
		return nil
	}
	return &events.PlanState{
		SourceHandoffID: strings.TrimSpace(handoff.HandoffID),
		Title:           plannerPlanTitle(handoff),
		Markdown:        strings.TrimRight(handoff.AssistantText, "\n"),
		CreatedByAgent:  strings.TrimSpace(handoff.ChildAgentID),
	}
}

func planStateFromTurn(turn *events.TurnState, turnID string) *events.PlanState {
	if turn == nil {
		return nil
	}
	return &events.PlanState{
		SourceTurnID:   strings.TrimSpace(turnID),
		Title:          plannerPlanTitleFromText(turn.AssistantText, turn.UserText),
		Markdown:       strings.TrimRight(turn.AssistantText, "\n"),
		CreatedByAgent: "planner",
	}
}

func ensurePlannerPlanRecorded(ctx context.Context, sessions *SessionService, sessionID, turnID string, handoff *events.AgentHandoffState) (string, error) {
	if handoff == nil {
		return "", ErrHandoffNotFound
	}
	state, err := sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}
	for _, planID := range state.PlanOrder {
		plan := state.Plans[planID]
		if plan != nil && strings.TrimSpace(plan.SourceHandoffID) == strings.TrimSpace(handoff.HandoffID) {
			return strings.TrimSpace(plan.PlanID), nil
		}
	}
	planID := newRuntimeID("plan")
	_, err = sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypePlanRecorded,
		Payload: events.PlanRecordedPayload{
			PlanID:          planID,
			SourceHandoffID: strings.TrimSpace(handoff.HandoffID),
			Title:           plannerPlanTitle(handoff),
			Markdown:        strings.TrimRight(handoff.AssistantText, "\n"),
			CreatedByAgent:  strings.TrimSpace(handoff.ChildAgentID),
		},
	})
	return planID, err
}

func ensurePrimaryPlannerPlanRecorded(ctx context.Context, sessions *SessionService, state events.SessionState, sessionID, turnID string) (string, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return "", ErrTurnIDRequired
	}
	for _, planID := range state.PlanOrder {
		plan := state.Plans[planID]
		if plan != nil && strings.TrimSpace(plan.SourceTurnID) == turnID {
			return strings.TrimSpace(plan.PlanID), nil
		}
	}
	turn := state.Turns[turnID]
	if turn == nil || strings.TrimSpace(turn.AssistantText) == "" {
		return "", ErrPlannerSavePlanQuestionRequiresVisiblePlan
	}
	planID := newRuntimeID("plan")
	_, err := sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypePlanRecorded,
		Payload: events.PlanRecordedPayload{
			PlanID:         planID,
			SourceTurnID:   turnID,
			Title:          plannerPlanTitleFromText(turn.AssistantText, turn.UserText),
			Markdown:       strings.TrimRight(turn.AssistantText, "\n"),
			CreatedByAgent: "planner",
		},
	})
	return planID, err
}

func plannerPlanTitle(handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return "Implementation Plan"
	}
	return plannerPlanTitleFromText(handoff.AssistantText, handoff.Task)
}

func plannerPlanTitleFromText(markdown, fallback string) string {
	if title := firstMarkdownHeading(markdown); title != "" {
		return title
	}
	if fallback = strings.TrimSpace(fallback); fallback != "" {
		return fallback
	}
	return "Implementation Plan"
}

func firstMarkdownHeading(markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if title != "" {
			return title
		}
	}
	return ""
}

func (r *Runtime) answerPlannerPlanApproval(ctx context.Context, state events.SessionState, input AnswerSessionQuestionInput, turnID string, request *events.QuestionRequestState, answer string) (RunSessionResult, bool, error) {
	if !isPlannerPlanApprovalQuestion(request) {
		return RunSessionResult{}, false, nil
	}
	switch strings.TrimSpace(request.ToolName) {
	case tool.DelegateToolName:
		return r.answerDelegatedPlannerPlanApproval(ctx, state, input, turnID, request, answer)
	case tool.QuestionToolName:
		return r.answerPrimaryPlannerPlanApproval(ctx, state, input, turnID, request, answer)
	default:
		return RunSessionResult{}, false, nil
	}
}

func (r *Runtime) answerDelegatedPlannerPlanApproval(ctx context.Context, state events.SessionState, input AnswerSessionQuestionInput, turnID string, request *events.QuestionRequestState, answer string) (RunSessionResult, bool, error) {
	answer = strings.TrimSpace(answer)
	if !plannerPlanApprovalAnswerAllowed(request, answer) {
		return RunSessionResult{}, true, ErrQuestionAnswerInvalid
	}
	handoff := plannerHandoffForToolCall(state.Turns[turnID], request.ToolCallID)
	if handoff == nil {
		return RunSessionResult{}, true, ErrHandoffNotFound
	}
	initialState, err := buildTurnRolloverInitialState(state.Turns[turnID])
	if err != nil {
		return RunSessionResult{}, true, err
	}
	if _, err := r.Sessions.AnswerQuestion(ctx, AnswerQuestionInput{
		SessionID: state.SessionID,
		TurnID:    turnID,
		RequestID: request.QuestionID,
		Answer:    answer,
	}); err != nil {
		return RunSessionResult{}, true, err
	}
	output, err := plannerDelegateRecordOutput(handoff)
	if err != nil {
		return RunSessionResult{}, true, err
	}
	if err := appendPlannerDelegateToolResult(ctx, r.Sessions, state.SessionID, turnID, request, handoff, output); err != nil {
		return RunSessionResult{}, true, err
	}
	if err := r.Runner.appendTurnDone(ctx, state.SessionID, turnID); err != nil {
		return RunSessionResult{}, true, err
	}
	plan := plannerPlanForHandoff(state, handoff)
	if plan == nil {
		plan = planStateFromHandoff(handoff)
	}

	switch answer {
	case plannerPlanApprovalSave:
		return r.continueAfterPlannerPlanSaveAnswer(ctx, state, turnID, handoff, plan, initialState)
	case plannerPlanApprovalApply:
		return r.continueAfterPlannerPlanApplyAnswer(ctx, state, input, turnID, request, plan, initialState)
	case plannerPlanApprovalRevise:
		return r.continueAfterPlannerPlanRevisionAnswer(ctx, state, input, turnID, request, plan, initialState)
	case plannerPlanApprovalStop:
		return r.continueAfterPlannerPlanStopAnswer(ctx, state, turnID, initialState)
	default:
		return RunSessionResult{}, true, ErrQuestionAnswerInvalid
	}
}

func (r *Runtime) answerPrimaryPlannerPlanApproval(ctx context.Context, state events.SessionState, input AnswerSessionQuestionInput, turnID string, request *events.QuestionRequestState, answer string) (RunSessionResult, bool, error) {
	answer = strings.TrimSpace(answer)
	if !plannerPlanApprovalAnswerAllowed(request, answer) {
		return RunSessionResult{}, true, ErrQuestionAnswerInvalid
	}
	initialState, err := buildTurnRolloverInitialState(state.Turns[turnID])
	if err != nil {
		return RunSessionResult{}, true, err
	}
	plan := plannerPlanForTurn(state, turnID)
	if plan == nil {
		plan = planStateFromTurn(state.Turns[turnID], turnID)
	}
	if _, err := r.Sessions.AnswerQuestion(ctx, AnswerQuestionInput{
		SessionID: state.SessionID,
		TurnID:    turnID,
		RequestID: request.QuestionID,
		Answer:    answer,
	}); err != nil {
		return RunSessionResult{}, true, err
	}
	if err := r.completeAnsweredQuestionSourceTurn(ctx, state.SessionID, turnID, *request, answer); err != nil {
		return RunSessionResult{}, true, err
	}

	switch answer {
	case plannerPlanApprovalSave:
		return r.continueAfterPlannerPlanSaveAnswer(ctx, state, turnID, nil, plan, initialState)
	case plannerPlanApprovalApply:
		return r.continueAfterPlannerPlanApplyAnswer(ctx, state, input, turnID, request, plan, initialState)
	case plannerPlanApprovalRevise:
		return r.continueAfterPlannerPlanRevisionAnswer(ctx, state, input, turnID, request, plan, initialState)
	case plannerPlanApprovalStop:
		return r.continueAfterPlannerPlanStopAnswer(ctx, state, turnID, initialState)
	default:
		return RunSessionResult{}, true, ErrQuestionAnswerInvalid
	}
}

func plannerPlanApprovalOptions(state events.SessionState) []string {
	options := []string{plannerPlanApprovalSave}
	if statePermissionMode(state, PermissionModeAuto) != PermissionModeReadOnly {
		options = append(options, plannerPlanApprovalApply)
	}
	options = append(options, plannerPlanApprovalRevise, plannerPlanApprovalStop)
	return options
}

func plannerPlanApprovalAnswerAllowed(request *events.QuestionRequestState, answer string) bool {
	answer = strings.TrimSpace(answer)
	if request == nil || answer == "" {
		return false
	}
	for _, option := range request.Options {
		if strings.TrimSpace(option) == answer {
			return true
		}
	}
	return false
}

func appendPlannerDelegateToolResult(ctx context.Context, sessions *SessionService, sessionID, turnID string, request *events.QuestionRequestState, handoff *events.AgentHandoffState, output string) error {
	_, err := sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeToolExecEnd,
		Payload: events.ToolExecEndPayload{
			Succeeded: true,
			CallID:    request.ToolCallID,
			ToolName:  request.ToolName,
			HandoffID: handoff.HandoffID,
			Output:    output,
		},
	})
	return err
}

func (r *Runtime) continueAfterPlannerPlanSaveAnswer(ctx context.Context, state events.SessionState, turnID string, handoff *events.AgentHandoffState, plan *events.PlanState, initialState *turnLoopState) (RunSessionResult, bool, error) {
	path := defaultPlannerPlanPath(state, turnID, handoff, plan)
	content := ""
	if plan != nil {
		content = plan.Markdown
	}
	if strings.TrimSpace(content) == "" && handoff != nil {
		content = handoff.AssistantText
	}
	if err := writeRuntimePlanFile(state, path, content); err != nil {
		return RunSessionResult{}, true, err
	}
	body := "Saved plan to `" + path + "`."
	return r.completePlannerPlanRuntimeDecisionTurn(ctx, state, turnID, plannerPlanApprovalSave, body, initialState)
}

func (r *Runtime) continueAfterPlannerPlanStopAnswer(ctx context.Context, state events.SessionState, turnID string, initialState *turnLoopState) (RunSessionResult, bool, error) {
	body := "Stopped. The plan was not saved or applied."
	return r.completePlannerPlanRuntimeDecisionTurn(ctx, state, turnID, plannerPlanApprovalStop, body, initialState)
}

func (r *Runtime) completePlannerPlanRuntimeDecisionTurn(ctx context.Context, state events.SessionState, previousTurnID string, userText, assistantText string, initialState *turnLoopState) (RunSessionResult, bool, error) {
	nextTurnID := newRuntimeID("turn")
	summary := turnWorkSummary{}
	if initialState != nil {
		summary = initialState.WorkState.Summary
	}
	if err := r.Runner.appendTurnContinuationStarted(ctx, state.SessionID, nextTurnID, previousTurnID, events.TurnContinuationReasonQuestionAnswer, summary); err != nil {
		return RunSessionResult{}, true, err
	}
	if err := r.Runner.appendUserMessage(ctx, state.SessionID, nextTurnID, userText, nil); err != nil {
		return RunSessionResult{}, true, err
	}
	if err := r.Runner.appendAssistantCommit(ctx, state.SessionID, nextTurnID, assistantText); err != nil {
		return RunSessionResult{}, true, err
	}
	if err := r.Runner.appendTurnDone(ctx, state.SessionID, nextTurnID); err != nil {
		return RunSessionResult{}, true, err
	}
	result, err := r.loadSessionTurnResult(ctx, state.SessionID, nextTurnID, RunTurnResult{Status: TurnRunStatusCompleted})
	return result, true, err
}

func (r *Runtime) continueAfterPlannerPlanApplyAnswer(ctx context.Context, state events.SessionState, input AnswerSessionQuestionInput, turnID string, request *events.QuestionRequestState, plan *events.PlanState, initialState *turnLoopState) (RunSessionResult, bool, error) {
	engineerDefinition, err := r.resolveTurnAgent(state.WorkspaceRoot, "engineer")
	if err != nil {
		return RunSessionResult{}, true, err
	}
	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		SessionID:    state.SessionID,
		TurnID:       turnID,
		AgentID:      "engineer",
		SkillIDs:     append([]string(nil), input.SkillIDs...),
		AllowedTools: r.allowedToolsForTurn(state, engineerDefinition),
	})
	if err != nil {
		return RunSessionResult{}, true, err
	}
	nextTurnID := newRuntimeID("turn")
	result, err := r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:            state.SessionID,
		TurnID:               nextTurnID,
		UserText:             plannerPlanApprovalApply,
		ResolvedAttachments:  cloneProviderAttachments(resolved.attachments),
		AgentID:              resolved.agentID,
		SkillIDs:             append([]string(nil), resolved.skillIDs...),
		ThinkingEnabled:      resolved.thinkingEnabled,
		ThinkingMode:         resolved.thinkingMode,
		AdditionalFragments:  []prompt.Fragment{plannerPlanRuntimeFragment(plan, "Apply the approved plan. Implement the requested changes and verify the result.")},
		AllowedToolsOverride: append([]string(nil), resolved.allowedTools...),
		ModelRouteOverride:   resolved.modelRoute,
		PreserveSessionModel: true,
		InitialState:         initialState,
		Continuation: &runtimeTurnContinuation{
			PreviousTurnID: turnID,
			Reason:         events.TurnContinuationReasonQuestionAnswer,
			Question:       request.Question,
		},
	})
	return result, true, err
}

func (r *Runtime) continueAfterPlannerPlanRevisionAnswer(ctx context.Context, state events.SessionState, input AnswerSessionQuestionInput, turnID string, request *events.QuestionRequestState, plan *events.PlanState, initialState *turnLoopState) (RunSessionResult, bool, error) {
	plannerDefinition, err := r.resolveTurnAgent(state.WorkspaceRoot, "planner")
	if err != nil {
		return RunSessionResult{}, true, err
	}
	allowedTools := r.allowedToolsForTurn(state, plannerDefinition)
	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		SessionID:    state.SessionID,
		TurnID:       turnID,
		AgentID:      "planner",
		SkillIDs:     append([]string(nil), input.SkillIDs...),
		AllowedTools: allowedTools,
	})
	if err != nil {
		return RunSessionResult{}, true, err
	}
	hideAssistantPreview := false
	if turn := state.Turns[turnID]; turn != nil && turn.Config != nil {
		hideAssistantPreview = turn.Config.HideAssistantPreview
	}
	nextTurnID := newRuntimeID("turn")
	result, err := r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:            state.SessionID,
		TurnID:               nextTurnID,
		UserText:             plannerPlanApprovalRevise,
		ResolvedAttachments:  cloneProviderAttachments(resolved.attachments),
		AgentID:              resolved.agentID,
		SkillIDs:             append([]string(nil), resolved.skillIDs...),
		ThinkingEnabled:      resolved.thinkingEnabled,
		ThinkingMode:         resolved.thinkingMode,
		Fragments:            input.Fragments,
		AdditionalFragments:  []prompt.Fragment{plannerPlanRuntimeFragment(plan, "Revise the approved plan. Show the complete revised plan, then use the planner save-plan question signal so the runtime can ask whether to save, apply, revise again, or stop.")},
		AllowedToolsOverride: append([]string(nil), resolved.allowedTools...),
		ModelRouteOverride:   resolved.modelRoute,
		PreserveSessionModel: true,
		HideAssistantPreview: hideAssistantPreview,
		InitialState:         initialState,
		Continuation: &runtimeTurnContinuation{
			PreviousTurnID: turnID,
			Reason:         events.TurnContinuationReasonQuestionAnswer,
			Question:       request.Question,
		},
	})
	return result, true, err
}

func plannerPlanRuntimeFragment(plan *events.PlanState, instruction string) prompt.Fragment {
	lines := []string{
		strings.TrimSpace(instruction),
		"",
		"Approved plan source:",
	}
	if plan != nil {
		if id := strings.TrimSpace(plan.PlanID); id != "" {
			lines = append(lines, "- Plan ID: "+id)
		}
		if id := strings.TrimSpace(plan.SourceHandoffID); id != "" {
			lines = append(lines, "- Source handoff ID: "+id)
		}
		if id := strings.TrimSpace(plan.SourceTurnID); id != "" {
			lines = append(lines, "- Source turn ID: "+id)
		}
		if title := strings.TrimSpace(plan.Title); title != "" {
			lines = append(lines, "- Title: "+title)
		}
		if markdown := strings.TrimSpace(plan.Markdown); markdown != "" {
			lines = append(lines, "", "Plan:", markdown)
		}
	}
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "planner-plan-approval",
		Label:     "planner-plan-approval",
		Content:   strings.Join(lines, "\n"),
	}
}

func plannerHandoffForToolCall(turn *events.TurnState, toolCallID string) *events.AgentHandoffState {
	if turn == nil {
		return nil
	}
	toolCallID = strings.TrimSpace(toolCallID)
	for _, handoffID := range turn.HandoffOrder {
		handoff := turn.Handoffs[handoffID]
		if handoff != nil && strings.TrimSpace(handoff.ToolCallID) == toolCallID && strings.TrimSpace(handoff.ChildAgentID) == "planner" {
			return handoff
		}
	}
	return nil
}

func plannerDelegateRecordOutput(handoff *events.AgentHandoffState) (string, error) {
	record := tool.DelegateRecord{
		HandoffID:      strings.TrimSpace(handoff.HandoffID),
		ChildSessionID: strings.TrimSpace(handoff.ChildSessionID),
		ChildTurnID:    strings.TrimSpace(handoff.ChildTurnID),
		ChildAgentID:   strings.TrimSpace(handoff.ChildAgentID),
		Status:         tool.DelegateStatusCompleted,
		AssistantText:  strings.TrimSpace(handoff.AssistantText),
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func defaultPlannerPlanPath(state events.SessionState, turnID string, handoff *events.AgentHandoffState, plan *events.PlanState) string {
	base := strings.TrimSpace(state.Title)
	if base == "" && handoff != nil {
		base = strings.TrimSpace(handoff.Task)
	}
	if base == "" && plan != nil {
		base = strings.TrimSpace(plan.Title)
	}
	if base == "" {
		base = strings.TrimSpace(turnID)
	}
	slug := slugPlanFilename(base)
	if slug == "" {
		slug = "implementation-plan"
	}
	if !strings.HasSuffix(slug, "-plan") {
		slug += "-plan"
	}
	return ".kodacode/plans/" + slug + ".md"
}

func slugPlanFilename(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out []rune
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			out = append(out, r)
			lastDash = false
		case !lastDash:
			out = append(out, '-')
			lastDash = true
		}
	}
	return strings.Trim(string(out), "-")
}

func writeRuntimePlanFile(state events.SessionState, path, content string) error {
	scope, err := workspace.New(state.WorkspaceRoot)
	if err != nil {
		return err
	}
	decision, err := scope.Authorize(workspace.AccessWrite, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(decision.ResolvedPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(decision.ResolvedPath, []byte(strings.TrimRight(content, "\n")+"\n"), 0o644)
}
