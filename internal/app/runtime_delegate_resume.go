package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
)

var (
	ErrHandoffIDRequired           = errors.New("handoff id is required")
	ErrHandoffNotFound             = errors.New("handoff not found")
	ErrHandoffPermissionNotPending = errors.New("handoff does not have a pending permission request")
	ErrHandoffQuestionNotPending   = errors.New("handoff does not have a pending question request")
)

type ResolveDelegatedSessionTurnInput struct {
	ParentSessionID        string
	HandoffID              string
	Decision               events.PermissionDecision
	Scope                  events.PermissionScope
	GrantPath              string
	Recursive              bool
	ExecutionDecision      events.ExecutionApprovalDecision
	ExecutionExecPolicy    *events.ExecutionPolicyAmendment
	ExecutionNetworkPolicy *events.ExecutionNetworkPolicyAmendment
}

type ResolveDelegatedSessionTurnResult struct {
	HandoffID  string
	ChildTurn  RunSessionResult
	ParentTurn RunSessionResult
}

type AnswerDelegatedSessionQuestionInput struct {
	ParentSessionID string
	HandoffID       string
	Answer          string
}

type AnswerDelegatedSessionQuestionResult struct {
	HandoffID  string
	ChildTurn  RunSessionResult
	ParentTurn RunSessionResult
}

func (r *Runtime) ResolveDelegatedSessionTurn(ctx context.Context, input ResolveDelegatedSessionTurnInput) (ResolveDelegatedSessionTurnResult, error) {
	if strings.TrimSpace(input.ParentSessionID) == "" {
		return ResolveDelegatedSessionTurnResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.HandoffID) == "" {
		return ResolveDelegatedSessionTurnResult{}, ErrHandoffIDRequired
	}

	parentState, err := r.Sessions.Snapshot(ctx, input.ParentSessionID)
	if err != nil {
		return ResolveDelegatedSessionTurnResult{}, err
	}
	parentTurnID, handoff := findHandoffState(parentState, input.HandoffID)
	if handoff == nil {
		return ResolveDelegatedSessionTurnResult{}, ErrHandoffNotFound
	}
	if strings.TrimSpace(handoff.PermissionRequestID) == "" {
		return ResolveDelegatedSessionTurnResult{}, ErrHandoffPermissionNotPending
	}

	handoffPayload, fragments, err := r.prepareDelegatedChildResume(ctx, input.ParentSessionID, parentTurnID, handoff)
	if err != nil {
		return ResolveDelegatedSessionTurnResult{}, err
	}
	stopPreview, err := r.startDelegatedHandoffPreview(ctx, handoffPayload, "resuming child turn")
	if err != nil {
		return ResolveDelegatedSessionTurnResult{}, err
	}
	defer stopPreview()

	result, err := r.ResolveSessionTurn(ctx, ResolveSessionTurnInput{
		SessionID:              handoff.ChildSessionID,
		TurnID:                 handoff.ChildTurnID,
		PermissionRequestID:    handoff.PermissionRequestID,
		UserText:               handoff.Task,
		AgentID:                handoff.ChildAgentID,
		AllowedTools:           slices.Clone(handoff.AllowedTools),
		Fragments:              fragments,
		Decision:               input.Decision,
		Scope:                  input.Scope,
		GrantPath:              input.GrantPath,
		Recursive:              input.Recursive,
		ExecutionDecision:      input.ExecutionDecision,
		ExecutionExecPolicy:    input.ExecutionExecPolicy,
		ExecutionNetworkPolicy: input.ExecutionNetworkPolicy,
	})
	if err != nil {
		r.log("runtime").Error("delegated permission resolution failed", err,
			"parent_session_id", input.ParentSessionID,
			"handoff_id", input.HandoffID,
		)
		return ResolveDelegatedSessionTurnResult{}, err
	}
	modelRoute, err := parseStoredModelRoute(handoffPayload.Model)
	if err != nil {
		return ResolveDelegatedSessionTurnResult{}, err
	}
	result, resultPayload, reviewPayload, err := r.prepareDelegatedHandoffCompletion(ctx, handoffPayload, result, modelRoute, workflowTurnBudget{})
	if err != nil {
		return ResolveDelegatedSessionTurnResult{}, err
	}
	if err := r.appendAgentResultForHandoff(ctx, parentTurnID, handoffPayload, resultPayload); err != nil {
		return ResolveDelegatedSessionTurnResult{}, err
	}
	if reviewPayload != nil {
		if err := r.appendStructuredDelegatedReview(ctx, handoffPayload, resultPayload.ChildTurnID, *reviewPayload); err != nil {
			return ResolveDelegatedSessionTurnResult{}, err
		}
	}
	parentResult, err := r.resumeDelegatedParentTurn(ctx, input.ParentSessionID, parentTurnID, input.HandoffID)
	if err != nil {
		return ResolveDelegatedSessionTurnResult{}, err
	}
	r.log("runtime").Op("delegated handoff resumed",
		"parent_session_id", input.ParentSessionID,
		"handoff_id", input.HandoffID,
		"child_session_id", handoff.ChildSessionID,
		"child_turn_id", handoff.ChildTurnID,
		"status", result.Status,
		"pending_request_id", result.PendingRequestID,
	)

	return ResolveDelegatedSessionTurnResult{
		HandoffID:  input.HandoffID,
		ChildTurn:  result,
		ParentTurn: parentResult,
	}, nil
}

