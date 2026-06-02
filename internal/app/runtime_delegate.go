package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

var (
	ErrParentTurnRequired                     = errors.New("parent turn id is required")
	ErrParentTurnNotFound                     = errors.New("parent turn not found")
	ErrParentAgentRequired                    = errors.New("parent agent id is required")
	ErrChildAgentRequired                     = errors.New("child agent id is required")
	ErrChildAgentUnavailable                  = errors.New("child agent is not available for delegation")
	ErrHandoffTaskRequired                    = errors.New("handoff task is required")
	ErrContextSummaryRequired                 = errors.New("context summary is required")
	ErrDelegatedReviewStructuredOutputInvalid = errors.New("delegated reviewer returned invalid structured review output")
)

type DelegateSessionTurnInput struct {
	ParentSessionID    string
	ParentTurnID       string
	ParentToolCallID   string
	ParentAgentID      string
	ChildAgentID       string
	Task               string
	ContextSummary     string
	SourceHandoffIDs   []string
	ModelRouteOverride provider.ModelRoute
	WorkflowBudget     workflowTurnBudget
}

type DelegateSessionTurnResult struct {
	HandoffID      string
	ChildSessionID string
	ChildTurn      RunSessionResult
}

func (r *Runtime) DelegateSessionTurn(ctx context.Context, input DelegateSessionTurnInput) (DelegateSessionTurnResult, error) {
	switch {
	case strings.TrimSpace(input.ParentSessionID) == "":
		return DelegateSessionTurnResult{}, ErrSessionIDRequired
	case strings.TrimSpace(input.ParentTurnID) == "":
		return DelegateSessionTurnResult{}, ErrParentTurnRequired
	case strings.TrimSpace(input.ParentAgentID) == "":
		return DelegateSessionTurnResult{}, ErrParentAgentRequired
	case strings.TrimSpace(input.ChildAgentID) == "":
		return DelegateSessionTurnResult{}, ErrChildAgentRequired
	case strings.TrimSpace(input.Task) == "":
		return DelegateSessionTurnResult{}, ErrHandoffTaskRequired
	case strings.TrimSpace(input.ContextSummary) == "":
		return DelegateSessionTurnResult{}, ErrContextSummaryRequired
	}

	parentState, err := r.Sessions.Snapshot(ctx, input.ParentSessionID)
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	if parentState.Turns[input.ParentTurnID] == nil {
		return DelegateSessionTurnResult{}, ErrParentTurnNotFound
	}
	explorationEntries, err := handoffExplorationEntries(ctx, r.Sessions, input.ParentSessionID, input.ParentTurnID, parentState.Turns[input.ParentTurnID])
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}

	childDefinition, err := r.resolveTurnAgent(parentState.WorkspaceRoot, input.ChildAgentID)
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	if !childDefinition.Delegatable() {
		return DelegateSessionTurnResult{}, ErrChildAgentUnavailable
	}
	parentDefinition, err := r.resolveTurnAgent(parentState.WorkspaceRoot, input.ParentAgentID)
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	sourceHandoffIDs, err := r.resolveDelegateSourceHandoffIDs(parentState, input.ParentTurnID, childDefinition, input.SourceHandoffIDs)
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	input.SourceHandoffIDs = sourceHandoffIDs
	var childModelRoute provider.ModelRoute
	if hasConfiguredModelRoute(input.ModelRouteOverride) {
		childModelRoute, err = r.resolveConfiguredTurnModelRoute(input.ModelRouteOverride)
	} else {
		childModelRoute, err = r.resolveDelegatedChildModelRoute(parentState, input.ParentTurnID, childDefinition)
	}
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}

	childTools := r.allowedToolsForTurn(parentState, childDefinition)
	effectiveTools := effectiveDelegatedChildTools(childDefinition, childTools, parentState)
	responseStyle := responseStyleForTurn(parentState.Turns[input.ParentTurnID], r.Config.Sessions.EffectiveResponseStyle())
	if handoff := matchingDelegateHandoff(parentState.Turns[input.ParentTurnID], input, explorationEntries); handoff != nil {
		switch handoff.Status {
		case events.AgentResultStatusCompleted, events.AgentResultStatusPendingPermission, events.AgentResultStatusPendingQuestion:
			r.log("runtime").Op("delegated handoff reused",
				"parent_session_id", input.ParentSessionID,
				"parent_turn_id", input.ParentTurnID,
				"handoff_id", handoff.HandoffID,
				"child_session_id", handoff.ChildSessionID,
				"child_turn_id", handoff.ChildTurnID,
				"child_agent_id", handoff.ChildAgentID,
			)
			return delegateSessionTurnResultFromHandoff(handoff), nil
		case events.AgentResultStatusFailed:
			if !failedDelegatedHandoffRetryable(handoff) {
				r.log("runtime").Op("failed delegated handoff reused",
					"parent_session_id", input.ParentSessionID,
					"parent_turn_id", input.ParentTurnID,
					"handoff_id", handoff.HandoffID,
					"child_session_id", handoff.ChildSessionID,
					"child_turn_id", handoff.ChildTurnID,
					"child_agent_id", handoff.ChildAgentID,
				)
				return delegateSessionTurnResultFromHandoff(handoff), nil
			}
			return r.retryDelegatedHandoff(ctx, parentState, input, explorationEntries, handoff, childDefinition, childModelRoute, effectiveTools, responseStyle)
		}
	}

	childSessionID, err := r.createSessionWithMode(ctx, parentState.WorkspaceRoot, parentState.AdditionalWorkspaceRoots, PermissionMode(parentState.PermissionMode))
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	if err := r.activateWorkspaceMCP(ctx, parentState.WorkspaceRoot); err != nil {
		return DelegateSessionTurnResult{}, err
	}
	if err := r.appendSessionMCPCatalog(ctx, childSessionID, parentState.WorkspaceRoot); err != nil {
		return DelegateSessionTurnResult{}, err
	}
	if err := r.syncSessionModelRoute(ctx, childSessionID, childModelRoute); err != nil {
		return DelegateSessionTurnResult{}, err
	}
	childTurnID := newRuntimeID("turn")
	handoffID := newRuntimeID("handoff")
	handoff := events.AgentHandoffPayload{
		HandoffID:          handoffID,
		ToolCallID:         strings.TrimSpace(input.ParentToolCallID),
		ParentSessionID:    input.ParentSessionID,
		ParentTurnID:       input.ParentTurnID,
		ParentAgentID:      parentDefinition.ID,
		ChildSessionID:     childSessionID,
		ChildTurnID:        childTurnID,
		ChildAgentID:       childDefinition.ID,
		Task:               input.Task,
		ContextSummary:     input.ContextSummary,
		SourceHandoffIDs:   append([]string(nil), sourceHandoffIDs...),
		ProvidedKinds:      handoffProvidedKinds(childDefinition),
		ExplorationEntries: cloneHandoffExplorationEntries(explorationEntries),
		Model:              childModelRoute.Primary.String(),
		AllowedTools:       slices.Clone(effectiveTools),
	}
	if err := r.appendDelegatedHandoff(ctx, handoff); err != nil {
		return DelegateSessionTurnResult{}, err
	}
	r.log("runtime").Op("delegated handoff started",
		"parent_session_id", input.ParentSessionID,
		"parent_turn_id", input.ParentTurnID,
		"handoff_id", handoffID,
		"child_session_id", childSessionID,
		"child_turn_id", childTurnID,
		"child_agent_id", childDefinition.ID,
	)
	r.log("runtime").Debug("delegated handoff input",
		"handoff_id", handoffID,
		"task", input.Task,
		"context_summary", input.ContextSummary,
		"allowed_tools", effectiveTools,
	)
	return r.executeDelegatedHandoff(ctx, parentState, handoff, childDefinition, childModelRoute, effectiveTools, responseStyle, input.WorkflowBudget, "starting child turn")
}

