package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
)

type AnswerSessionQuestionInput struct {
	SessionID    string
	TurnID       string
	RequestID    string
	Answer       string
	UserText     string
	AgentID      string
	SkillIDs     []string
	AllowedTools []string
	Fragments    []prompt.Fragment
}

func (r *Runtime) AnswerSessionQuestion(ctx context.Context, input AnswerSessionQuestionInput) (RunSessionResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return RunSessionResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return RunSessionResult{}, ErrTurnIDRequired
	}
	if strings.TrimSpace(input.RequestID) == "" {
		return RunSessionResult{}, ErrQuestionRequestMissing
	}

	state, err := r.Sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	request := pendingQuestionRequestState(state, input.RequestID)
	if request == nil {
		return RunSessionResult{}, ErrQuestionRequestMissing
	}
	turnID := pendingInteractionTurnID(state, input.RequestID)
	if turnID == "" {
		turnID = strings.TrimSpace(input.TurnID)
	}
	loopQuestion := isLoopResolutionQuestion(request)
	answer := strings.TrimSpace(input.Answer)
	activeTask := activeWorkflowTask(state)
	if result, handled, err := r.answerPlannerPlanApproval(ctx, state, input, turnID, request, answer); handled || err != nil {
		return result, err
	}
	if result, handled, err := r.answerWorkflowApproval(ctx, state, input, turnID, request, answer); handled || err != nil {
		return result, err
	}

	if _, err := r.Sessions.AnswerQuestion(ctx, AnswerQuestionInput{
		SessionID: input.SessionID,
		TurnID:    turnID,
		RequestID: input.RequestID,
		Answer:    input.Answer,
	}); err != nil {
		r.log("runtime").Error("question answer failed", err,
			"session_id", input.SessionID,
			"turn_id", turnID,
			"request_id", input.RequestID,
		)
		return RunSessionResult{}, err
	}
	if loopQuestion {
		switch answer {
		case loopResolutionAnswerStop:
			if err := r.Runner.appendTurnCanceled(ctx, input.SessionID, turnID, ErrTurnCanceledByUser); err != nil {
				return RunSessionResult{}, err
			}
			return r.loadSessionTurnResult(ctx, input.SessionID, turnID, RunTurnResult{Status: TurnRunStatusCanceled})
		case providerRequestLimitAnswerAllowSessionYOLO:
			if !providerRequestLimitAllowsSessionDisable(state, turnID) || !questionOptionAllowed(request.Options, providerRequestLimitAnswerAllowSessionYOLO) {
				return RunSessionResult{}, ErrQuestionAnswerInvalid
			}
			if err := r.Sessions.SetProviderRequestLimitDisabled(ctx, input.SessionID, turnID, true); err != nil {
				return RunSessionResult{}, err
			}
		case loopResolutionAnswerBlock:
			if activeTask != nil {
				if _, err := r.Sessions.BlockTask(ctx, BlockTaskInput{
					SessionID:   input.SessionID,
					TurnID:      turnID,
					TaskID:      activeTask.TaskID,
					BlockReason: taskWorkflowBlockReasonForCode(activeTask, events.TurnFailureCodeNoProgress),
				}); err != nil {
					return RunSessionResult{}, err
				}
			}
			if err := r.Runner.appendTurnCanceled(ctx, input.SessionID, turnID, ErrTurnCanceledByUser); err != nil {
				return RunSessionResult{}, err
			}
			return r.loadSessionTurnResult(ctx, input.SessionID, turnID, RunTurnResult{Status: TurnRunStatusCanceled})
		}
	}

	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		SessionID:    input.SessionID,
		TurnID:       turnID,
		AgentID:      input.AgentID,
		SkillIDs:     append([]string(nil), input.SkillIDs...),
		AllowedTools: slices.Clone(input.AllowedTools),
	})
	if err != nil {
		return RunSessionResult{}, err
	}
	result, err := r.resumeAnsweredQuestionTurn(ctx, state, input, turnID, resolved, answer)
	if err != nil {
		return RunSessionResult{}, err
	}
	r.log("runtime").Op("question answered",
		"session_id", result.SessionID,
		"turn_id", result.TurnID,
		"status", result.Status,
		"pending_request_id", result.PendingRequestID,
	)
	return result, nil
}