func (r *Runtime) AnswerDelegatedSessionQuestion(ctx context.Context, input AnswerDelegatedSessionQuestionInput) (AnswerDelegatedSessionQuestionResult, error) {
	if strings.TrimSpace(input.ParentSessionID) == "" {
		return AnswerDelegatedSessionQuestionResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.HandoffID) == "" {
		return AnswerDelegatedSessionQuestionResult{}, ErrHandoffIDRequired
	}

	parentState, err := r.Sessions.Snapshot(ctx, input.ParentSessionID)
	if err != nil {
		return AnswerDelegatedSessionQuestionResult{}, err
	}
	parentTurnID, handoff := findHandoffState(parentState, input.HandoffID)
	if handoff == nil {
		return AnswerDelegatedSessionQuestionResult{}, ErrHandoffNotFound
	}
	if strings.TrimSpace(handoff.QuestionRequestID) == "" {
		return AnswerDelegatedSessionQuestionResult{}, ErrHandoffQuestionNotPending
	}
	questionPurpose, err := r.pendingDelegatedQuestionPurpose(ctx, handoff)
	if err != nil {
		return AnswerDelegatedSessionQuestionResult{}, err
	}

	handoffPayload, fragments, err := r.prepareDelegatedChildResume(ctx, input.ParentSessionID, parentTurnID, handoff)
	if err != nil {
		return AnswerDelegatedSessionQuestionResult{}, err
	}
	stopPreview, err := r.startDelegatedHandoffPreview(ctx, handoffPayload, "answering child question")
	if err != nil {
		return AnswerDelegatedSessionQuestionResult{}, err
	}
	defer stopPreview()

	result, err := r.AnswerSessionQuestion(ctx, AnswerSessionQuestionInput{
		SessionID:    handoff.ChildSessionID,
		TurnID:       handoff.ChildTurnID,
		RequestID:    handoff.QuestionRequestID,
		Answer:       input.Answer,
		UserText:     handoff.Task,
		AgentID:      handoff.ChildAgentID,
		AllowedTools: slices.Clone(handoff.AllowedTools),
		Fragments:    fragments,
	})
	if err != nil {
		r.log("runtime").Error("delegated question answer failed", err,
			"parent_session_id", input.ParentSessionID,
			"handoff_id", input.HandoffID,
		)
		return AnswerDelegatedSessionQuestionResult{}, err
	}
	modelRoute, err := parseStoredModelRoute(handoffPayload.Model)
	if err != nil {
		return AnswerDelegatedSessionQuestionResult{}, err
	}
	result, resultPayload, reviewPayload, err := r.prepareDelegatedHandoffCompletion(ctx, handoffPayload, result, modelRoute, workflowTurnBudget{})
	if err != nil {
		return AnswerDelegatedSessionQuestionResult{}, err
	}
	if err := r.appendAgentResultForHandoff(ctx, parentTurnID, handoffPayload, resultPayload); err != nil {
		return AnswerDelegatedSessionQuestionResult{}, err
	}
	if reviewPayload != nil {
		if err := r.appendStructuredDelegatedReview(ctx, handoffPayload, resultPayload.ChildTurnID, *reviewPayload); err != nil {
			return AnswerDelegatedSessionQuestionResult{}, err
		}
	}
	var parentResult RunSessionResult
	if delegatedQuestionAnswerCompletesParentWithoutResume(handoff, questionPurpose, result) {
		parentResult, err = r.completeDelegatedParentTurnWithoutResume(ctx, input.ParentSessionID, parentTurnID, input.HandoffID)
		if err != nil {
			return AnswerDelegatedSessionQuestionResult{}, err
		}
	} else {
		parentResult, err = r.resumeDelegatedParentTurn(ctx, input.ParentSessionID, parentTurnID, input.HandoffID)
		if err != nil {
			return AnswerDelegatedSessionQuestionResult{}, err
		}
	}
	r.log("runtime").Op("delegated question answered",
		"parent_session_id", input.ParentSessionID,
		"handoff_id", input.HandoffID,
		"child_session_id", handoff.ChildSessionID,
		"child_turn_id", result.TurnID,
		"status", result.Status,
		"pending_request_id", result.PendingRequestID,
	)

	return AnswerDelegatedSessionQuestionResult{
		HandoffID:  input.HandoffID,
		ChildTurn:  result,
		ParentTurn: parentResult,
	}, nil
}