func effectiveDelegatedChildTools(childDefinition agent.Definition, tools []string, parentState events.SessionState) []string {
	out := slices.Clone(tools)
	switch strings.TrimSpace(childDefinition.ID) {
	case "planner":
		return slices.DeleteFunc(out, func(name string) bool {
			switch strings.TrimSpace(name) {
			case tool.QuestionToolName, tool.TaskWorkflowToolName, "save_plan":
				return true
			default:
				return false
			}
		})
	case reviewerAgentID:
		return slices.DeleteFunc(out, func(name string) bool {
			switch strings.TrimSpace(name) {
			case tool.TaskWorkflowToolName:
				return true
			case tool.TaskReviewToolName:
				return !sessionHasReviewableTasks(parentState)
			default:
				return false
			}
		})
	default:
		return out
	}
}

func sessionHasReviewableTasks(state events.SessionState) bool {
	return len(state.TaskOrder) > 0
}

func matchingDelegateHandoff(turn *events.TurnState, input DelegateSessionTurnInput, explorationEntries []events.AgentHandoffExplorationEntry) *events.AgentHandoffState {
	if turn == nil {
		return nil
	}
	for idx := len(turn.HandoffOrder) - 1; idx >= 0; idx-- {
		handoff := turn.Handoffs[turn.HandoffOrder[idx]]
		if handoff == nil {
			continue
		}
		if delegateRequestMatchesHandoff(input, explorationEntries, handoff) {
			return handoff
		}
	}
	return nil
}

