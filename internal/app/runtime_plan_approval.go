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
	plannerPlanApprovalQuestion = "What should happen with the completed plan?"
	plannerPlanApprovalSave     = "Save plan"
	plannerPlanApprovalApply    = "Apply plan"
	plannerPlanApprovalRevise   = "Revise plan"
	plannerPlanApprovalStop     = "Stop"
)

func (e *ToolExecutor) preparePlannerSavePlanQuestion(ctx context.Context, state events.SessionState, input ExecuteToolInput) (ExecuteToolInput, error) {
	if !isPlannerSavePlanQuestion(input.ToolName, input.Arguments) {
		return input, nil
	}
	if workflowOwnsPlanApproval(state) {
		return ExecuteToolInput{}, ErrPlannerPlanApprovalDisabledByWorkflow
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

func (e *ToolExecutor) plannerApprovalEnabled() bool {
	return e != nil && e.workflowConfig.PlannerApproval
}

func plannerApprovalPromptFragment() prompt.Fragment {
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "planner-approval-opt-in",
		Label:     "planner approval",
		Content: strings.Join([]string{
			"Planner approval is enabled for this turn.",
			"When you are running as the primary planner and the current repository-grounded implementation plan is complete, first show the complete finished plan to the user in assistant text. Then use the `question` tool to signal that the runtime should ask which action to take.",
			"Do not ask a broader strategy question after producing a plan. Implementation, checklist generation, and \"do nothing\" are not save-plan approval options.",
			"Use exactly these options:",
			"- Save plan",
			"- Revise plan",
			"Use purpose `planner_save_plan`.",
			"Do not ask this question in assistant prose.",
			"The question must not be the first user-visible output for a new plan. The visible plan body is the accepted plan body the runtime will save, apply, or pass back for revision.",
			"If `question` fails with a planner plan-decision contract error, do not re-explore the repository or recreate the plan. Reuse the already-produced plan text and repair only the plan-decision workflow ordering.",
			"If the answer is `Save plan`, `Apply plan`, or `Stop`, the runtime owns the next action. Do not call tools or add prose for those decisions.",
			"If the answer is `Revise plan`, continue revising the plan in the current turn and do not delegate.",
		}, "\n"),
	}
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
	case tool.QuestionToolName:
		return r.answerPrimaryPlannerPlanApproval(ctx, state, input, turnID, request, answer)
	default:
		return RunSessionResult{}, false, nil
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
		return r.continueAfterPlannerPlanSaveAnswer(ctx, state, turnID, plan, initialState)
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

func (r *Runtime) continueAfterPlannerPlanSaveAnswer(ctx context.Context, state events.SessionState, turnID string, plan *events.PlanState, initialState *turnLoopState) (RunSessionResult, bool, error) {
	path := defaultPlannerPlanPath(state, turnID, plan)
	content := ""
	if plan != nil {
		content = plan.Markdown
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

func defaultPlannerPlanPath(state events.SessionState, turnID string, plan *events.PlanState) string {
	base := strings.TrimSpace(state.Title)
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