func (r *Runtime) pendingDelegatedQuestionPurpose(ctx context.Context, handoff *events.AgentHandoffState) (string, error) {
	if handoff == nil || strings.TrimSpace(handoff.ChildSessionID) == "" || strings.TrimSpace(handoff.QuestionRequestID) == "" {
		return "", nil
	}
	childState, err := r.Sessions.Snapshot(ctx, handoff.ChildSessionID)
	if err != nil {
		return "", err
	}
	request := pendingQuestionRequestState(childState, handoff.QuestionRequestID)
	if request == nil {
		return "", nil
	}
	return strings.TrimSpace(request.Purpose), nil
}

func delegatedQuestionAnswerCompletesParentWithoutResume(handoff *events.AgentHandoffState, questionPurpose string, result RunSessionResult) bool {
	if handoff == nil || result.Status != TurnRunStatusCompleted {
		return false
	}
	return strings.TrimSpace(handoff.ChildAgentID) == "planner" &&
		strings.TrimSpace(questionPurpose) == events.QuestionPurposePlannerPlanDecision
}

func (r *Runtime) completeDelegatedParentTurnWithoutResume(ctx context.Context, parentSessionID, parentTurnID, handoffID string) (RunSessionResult, error) {
	state, err := r.Sessions.Snapshot(ctx, parentSessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	turn := state.Turns[parentTurnID]
	if turn == nil {
		return RunSessionResult{}, ErrTurnIDRequired
	}
	if !delegatedParentTurnResumable(turn, handoffID) {
		return r.loadSessionTurnResult(ctx, parentSessionID, parentTurnID, RunTurnResult{Status: TurnRunStatusCompleted})
	}

	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		SessionID: parentSessionID,
		TurnID:    parentTurnID,
	})
	if err != nil {
		return RunSessionResult{}, err
	}
	history, err := r.Runner.loadTurnReplay(ctx, ResumeTurnInput{
		SessionID: parentSessionID,
		TurnID:    parentTurnID,
		RequestID: handoffID,
	})
	if err != nil {
		return RunSessionResult{}, err
	}
	toolResult, err := r.Runner.resumePendingTool(ctx, parentSessionID, parentTurnID, history, resolved.allowedTools)
	if err != nil {
		return RunSessionResult{}, err
	}
	if toolResult.Status == ToolExecutionStatusPending {
		return r.loadSessionTurnResult(ctx, parentSessionID, parentTurnID, RunTurnResult{
			Status:           TurnRunStatusPending,
			PendingRequestID: toolResult.PendingRequestID,
		})
	}
	if strings.TrimSpace(toolResult.Error) != "" {
		return r.recordTurnFailure(ctx, parentSessionID, parentTurnID, resolved.userText, resolved.attachments, errors.New(toolResult.Error))
	}
	if err := r.Runner.appendTurnDone(ctx, parentSessionID, parentTurnID); err != nil {
		return RunSessionResult{}, err
	}
	return r.loadSessionTurnResult(ctx, parentSessionID, parentTurnID, RunTurnResult{Status: TurnRunStatusCompleted})
}