func delegateRequestMatchesHandoff(input DelegateSessionTurnInput, explorationEntries []events.AgentHandoffExplorationEntry, handoff *events.AgentHandoffState) bool {
	if handoff == nil {
		return false
	}
	if strings.TrimSpace(input.ChildAgentID) != strings.TrimSpace(handoff.ChildAgentID) {
		return false
	}
	if strings.TrimSpace(input.Task) != strings.TrimSpace(handoff.Task) {
		return false
	}
	if strings.TrimSpace(input.ContextSummary) != strings.TrimSpace(handoff.ContextSummary) {
		return false
	}
	if !sameHandoffExplorationEntries(explorationEntries, handoff.ExplorationEntries) {
		return false
	}
	if !slices.Equal(compactHandoffIDs(input.SourceHandoffIDs), compactHandoffIDs(handoff.SourceHandoffIDs)) {
		return false
	}
	if hasConfiguredModelRoute(input.ModelRouteOverride) && strings.TrimSpace(handoff.Model) != strings.TrimSpace(input.ModelRouteOverride.Primary.String()) {
		return false
	}
	return true
}

func delegateSessionTurnResultFromHandoff(handoff *events.AgentHandoffState) DelegateSessionTurnResult {
	childTurn := RunSessionResult{
		SessionID:     strings.TrimSpace(handoff.ChildSessionID),
		TurnID:        strings.TrimSpace(handoff.ChildTurnID),
		AssistantText: strings.TrimSpace(handoff.AssistantText),
		Error:         strings.TrimSpace(handoff.Error),
	}
	switch handoff.Status {
	case events.AgentResultStatusPendingPermission:
		childTurn.Status = TurnRunStatusPending
		childTurn.PendingRequestID = strings.TrimSpace(handoff.PermissionRequestID)
		if handoff.ExecutionApproval != nil {
			copyPending := *handoff.ExecutionApproval
			childTurn.PendingExecution = &copyPending
		} else {
			childTurn.PendingPermission = &events.PermissionRequestState{
				RequestID:        strings.TrimSpace(handoff.PermissionRequestID),
				Kind:             handoff.PermissionKind,
				Access:           strings.TrimSpace(handoff.PermissionAccess),
				Path:             strings.TrimSpace(handoff.PermissionPath),
				WorkingDirectory: strings.TrimSpace(handoff.PermissionDir),
				ToolName:         strings.TrimSpace(handoff.PermissionToolName),
				Command:          strings.TrimSpace(handoff.PermissionCommand),
				Reason:           strings.TrimSpace(handoff.PermissionReason),
				TurnID:           strings.TrimSpace(handoff.ChildTurnID),
			}
		}
	case events.AgentResultStatusPendingQuestion:
		childTurn.Status = TurnRunStatusPending
		childTurn.PendingRequestID = strings.TrimSpace(handoff.QuestionRequestID)
		childTurn.PendingQuestion = &events.QuestionRequestState{
			QuestionID: strings.TrimSpace(handoff.QuestionRequestID),
			TurnID:     strings.TrimSpace(handoff.ChildTurnID),
			ToolName:   strings.TrimSpace(handoff.QuestionToolName),
			Question:   strings.TrimSpace(handoff.QuestionText),
			Options:    append([]string(nil), handoff.QuestionOptions...),
		}
	case events.AgentResultStatusFailed:
		childTurn.Status = TurnRunStatusFailed
	default:
		childTurn.Status = TurnRunStatusCompleted
	}
	return DelegateSessionTurnResult{
		HandoffID:      strings.TrimSpace(handoff.HandoffID),
		ChildSessionID: strings.TrimSpace(handoff.ChildSessionID),
		ChildTurn:      childTurn,
	}
}

