package app

import (
	"context"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
)

type ResolveSessionTurnInput struct {
	SessionID              string
	TurnID                 string
	PermissionRequestID    string
	UserText               string
	AgentID                string
	SkillIDs               []string
	AllowedTools           []string
	Fragments              []prompt.Fragment
	Decision               events.PermissionDecision
	Scope                  events.PermissionScope
	GrantPath              string
	Recursive              bool
	ExecutionDecision      events.ExecutionApprovalDecision
	ExecutionExecPolicy    *events.ExecutionPolicyAmendment
	ExecutionNetworkPolicy *events.ExecutionNetworkPolicyAmendment
}

func (r *Runtime) ResolveSessionTurn(ctx context.Context, input ResolveSessionTurnInput) (RunSessionResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return RunSessionResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return RunSessionResult{}, ErrTurnIDRequired
	}
	if strings.TrimSpace(input.PermissionRequestID) == "" {
		return RunSessionResult{}, ErrPermissionRequestMissing
	}
	state, err := r.Sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	turnID := pendingInteractionTurnID(state, input.PermissionRequestID)
	if turnID == "" {
		turnID = strings.TrimSpace(input.TurnID)
	}
	resolvedInput := input
	resolvedInput.TurnID = turnID
	mcpState, err := r.syncSessionMCPState(ctx, input.SessionID, state.WorkspaceRoot, state.MCP)
	if err != nil {
		return RunSessionResult{}, err
	}
	if isLocalShellTurnState(state.Turns[turnID]) {
		return r.resolveLocalShellTurn(ctx, resolvedInput)
	}
	resolved, err := resolveResumeTurn(state, resolvedInput)
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
	thinkingSupported := capabilities.SupportsThinkingOutput()

	if _, err := r.Sessions.ResolvePermission(ctx, ResolvePermissionInput{
		SessionID:              input.SessionID,
		TurnID:                 turnID,
		RequestID:              input.PermissionRequestID,
		Decision:               input.Decision,
		Scope:                  input.Scope,
		GrantPath:              input.GrantPath,
		Recursive:              input.Recursive,
		ExecutionDecision:      input.ExecutionDecision,
		ExecutionExecPolicy:    input.ExecutionExecPolicy,
		ExecutionNetworkPolicy: input.ExecutionNetworkPolicy,
	}); err != nil {
		r.log("runtime").Error("permission resolution failed", err,
			"session_id", input.SessionID,
			"turn_id", turnID,
			"request_id", input.PermissionRequestID,
		)
		return RunSessionResult{}, err
	}
	r.log("runtime").Op("permission resolved",
		"session_id", input.SessionID,
		"turn_id", turnID,
		"request_id", input.PermissionRequestID,
		"decision", input.Decision,
		"scope", input.Scope,
	)
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
		RequestID:                   input.PermissionRequestID,
	})
	if turnHandle.canceled() {
		return r.loadCanceledSessionTurnResult(input.SessionID, turnID, resolved.userText, resolved.attachments)
	}
	if err != nil {
		if failed, ok, failedErr := r.failedSessionTurnResult(ctx, input.SessionID, turnID); failedErr == nil && ok {
			r.log("runtime").Op("session turn failed",
				"session_id", failed.SessionID,
				"turn_id", failed.TurnID,
				"request_id", input.PermissionRequestID,
				"error", failed.Error,
			)
			return failed, nil
		}
		r.log("runtime").Error("session turn resume failed", err,
			"session_id", input.SessionID,
			"turn_id", turnID,
			"request_id", input.PermissionRequestID,
		)
		return RunSessionResult{}, err
	}
	if result.Status == TurnRunStatusRolled {
		hideAssistantPreview := false
		if turn := state.Turns[turnID]; turn != nil && turn.Config != nil {
			hideAssistantPreview = turn.Config.HideAssistantPreview
		}
		finishTurn()
		finishedTurn = true
		rolled, rolloverErr := r.continueRolledOverResumedTurn(
			ctx,
			input.SessionID,
			turnID,
			resolved,
			capabilities,
			effectiveThinkingEnabled,
			effectiveThinkingMode,
			fragments,
			hideAssistantPreview,
			false,
		)
		if rolloverErr != nil {
			return RunSessionResult{}, rolloverErr
		}
		r.log("runtime").Op("session turn resumed",
			"session_id", rolled.SessionID,
			"turn_id", rolled.TurnID,
			"status", rolled.Status,
			"pending_request_id", rolled.PendingRequestID,
		)
		return rolled, nil
	}

	loaded, err := r.finalizeTurnRunResult(ctx, turnCtx, turnHandle, input.SessionID, turnID, result)
	if err != nil {
		r.log("runtime").Error("resumed turn result load failed", err,
			"session_id", input.SessionID,
			"turn_id", turnID,
		)
		return RunSessionResult{}, err
	}
	r.log("runtime").Op("session turn resumed",
		"session_id", loaded.SessionID,
		"turn_id", loaded.TurnID,
		"status", loaded.Status,
		"pending_request_id", loaded.PendingRequestID,
	)
	return loaded, nil
}