func findHandoffState(state events.SessionState, handoffID string) (string, *events.AgentHandoffState) {
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		handoff := turn.Handoffs[handoffID]
		if handoff != nil {
			return turnID, handoff
		}
	}
	return "", nil
}

func handoffPayloadFromState(parentSessionID, parentTurnID string, handoff *events.AgentHandoffState) events.AgentHandoffPayload {
	return events.AgentHandoffPayload{
		HandoffID:          handoff.HandoffID,
		ToolCallID:         handoff.ToolCallID,
		ParentSessionID:    parentSessionID,
		ParentTurnID:       parentTurnID,
		ParentAgentID:      handoff.ParentAgentID,
		ChildSessionID:     handoff.ChildSessionID,
		ChildTurnID:        handoff.ChildTurnID,
		ChildAgentID:       handoff.ChildAgentID,
		Task:               handoff.Task,
		ContextSummary:     handoff.ContextSummary,
		SourceHandoffIDs:   append([]string(nil), handoff.SourceHandoffIDs...),
		ProvidedKinds:      append([]string(nil), handoff.ProvidedKinds...),
		ExplorationEntries: cloneHandoffExplorationEntries(handoff.ExplorationEntries),
		Model:              handoff.Model,
		AllowedTools:       slices.Clone(handoff.AllowedTools),
	}
}

func (r *Runtime) prepareDelegatedChildResume(ctx context.Context, parentSessionID, parentTurnID string, handoff *events.AgentHandoffState) (events.AgentHandoffPayload, []prompt.Fragment, error) {
	childState, err := r.Sessions.Snapshot(ctx, handoff.ChildSessionID)
	if err != nil {
		return events.AgentHandoffPayload{}, nil, err
	}
	childDefinition, err := r.resolveTurnAgent(childState.WorkspaceRoot, handoff.ChildAgentID)
	if err != nil {
		return events.AgentHandoffPayload{}, nil, err
	}
	mcpState, err := r.syncSessionMCPState(ctx, handoff.ChildSessionID, childState.WorkspaceRoot, childState.MCP)
	if err != nil {
		return events.AgentHandoffPayload{}, nil, err
	}
	responseStyle := responseStyleForTurn(childState.Turns[handoff.ChildTurnID], r.Config.Sessions.EffectiveResponseStyle())
	availableSkills, err := r.availableTurnSkills(childState.WorkspaceRoot)
	if err != nil {
		return events.AgentHandoffPayload{}, nil, err
	}
	fragments, err := defaultTurnFragments(childDefinition, childState.WorkspaceRoot, childState.AdditionalWorkspaceRoots, availableSkills, nil, handoff.AllowedTools, mcpState, r.Memory, responseStyle, r.Config.Execution, collectCompactedInspectionProgressPromptEntries(&childState))
	if err != nil {
		return events.AgentHandoffPayload{}, nil, err
	}
	handoffPayload := handoffPayloadFromState(parentSessionID, parentTurnID, handoff)
	fragments = append(fragments, handoffPromptFragments(handoffPayload)...)
	if len(handoffPayload.SourceHandoffIDs) > 0 {
		parentState, err := r.Sessions.Snapshot(ctx, parentSessionID)
		if err != nil {
			return events.AgentHandoffPayload{}, nil, err
		}
		fragments = append(fragments, handoffSourceFragments(parentState, handoffPayload.SourceHandoffIDs, childDefinition.ID)...)
	}
	return handoffPayload, fragments, nil
}