func failedDelegatedHandoffRetryable(handoff *events.AgentHandoffState) bool {
	if handoff == nil || handoff.Status != events.AgentResultStatusFailed {
		return false
	}
	return !failedDelegatedHandoffHasTerminalContractError(handoff)
}

func failedDelegatedHandoffHasTerminalContractError(handoff *events.AgentHandoffState) bool {
	if handoff == nil {
		return false
	}
	errorText := strings.TrimSpace(handoff.Error)
	if errorText == "" {
		return false
	}
	if strings.TrimSpace(handoff.ChildAgentID) == reviewerAgentID &&
		strings.Contains(errorText, ErrDelegatedReviewStructuredOutputInvalid.Error()) {
		return true
	}
	if strings.TrimSpace(handoff.ChildAgentID) != "planner" {
		return false
	}
	switch errorText {
	case ErrPlannerSavePlanQuestionRequiresVisiblePlan.Error(),
		userFacingTurnErrorMessage(ErrPlannerSavePlanQuestionRequiresVisiblePlan),
		plannerSavePlanQuestionRequiresVisiblePlanText:
		return true
	default:
		return false
	}
}

func (r *Runtime) retryDelegatedHandoff(ctx context.Context, parentState events.SessionState, input DelegateSessionTurnInput, explorationEntries []events.AgentHandoffExplorationEntry, handoff *events.AgentHandoffState, childDefinition agent.Definition, childModelRoute provider.ModelRoute, effectiveTools []string, responseStyle ResponseStyle) (DelegateSessionTurnResult, error) {
	if handoff == nil || strings.TrimSpace(handoff.ChildSessionID) == "" {
		return DelegateSessionTurnResult{}, ErrHandoffNotFound
	}
	if err := r.syncSessionModelRoute(ctx, handoff.ChildSessionID, childModelRoute); err != nil {
		return DelegateSessionTurnResult{}, err
	}
	payload := handoffPayloadFromState(input.ParentSessionID, input.ParentTurnID, handoff)
	payload.ToolCallID = strings.TrimSpace(input.ParentToolCallID)
	payload.ParentAgentID = strings.TrimSpace(input.ParentAgentID)
	payload.ChildAgentID = childDefinition.ID
	payload.Task = strings.TrimSpace(input.Task)
	payload.ContextSummary = strings.TrimSpace(input.ContextSummary)
	payload.SourceHandoffIDs = append([]string(nil), input.SourceHandoffIDs...)
	payload.ProvidedKinds = handoffProvidedKinds(childDefinition)
	payload.ExplorationEntries = cloneHandoffExplorationEntries(explorationEntries)
	payload.Model = childModelRoute.Primary.String()
	payload.AllowedTools = slices.Clone(effectiveTools)
	payload.ChildTurnID = newRuntimeID("turn")
	if err := r.appendDelegatedHandoff(ctx, payload); err != nil {
		return DelegateSessionTurnResult{}, err
	}
	r.log("runtime").Op("delegated handoff retried",
		"parent_session_id", input.ParentSessionID,
		"parent_turn_id", input.ParentTurnID,
		"handoff_id", payload.HandoffID,
		"child_session_id", payload.ChildSessionID,
		"child_turn_id", payload.ChildTurnID,
		"child_agent_id", payload.ChildAgentID,
	)
	r.log("runtime").Debug("delegated handoff retry input",
		"handoff_id", payload.HandoffID,
		"task", payload.Task,
		"context_summary", payload.ContextSummary,
		"allowed_tools", payload.AllowedTools,
	)
	return r.executeDelegatedHandoff(ctx, parentState, payload, childDefinition, childModelRoute, effectiveTools, responseStyle, input.WorkflowBudget, "retrying child turn")
}