func (r *Runtime) resumeAnsweredQuestionTurn(ctx context.Context, state events.SessionState, input AnswerSessionQuestionInput, turnID string, resolved resolvedResumeTurn, answer string) (RunSessionResult, error) {
	mcpState, err := r.syncSessionMCPState(ctx, input.SessionID, state.WorkspaceRoot, state.MCP)
	if err != nil {
		return RunSessionResult{}, err
	}
	availableSkills, err := r.availableTurnSkills(state.WorkspaceRoot)
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, turnID, resolved.userText, resolved.attachments, err)
	}
	effectiveSkillIDs := skillIDsForTurn(resolved.userText, resolved.skillIDs, availableSkills)
	capabilities, err := r.resolveTurnCapabilitiesFromState(state, resolveTurnCapabilitiesOptions{
		AgentID:              resolved.agentID,
		SkillIDs:             append([]string(nil), effectiveSkillIDs...),
		ModelRouteOverride:   resolved.modelRoute,
		AllowedToolsOverride: slices.Clone(resolved.allowedTools),
		StrictModelRoute:     true,
	})
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, turnID, resolved.userText, resolved.attachments, err)
	}
	responseStyle := resolved.responseStyle
	if responseStyle == "" {
		responseStyle = r.Config.Sessions.EffectiveResponseStyle()
	}
	fragments := input.Fragments
	if len(fragments) == 0 && strings.TrimSpace(resolved.instructions) == "" {
		fragments, err = defaultTurnFragments(capabilities.definition, state.WorkspaceRoot, state.AdditionalWorkspaceRoots, availableSkills, capabilities.selectedSkills, capabilities.AllowedTools, mcpState, r.Memory, responseStyle, r.Config.Execution, collectCompactedInspectionProgressPromptEntries(&state))
		if err != nil {
			return r.recordTurnFailure(ctx, input.SessionID, turnID, resolved.userText, resolved.attachments, err)
		}
	}
	effectiveThinkingMode := capabilities.EffectiveReasoningVariant(resolved.thinkingMode)
	effectiveThinkingEnabled := capabilities.EffectiveThinkingEnabled(resolved.thinkingEnabled)
	if !resolved.preserveSessionModel {
		if err := r.syncSessionModelRoute(ctx, input.SessionID, capabilities.ModelRoute); err != nil {
			return RunSessionResult{}, err
		}
	}

	turnCtx, turnHandle, finishTurn, err := r.beginCancelableTurnContext(ctx, input.SessionID, turnID)
	if err != nil {
		return RunSessionResult{}, err
	}
	finishedTurn := false
	defer func() {
		if !finishedTurn {
			finishTurn()
		}
	}()

	result, err := r.Runner.Resume(turnCtx, ResumeTurnInput{
		SessionID:                   input.SessionID,
		TurnID:                      turnID,
		AgentID:                     capabilities.AgentID,
		UserText:                    resolved.userText,
		QuestionAnswer:              answer,
		Attachments:                 resolved.attachments,
		Fragments:                   fragments,
		Instructions:                resolved.instructions,
		CacheablePrefix:             resolved.cacheablePrefix,
		DynamicSuffix:               resolved.dynamicSuffix,
		PromptCompactionTokensSaved: resolved.promptCompactionTokensSaved,
		ModelRoute:                  capabilities.ModelRoute,
		ThinkingSupported:           capabilities.SupportsThinkingOutput(),
		ThinkingEnabled:             effectiveThinkingEnabled,
		ThinkingMode:                effectiveThinkingMode,
		AllowedTools:                capabilities.AllowedTools,
		RequestID:                   input.RequestID,
	})
	if turnHandle.canceled() {
		return r.loadCanceledSessionTurnResult(input.SessionID, turnID, resolved.userText, resolved.attachments)
	}
	if err != nil {
		if failed, ok, failedErr := r.failedSessionTurnResult(ctx, input.SessionID, turnID); failedErr == nil && ok {
			return failed, nil
		}
		return RunSessionResult{}, err
	}
	if result.Status == TurnRunStatusRolled {
		hideAssistantPreview := false
		if turn := state.Turns[turnID]; turn != nil && turn.Config != nil {
			hideAssistantPreview = turn.Config.HideAssistantPreview
		}
		finishTurn()
		finishedTurn = true
		return r.continueRolledOverResumedTurn(ctx, input.SessionID, turnID, resolved, capabilities, effectiveThinkingEnabled, effectiveThinkingMode, fragments, hideAssistantPreview, false)
	}
	return r.finalizeTurnRunResult(ctx, turnCtx, turnHandle, input.SessionID, turnID, result)
}

func (r *Runtime) completeAnsweredQuestionSourceTurn(ctx context.Context, sessionID, turnID string, request events.QuestionRequestState, answer string) error {
	if strings.TrimSpace(request.ToolCallID) != "" {
		output, err := json.Marshal(struct {
			Answer string `json:"answer"`
		}{
			Answer: strings.TrimSpace(answer),
		})
		if err != nil {
			return err
		}
		if _, err := r.Sessions.append(ctx, events.Draft{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeToolExecEnd,
			Payload: events.ToolExecEndPayload{
				Succeeded: true,
				CallID:    request.ToolCallID,
				ToolName:  request.ToolName,
				Output:    string(output),
			},
		}); err != nil {
			return err
		}
	}
	return r.Runner.appendTurnDone(ctx, sessionID, turnID)
}