func (r *Runtime) resumeDelegatedParentTurn(ctx context.Context, parentSessionID, parentTurnID, handoffID string) (RunSessionResult, error) {
	state, err := r.Sessions.Snapshot(ctx, parentSessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	if !delegatedParentTurnResumable(state.Turns[parentTurnID], handoffID) {
		return RunSessionResult{}, nil
	}
	mcpState, err := r.syncSessionMCPState(ctx, parentSessionID, state.WorkspaceRoot, state.MCP)
	if err != nil {
		return RunSessionResult{}, err
	}
	resolved, err := resolveResumeTurn(state, ResolveSessionTurnInput{
		SessionID: parentSessionID,
		TurnID:    parentTurnID,
	})
	if err != nil {
		return RunSessionResult{}, err
	}
	availableSkills, err := r.availableTurnSkills(state.WorkspaceRoot)
	if err != nil {
		return r.recordTurnFailure(ctx, parentSessionID, parentTurnID, resolved.userText, resolved.attachments, err)
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
		return r.recordTurnFailure(ctx, parentSessionID, parentTurnID, resolved.userText, resolved.attachments, err)
	}
	responseStyle := resolved.responseStyle
	if responseStyle == "" {
		responseStyle = r.Config.Sessions.EffectiveResponseStyle()
	}
	var fragments []prompt.Fragment
	if strings.TrimSpace(resolved.instructions) == "" {
		fragments, err = defaultTurnFragments(capabilities.definition, state.WorkspaceRoot, state.AdditionalWorkspaceRoots, availableSkills, capabilities.selectedSkills, capabilities.AllowedTools, mcpState, r.Memory, responseStyle, r.Config.Execution, collectCompactedInspectionProgressPromptEntries(&state))
		if err != nil {
			return r.recordTurnFailure(ctx, parentSessionID, parentTurnID, resolved.userText, resolved.attachments, err)
		}
	}
	effectiveThinkingMode := capabilities.EffectiveReasoningVariant(resolved.thinkingMode)
	effectiveThinkingEnabled := capabilities.EffectiveThinkingEnabled(resolved.thinkingEnabled)
	thinkingSupported := capabilities.SupportsThinkingOutput()
	if !resolved.preserveSessionModel {
		if err := r.syncSessionModelRoute(ctx, parentSessionID, capabilities.ModelRoute); err != nil {
			return RunSessionResult{}, err
		}
	}

	turnCtx, turnHandle, finishTurn, err := r.beginCancelableTurnContext(ctx, parentSessionID, parentTurnID)
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
		SessionID:                   parentSessionID,
		TurnID:                      parentTurnID,
		AgentID:                     capabilities.AgentID,
		UserText:                    resolved.userText,
		Attachments:                 resolved.attachments,
		Fragments:                   fragments,
		Instructions:                resolved.instructions,
		CacheablePrefix:             resolved.cacheablePrefix,
		DynamicSuffix:               resolved.dynamicSuffix,
		PromptCompactionTokensSaved: resolved.promptCompactionTokensSaved,
		ModelRoute:                  capabilities.ModelRoute,
		ThinkingSupported:           thinkingSupported,
		ThinkingEnabled:             effectiveThinkingEnabled,
		ThinkingMode:                effectiveThinkingMode,
		AllowedTools:                capabilities.AllowedTools,
		RequestID:                   handoffID,
	})
	if turnHandle.canceled() {
		return r.loadCanceledSessionTurnResult(parentSessionID, parentTurnID, resolved.userText, resolved.attachments)
	}
	if err != nil {
		if failed, ok, failedErr := r.failedSessionTurnResult(ctx, parentSessionID, parentTurnID); failedErr == nil && ok {
			return failed, nil
		}
		return RunSessionResult{}, err
	}
	if result.Status == TurnRunStatusRolled {
		hideAssistantPreview := false
		if turn := state.Turns[parentTurnID]; turn != nil && turn.Config != nil {
			hideAssistantPreview = turn.Config.HideAssistantPreview
		}
		finishTurn()
		finishedTurn = true
		return r.continueRolledOverResumedTurn(
			ctx,
			parentSessionID,
			parentTurnID,
			resolved,
			capabilities,
			effectiveThinkingEnabled,
			effectiveThinkingMode,
			fragments,
			hideAssistantPreview,
			false,
		)
	}
	return r.finalizeTurnRunResult(ctx, turnCtx, turnHandle, parentSessionID, parentTurnID, result)
}

func delegatedParentTurnResumable(turn *events.TurnState, handoffID string) bool {
	if turn == nil || turn.Status != events.TurnStatusRunning || strings.TrimSpace(handoffID) == "" {
		return false
	}
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil || call.Completed {
			continue
		}
		if strings.TrimSpace(call.HandoffID) == strings.TrimSpace(handoffID) {
			return true
		}
	}
	return false
}