func (r *Runtime) appendDelegatedHandoff(ctx context.Context, handoff events.AgentHandoffPayload) error {
	for _, draft := range []events.Draft{
		{
			SessionID: handoff.ParentSessionID,
			TurnID:    handoff.ParentTurnID,
			Type:      events.TypeAgentHandoff,
			Payload:   handoff,
		},
		{
			SessionID: handoff.ChildSessionID,
			TurnID:    handoff.ChildTurnID,
			Type:      events.TypeAgentHandoff,
			Payload:   handoff,
		},
	} {
		if _, err := r.Sessions.append(ctx, draft); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) executeDelegatedHandoff(ctx context.Context, parentState events.SessionState, handoff events.AgentHandoffPayload, childDefinition agent.Definition, childModelRoute provider.ModelRoute, effectiveTools []string, responseStyle ResponseStyle, workflowBudget workflowTurnBudget, previewAction string) (DelegateSessionTurnResult, error) {
	mcpState, err := r.syncSessionMCPState(ctx, handoff.ChildSessionID, parentState.WorkspaceRoot, parentState.MCP)
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	availableSkills, err := r.availableTurnSkills(parentState.WorkspaceRoot)
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	fragments, err := defaultTurnFragments(childDefinition, parentState.WorkspaceRoot, parentState.AdditionalWorkspaceRoots, availableSkills, nil, effectiveTools, mcpState, r.Memory, responseStyle, r.Config.Execution, collectCompactedInspectionProgressPromptEntries(&parentState))
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	fragments = append(fragments, handoffPromptFragments(handoff)...)
	fragments = append(fragments, handoffSourceFragments(parentState, handoff.SourceHandoffIDs, childDefinition.ID)...)
	thinkingEnabled := false
	thinkingMode := ""
	if parentTurn := parentState.Turns[handoff.ParentTurnID]; parentTurn != nil && parentTurn.Config != nil {
		thinkingEnabled = parentTurn.Config.ThinkingEnabled
		thinkingMode = strings.TrimSpace(parentTurn.Config.ThinkingMode)
	}
	thinkingSupported := r.supportsThinkingOutput(childModelRoute.Primary, effectiveTools)
	effectiveThinkingEnabled := thinkingEnabled && thinkingSupported
	capabilities := TurnCapabilities{
		AgentID:                    childDefinition.ID,
		ModelRoute:                 childModelRoute,
		AllowedTools:               slices.Clone(effectiveTools),
		SupportedReasoningVariants: r.supportedReasoningVariants(childModelRoute.Primary, effectiveTools),
		SupportsThinking:           thinkingSupported,
	}
	effectiveThinkingMode := capabilities.EffectiveReasoningVariant(thinkingMode)
	if err := r.Runner.appendTurnConfigured(ctx, handoff.ChildSessionID, handoff.ChildTurnID, newTurnConfiguredPayload(capabilities, nil, "", false, effectiveThinkingEnabled, effectiveThinkingMode, responseStyle, false)); err != nil {
		return DelegateSessionTurnResult{}, err
	}
	stopPreview, err := r.startDelegatedHandoffPreview(ctx, handoff, previewAction)
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	defer stopPreview()

	runResult, err := r.Runner.Run(ctx, RunTurnInput{
		SessionID:         handoff.ChildSessionID,
		TurnID:            handoff.ChildTurnID,
		AgentID:           childDefinition.ID,
		UserText:          handoff.Task,
		Fragments:         fragments,
		ModelRoute:        childModelRoute,
		ThinkingSupported: thinkingSupported,
		ThinkingEnabled:   effectiveThinkingEnabled,
		ThinkingMode:      effectiveThinkingMode,
		AllowedTools:      slices.Clone(effectiveTools),
		WorkflowBudget:    workflowBudget,
	})
	if err != nil {
		r.log("runtime").Error("delegated handoff run failed", err,
			"handoff_id", handoff.HandoffID,
			"child_session_id", handoff.ChildSessionID,
			"child_turn_id", handoff.ChildTurnID,
		)
		if appendErr := r.appendAgentResultForHandoff(ctx, handoff.ParentTurnID, handoff, events.AgentResultPayload{
			HandoffID:      handoff.HandoffID,
			ChildSessionID: handoff.ChildSessionID,
			ChildTurnID:    handoff.ChildTurnID,
			Status:         events.AgentResultStatusFailed,
			Error:          err.Error(),
		}); appendErr != nil {
			return DelegateSessionTurnResult{}, appendErr
		}
		return DelegateSessionTurnResult{}, err
	}
	var childResult RunSessionResult
	if runResult.Status == TurnRunStatusRolled {
		childResult, err = r.continueRolledOverTurn(ctx, runExistingTurnInput{
			SessionID:            handoff.ChildSessionID,
			TurnID:               handoff.ChildTurnID,
			UserText:             handoff.Task,
			AgentID:              childDefinition.ID,
			ThinkingEnabled:      effectiveThinkingEnabled,
			ThinkingMode:         effectiveThinkingMode,
			Fragments:            append([]prompt.Fragment(nil), fragments...),
			AllowedToolsOverride: append([]string(nil), effectiveTools...),
			ModelRouteOverride:   childModelRoute,
			HideAssistantPreview: false,
			DisableAutoReview:    true,
			WorkflowBudget:       workflowBudget,
		}, resolvedTurnCapabilities{TurnCapabilities: capabilities}, effectiveThinkingEnabled, effectiveThinkingMode)
		if err != nil {
			return DelegateSessionTurnResult{}, err
		}
	} else {
		childResult, err = r.finalizeTurnRunResult(ctx, ctx, nil, handoff.ChildSessionID, handoff.ChildTurnID, runResult)
		if err != nil {
			return DelegateSessionTurnResult{}, err
		}
	}
	childResult, resultPayload, reviewPayload, err := r.prepareDelegatedHandoffCompletion(ctx, handoff, childResult, childModelRoute, workflowBudget)
	if err != nil {
		return DelegateSessionTurnResult{}, err
	}
	if err := r.appendAgentResultForHandoff(ctx, handoff.ParentTurnID, handoff, resultPayload); err != nil {
		return DelegateSessionTurnResult{}, err
	}
	if reviewPayload != nil {
		if err := r.appendStructuredDelegatedReview(ctx, handoff, resultPayload.ChildTurnID, *reviewPayload); err != nil {
			return DelegateSessionTurnResult{}, err
		}
	}
	r.log("runtime").Op("delegated handoff completed",
		"handoff_id", handoff.HandoffID,
		"child_session_id", handoff.ChildSessionID,
		"child_turn_id", handoff.ChildTurnID,
		"status", childResult.Status,
		"pending_request_id", childResult.PendingRequestID,
	)

	return DelegateSessionTurnResult{
		HandoffID:      handoff.HandoffID,
		ChildSessionID: handoff.ChildSessionID,
		ChildTurn:      childResult,
	}, nil
}

func handoffFragment(handoff events.AgentHandoffPayload) prompt.Fragment {
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "handoff",
		Label:     "handoff",
		Content: "Delegated work handoff.\n" +
			"Parent session: " + strings.TrimSpace(handoff.ParentSessionID) + "\n" +
			"Parent turn: " + strings.TrimSpace(handoff.ParentTurnID) + "\n" +
			"Parent agent: " + strings.TrimSpace(handoff.ParentAgentID) + "\n" +
			"Task: " + strings.TrimSpace(handoff.Task) + "\n" +
			"Context summary: " + strings.TrimSpace(handoff.ContextSummary),
	}
}

func handoffPromptFragments(handoff events.AgentHandoffPayload) []prompt.Fragment {
	fragments := []prompt.Fragment{handoffFragment(handoff)}
	if fragment, ok := delegatedPlannerPlanOnlyFragment(handoff); ok {
		fragments = append(fragments, fragment)
	}
	if fragment, ok := handoffExplorationFragment(handoff.ExplorationEntries); ok {
		fragments = append(fragments, fragment)
	}
	return fragments
}

func delegatedPlannerPlanOnlyFragment(handoff events.AgentHandoffPayload) (prompt.Fragment, bool) {
	if strings.TrimSpace(handoff.ChildAgentID) != "planner" {
		return prompt.Fragment{}, false
	}
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "delegated-planner-plan-only",
		Label:     "delegated planner",
		Content: "Delegated planner output contract.\n" +
			"You are running as a delegated child planner. Produce the complete plan as assistant text and then finish.\n" +
			"Do not ask the user whether to save, apply, revise, or stop the plan. Do not call `question` or persist plan files.\n" +
			"The parent engineer turn owns showing the plan to the user and asking for approval.",
	}, true
}

func (r *Runtime) appendAgentResultForHandoff(ctx context.Context, parentTurnID string, handoff events.AgentHandoffPayload, payload events.AgentResultPayload) error {
	targets := []events.Draft{
		{
			SessionID: handoff.ParentSessionID,
			TurnID:    parentTurnID,
			Type:      events.TypeAgentResult,
			Payload:   payload,
		},
		{
			SessionID: handoff.ChildSessionID,
			TurnID:    handoff.ChildTurnID,
			Type:      events.TypeAgentResult,
			Payload:   payload,
		},
	}
	for _, draft := range targets {
		if _, err := r.Sessions.append(ctx, draft); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) prepareDelegatedHandoffCompletion(ctx context.Context, handoff events.AgentHandoffPayload, result RunSessionResult, modelRoute provider.ModelRoute, workflowBudget workflowTurnBudget) (RunSessionResult, events.AgentResultPayload, *events.ReviewRecordedPayload, error) {
	payload := agentResultPayload(handoff, result)
	if payload.Status != events.AgentResultStatusCompleted || strings.TrimSpace(handoff.ChildAgentID) != reviewerAgentID {
		return result, payload, nil, nil
	}
	reviewPayload, err := delegatedReviewPayload(handoff, payload.AssistantText)
	if err == nil {
		return result, payload, &reviewPayload, nil
	}
	if logger := r.log("runtime_review"); logger != nil {
		logger.Debug("delegated review output did not validate as structured json",
			"session_id", handoff.ParentSessionID,
			"turn_id", handoff.ParentTurnID,
			"handoff_id", handoff.HandoffID,
			"error", err.Error(),
		)
	}

	repairResult, repairErr := r.repairDelegatedReviewOutput(ctx, handoff, err, modelRoute, workflowBudget)
	if repairErr != nil {
		return RunSessionResult{}, events.AgentResultPayload{}, nil, repairErr
	}
	repairPayload := agentResultPayload(handoff, repairResult)
	if repairPayload.Status != events.AgentResultStatusCompleted {
		if strings.TrimSpace(repairPayload.Error) == "" {
			repairPayload.Error = fmt.Errorf("%w after repair: child turn status %s", ErrDelegatedReviewStructuredOutputInvalid, repairResult.Status).Error()
		}
		repairPayload.Status = events.AgentResultStatusFailed
		repairResult.Status = TurnRunStatusFailed
		repairResult.Error = repairPayload.Error
		return repairResult, repairPayload, nil, nil
	}
	reviewPayload, err = delegatedReviewPayload(handoff, repairPayload.AssistantText)
	if err != nil {
		contractErr := fmt.Errorf("%w after repair: %v", ErrDelegatedReviewStructuredOutputInvalid, err)
		repairPayload.Status = events.AgentResultStatusFailed
		repairPayload.Error = contractErr.Error()
		repairResult.Status = TurnRunStatusFailed
		repairResult.Error = contractErr.Error()
		return repairResult, repairPayload, nil, nil
	}
	return repairResult, repairPayload, &reviewPayload, nil
}

func delegatedReviewPayload(handoff events.AgentHandoffPayload, raw string) (events.ReviewRecordedPayload, error) {
	payload, err := parseStructuredManualReview(raw, strings.TrimSpace(handoff.Task))
	if err != nil {
		return events.ReviewRecordedPayload{}, err
	}
	payload.ReviewID = strings.TrimSpace(handoff.HandoffID)
	payload.SourceHandoffID = strings.TrimSpace(handoff.HandoffID)
	return payload, nil
}

func (r *Runtime) repairDelegatedReviewOutput(ctx context.Context, handoff events.AgentHandoffPayload, validationErr error, modelRoute provider.ModelRoute, workflowBudget workflowTurnBudget) (RunSessionResult, error) {
	repairTurnID := newRuntimeID("turn")
	return r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:            handoff.ChildSessionID,
		TurnID:               repairTurnID,
		UserText:             delegatedReviewRepairUserText(validationErr),
		AgentID:              handoff.ChildAgentID,
		AdditionalFragments:  delegatedReviewRepairPromptFragments(handoff),
		AllowedToolsOverride: []string{},
		ModelRouteOverride:   modelRoute,
		PreserveSessionModel: true,
		HideAssistantPreview: false,
		DisableAutoReview:    true,
		WorkflowBudget:       workflowBudget,
	})
}

func delegatedReviewRepairUserText(validationErr error) string {
	message := "The previous delegated review output was not valid structured JSON."
	if validationErr != nil {
		message += "\nValidation error: " + strings.TrimSpace(validationErr.Error())
	}
	return message + "\nReturn the corrected review as exactly one JSON object and nothing else."
}

func delegatedReviewRepairPromptFragments(handoff events.AgentHandoffPayload) []prompt.Fragment {
	return []prompt.Fragment{{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "delegated-review-repair",
		Label:     "delegated review repair",
		Content: strings.Join([]string{
			"Delegated reviewer repair contract.",
			"The previous answer in this child session failed the structured review contract.",
			"Return exactly one JSON object and nothing else. Do not wrap it in markdown or code fences.",
			"The object must contain `findings`, `overall_correctness`, and `overall_summary`.",
			"`findings` must be an array of objects with `severity`, `path`, `line`, `title`, and `explanation`.",
			"`severity` must be one of `P0`, `P1`, `P2`, or `P3`.",
			"`path` must be workspace-relative and `line` must be a 1-based integer.",
			"`overall_correctness` must be `correct` or `incorrect`.",
			"`overall_summary` must be 1-3 sentences.",
			"If there are no qualifying findings, return `\"findings\": []`.",
			"Do not call tools during this repair turn.",
			"Original delegated review task: " + strings.TrimSpace(handoff.Task),
		}, "\n"),
	}}
}

func (r *Runtime) appendStructuredDelegatedReview(ctx context.Context, handoff events.AgentHandoffPayload, childTurnID string, payload events.ReviewRecordedPayload) error {
	if strings.TrimSpace(childTurnID) == "" {
		childTurnID = handoff.ChildTurnID
	}
	targets := []events.Draft{
		{
			SessionID: handoff.ParentSessionID,
			TurnID:    handoff.ParentTurnID,
			Type:      events.TypeReviewRecorded,
			Payload:   payload,
		},
		{
			SessionID: handoff.ChildSessionID,
			TurnID:    childTurnID,
			Type:      events.TypeReviewRecorded,
			Payload:   payload,
		},
	}
	for _, draft := range targets {
		if _, err := r.Sessions.append(ctx, draft); err != nil {
			return err
		}
	}
	return nil
}

func agentResultPayload(handoff events.AgentHandoffPayload, result RunSessionResult) events.AgentResultPayload {
	childTurnID := strings.TrimSpace(result.TurnID)
	if childTurnID == "" {
		childTurnID = handoff.ChildTurnID
	}
	payload := events.AgentResultPayload{
		HandoffID:      handoff.HandoffID,
		ChildSessionID: handoff.ChildSessionID,
		ChildTurnID:    childTurnID,
		AssistantText:  result.AssistantText,
	}
	switch result.Status {
	case TurnRunStatusPending:
		switch {
		case result.PendingExecution != nil:
			payload.Status = events.AgentResultStatusPendingPermission
			payload.PermissionRequestID = result.PendingRequestID
			payload.PermissionKind = events.PermissionRequestKindExecution
			payload.PermissionToolName = result.PendingExecution.ToolName
			payload.PermissionDir = result.PendingExecution.WorkingDirectory
			payload.PermissionCommand = result.PendingExecution.Command
			payload.PermissionReason = result.PendingExecution.Reason
			copyPending := *result.PendingExecution
			payload.ExecutionApproval = &copyPending
		case result.PendingPermission != nil:
			payload.Status = events.AgentResultStatusPendingPermission
			payload.PermissionRequestID = result.PendingRequestID
			payload.PermissionKind = result.PendingPermission.Kind
			payload.PermissionToolName = result.PendingPermission.ToolName
			payload.PermissionAccess = result.PendingPermission.Access
			payload.PermissionPath = result.PendingPermission.Path
			payload.PermissionDir = result.PendingPermission.WorkingDirectory
			payload.PermissionCommand = result.PendingPermission.Command
			payload.PermissionReason = result.PendingPermission.Reason
		case result.PendingQuestion != nil:
			payload.Status = events.AgentResultStatusPendingQuestion
			payload.QuestionRequestID = result.PendingRequestID
			payload.QuestionToolName = result.PendingQuestion.ToolName
			payload.QuestionText = result.PendingQuestion.Question
			payload.QuestionOptions = append([]string(nil), result.PendingQuestion.Options...)
		default:
			payload.Status = events.AgentResultStatusFailed
			payload.Error = "child turn is pending without a resolvable request"
		}
	case TurnRunStatusFailed:
		payload.Status = events.AgentResultStatusFailed
		payload.Error = result.Error
	default:
		payload.Status = events.AgentResultStatusCompleted
	}
	return payload
}
