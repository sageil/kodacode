package app

import (
	"context"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

type turnStartSessionView struct {
	workspaceRoot            string
	additionalWorkspaceRoots []string
	inspectionProgress       []inspectionProgressPromptEntry
	mcp                      *events.SessionMCPState
	model                    string
	turnOrder                []string
	deterministicContext     map[string]string
}

type runExistingTurnInput struct {
	SessionID            string
	TurnID               string
	UserText             string
	Attachments          []AttachmentInput
	ResolvedAttachments  []provider.Attachment
	AgentID              string
	SkillIDs             []string
	SelectedSkillIDs     []string
	ThinkingEnabled      bool
	ThinkingMode         string
	Fragments            []prompt.Fragment
	AdditionalFragments  []prompt.Fragment
	AllowedToolsOverride []string
	ModelRouteOverride   provider.ModelRoute
	PreserveSessionModel bool
	HideAssistantPreview bool
	DisableAutoReview    bool
	InitialState         *turnLoopState
	Continuation         *runtimeTurnContinuation
}

func (r *Runtime) CreateSession(ctx context.Context, workspaceRoot string) (string, error) {
	return r.CreateSessionWithRoots(ctx, workspaceRoot, nil)
}

func (r *Runtime) CreateSessionWithRoots(ctx context.Context, workspaceRoot string, additionalRoots []string) (string, error) {
	return r.createSessionWithMode(ctx, workspaceRoot, additionalRoots, r.Config.Execution.PermissionMode)
}

func (r *Runtime) createSessionWithMode(ctx context.Context, workspaceRoot string, additionalRoots []string, permissionMode PermissionMode) (string, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return "", ErrWorkspaceRootRequired
	}

	sessionID := newRuntimeID("session")
	event, err := r.Sessions.CreateSession(ctx, CreateSessionInput{
		SessionID:                sessionID,
		WorkspaceRoot:            workspaceRoot,
		AdditionalWorkspaceRoots: append([]string(nil), additionalRoots...),
		PermissionMode:           normalizePermissionMode(permissionMode),
	})
	if err != nil {
		r.log("runtime").Error("session creation failed", err, "session_id", sessionID, "workspace_root", workspaceRoot)
		return "", err
	}
	if err := r.syncSessionModelRoute(ctx, sessionID, r.Config.ModelRoute); err != nil {
		r.log("runtime").Error("session model route persist failed", err, "session_id", sessionID)
		return "", err
	}
	if payload, ok := event.Payload.(events.SessionConfiguredPayload); ok && r.Search != nil {
		r.Search.TrackWorkspace(payload.WorkspaceRoot, searchTrackOptions(r.Config.Search))
	}
	r.log("runtime").Op(
		"session created",
		"session_id", sessionID,
		"workspace_root", workspaceRoot,
		"additional_workspace_roots", additionalRoots,
		"permission_mode", string(normalizePermissionMode(permissionMode)),
	)
	return sessionID, nil
}

func (r *Runtime) runExistingSessionTurn(ctx context.Context, input runExistingTurnInput) (RunSessionResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return RunSessionResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return RunSessionResult{}, ErrTurnIDRequired
	}
	if input.Continuation != nil && input.InitialState == nil {
		return RunSessionResult{}, ErrTurnRolloverStateMissing
	}
	if strings.TrimSpace(input.UserText) == "" && len(input.Attachments) == 0 && input.Continuation == nil {
		return RunSessionResult{}, ErrUserTextRequired
	}

	if r.Tools != nil {
		if err := r.Tools.ResumeBackgroundExecutions(ctx, input.SessionID); err != nil {
			return RunSessionResult{}, err
		}
	}
	view, err := r.loadTurnStartSessionView(ctx, input.SessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	mcpState, err := r.syncSessionMCPState(ctx, input.SessionID, view.workspaceRoot, view.mcp)
	if err != nil {
		return RunSessionResult{}, err
	}
	effectiveThinkingMode := strings.TrimSpace(input.ThinkingMode)
	effectiveThinkingEnabled := input.ThinkingEnabled
	responseStyle := r.Config.Sessions.EffectiveResponseStyle()
	availableSkills, err := r.availableTurnSkills(view.workspaceRoot)
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, input.UserText, nil, err)
	}
	effectiveSkillIDs := skillIDsForTurn(input.UserText, input.SkillIDs, availableSkills)

	capabilities, err := r.resolveTurnCapabilitiesFromState(view.capabilitiesState(), resolveTurnCapabilitiesOptions{
		AgentID:              input.AgentID,
		SkillIDs:             append([]string(nil), effectiveSkillIDs...),
		ModelRouteOverride:   input.ModelRouteOverride,
		AllowedToolsOverride: slices.Clone(input.AllowedToolsOverride),
		StrictModelRoute:     true,
	})
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, input.UserText, nil, err)
	}
	capabilities.AllowedTools = reviewPlanHarnessParentTools(input.AgentID, input.UserText, capabilities.AllowedTools)
	resolvedAttachments := cloneProviderAttachments(input.ResolvedAttachments)
	if len(resolvedAttachments) == 0 {
		resolvedAttachments, err = r.resolveTurnAttachments(view.workspaceRoot, capabilities.ModelRoute, input.Attachments)
		if err != nil {
			return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, input.UserText, nil, err)
		}
	}
	fragments := input.Fragments
	if len(fragments) == 0 {
		fragments, err = defaultTurnFragments(capabilities.definition, view.workspaceRoot, view.additionalWorkspaceRoots, availableSkills, capabilities.selectedSkills, capabilities.AllowedTools, mcpState, r.Memory, responseStyle, r.Config.Execution, view.inspectionProgress)
		if err != nil {
			return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, input.UserText, resolvedAttachments, err)
		}
	}
	if len(input.AdditionalFragments) > 0 {
		fragments = append(fragments, input.AdditionalFragments...)
	}
	if fragment, ok := r.deterministicContextPacketFragment(ctx, deterministicContextPacketRuntimeInput{
		WorkspaceRoot:             view.workspaceRoot,
		AdditionalWorkspaceRoots:  view.additionalWorkspaceRoots,
		UserText:                  input.UserText,
		Route:                     capabilities.ModelRoute,
		IsFirstTurn:               view.isFirstTurn(input.TurnID),
		IsContinuation:            input.Continuation != nil,
		PreviousSentSectionBodies: view.deterministicContext,
	}); ok {
		fragments = append(fragments, fragment)
	}
	if input.Continuation != nil {
		fragments = append(fragments, turnContinuationPromptFragment(*input.Continuation))
	}
	effectiveThinkingMode = capabilities.EffectiveReasoningVariant(effectiveThinkingMode)
	effectiveThinkingEnabled = capabilities.EffectiveThinkingEnabled(effectiveThinkingEnabled)
	thinkingSupported := capabilities.SupportsThinkingOutput()
	selectedSkillIDs := input.SelectedSkillIDs
	if selectedSkillIDs == nil {
		selectedSkillIDs = input.SkillIDs
	}
	r.log("runtime").Op("session turn started",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"agent_id", capabilities.AgentID,
	)
	r.log("runtime").Debug("session turn input",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"agent_id", capabilities.AgentID,
		"skill_ids", effectiveSkillIDs,
		"model", capabilities.ModelRoute.Primary.String(),
		"allowed_tools", capabilities.AllowedTools,
		"fragment_count", len(fragments),
		"user_text_len", len(input.UserText),
		"attachment_count", len(resolvedAttachments),
	)
	if !input.PreserveSessionModel {
		if err := r.syncSessionModelRoute(ctx, input.SessionID, capabilities.ModelRoute); err != nil {
			return RunSessionResult{}, err
		}
	}
	if err := r.Runner.appendTurnConfigured(ctx, input.SessionID, input.TurnID, newTurnConfiguredPayload(capabilities.TurnCapabilities, selectedSkillIDs, input.PreserveSessionModel, effectiveThinkingEnabled, effectiveThinkingMode, responseStyle, input.HideAssistantPreview)); err != nil {
		return RunSessionResult{}, err
	}
	if input.InitialState != nil {
		if err := r.Runner.appendTurnWorkStateUpdated(ctx, input.SessionID, input.TurnID, input.InitialState.WorkState); err != nil {
			return RunSessionResult{}, err
		}
	}
	if input.Continuation != nil {
		if err := r.Runner.appendTurnContinuationStarted(ctx, input.SessionID, input.TurnID, input.Continuation.PreviousTurnID, input.Continuation.Reason, input.InitialState.WorkState.Summary); err != nil {
			return RunSessionResult{}, err
		}
	}
	turnStartAfterSequence, err := r.latestSessionSequence(ctx, input.SessionID)
	if err != nil {
		return RunSessionResult{}, err
	}

	turnCtx, turnHandle, finishTurn, err := r.beginCancelableTurnContext(ctx, input.SessionID, input.TurnID)
	if err != nil {
		return RunSessionResult{}, err
	}
	finishedTurn := false
	defer func() {
		if !finishedTurn {
			finishTurn()
		}
	}()

	if strings.TrimSpace(input.UserText) != "" {
		r.maybeStartSessionTitleGeneration(ctx, input.SessionID, input.TurnID, input.UserText, capabilities.ModelRoute, view.isFirstTurn(input.TurnID))
	}

	result, err := r.Runner.Run(turnCtx, RunTurnInput{
		SessionID:            input.SessionID,
		TurnID:               input.TurnID,
		AgentID:              capabilities.AgentID,
		UserText:             input.UserText,
		Attachments:          resolvedAttachments,
		Fragments:            fragments,
		ModelRoute:           capabilities.ModelRoute,
		ThinkingSupported:    thinkingSupported,
		ThinkingEnabled:      effectiveThinkingEnabled,
		ThinkingMode:         effectiveThinkingMode,
		AllowedTools:         capabilities.AllowedTools,
		InitialState:         input.InitialState,
		ContinuationReason:   continuationReason(input.Continuation),
		SkipUserMessageEvent: input.Continuation != nil && strings.TrimSpace(input.UserText) == "" && len(resolvedAttachments) == 0,
		TurnStartAfterSeq:    turnStartAfterSequence,
	})
	if turnHandle.canceled() {
		loaded, cancelErr := r.loadCanceledSessionTurnResult(input.SessionID, input.TurnID, input.UserText, resolvedAttachments)
		if cancelErr != nil {
			r.log("runtime").Error("session turn cancellation finalize failed", cancelErr,
				"session_id", input.SessionID,
				"turn_id", input.TurnID,
				"agent_id", capabilities.AgentID,
			)
			return RunSessionResult{}, cancelErr
		}
		r.log("runtime").Op("session turn canceled",
			"session_id", loaded.SessionID,
			"turn_id", loaded.TurnID,
			"agent_id", capabilities.AgentID,
			"status", loaded.Status,
		)
		return loaded, nil
	}
	if err != nil {
		if failed, ok, failedErr := r.failedSessionTurnResult(ctx, input.SessionID, input.TurnID); failedErr == nil && ok {
			r.log("runtime").Op("session turn failed",
				"session_id", failed.SessionID,
				"turn_id", failed.TurnID,
				"agent_id", capabilities.AgentID,
				"error", failed.Error,
			)
			return failed, nil
		}
		r.log("runtime").Error("session turn failed", err,
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
			"agent_id", capabilities.AgentID,
		)
		return RunSessionResult{}, err
	}
	if result.Status == TurnRunStatusRolled {
		finishTurn()
		finishedTurn = true
		rolled, rolloverErr := r.continueRolledOverTurn(ctx, input, capabilities, effectiveThinkingEnabled, effectiveThinkingMode)
		if rolloverErr != nil {
			return RunSessionResult{}, rolloverErr
		}
		return rolled, nil
	}

	loaded, err := r.finalizeTurnRunResult(ctx, turnCtx, turnHandle, input.SessionID, input.TurnID, result)
	if err != nil {
		r.log("runtime").Error("session turn result load failed", err,
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
		)
		return RunSessionResult{}, err
	}
	r.log("runtime").Op("session turn completed",
		"session_id", loaded.SessionID,
		"turn_id", loaded.TurnID,
		"status", loaded.Status,
		"pending_request_id", loaded.PendingRequestID,
	)
	if input.DisableAutoReview {
		return loaded, nil
	}
	if reviewed, ok, reviewErr := r.maybeRunAutoReview(ctx, view.workspaceRoot, capabilities.AgentID, loaded); reviewErr != nil {
		return RunSessionResult{}, reviewErr
	} else if ok {
		return reviewed, nil
	}
	return loaded, nil
}

func (r *Runtime) loadTurnStartSessionView(ctx context.Context, sessionID string) (turnStartSessionView, error) {
	var view turnStartSessionView
	if err := r.Sessions.Inspect(ctx, sessionID, func(state events.SessionState) error {
		view = turnStartSessionView{
			workspaceRoot:            state.WorkspaceRoot,
			additionalWorkspaceRoots: append([]string(nil), state.AdditionalWorkspaceRoots...),
			inspectionProgress:       collectCompactedInspectionProgressPromptEntries(&state),
			mcp:                      cloneSessionMCPState(state.MCP),
			model:                    state.Model,
			turnOrder:                append([]string(nil), state.TurnOrder...),
			deterministicContext:     lastSentDeterministicContextPacketSections(state, r.Config.ContextPacket.EnabledSections),
		}
		return nil
	}); err != nil {
		return turnStartSessionView{}, err
	}
	return view, nil
}

func (v turnStartSessionView) capabilitiesState() events.SessionState {
	return events.SessionState{
		WorkspaceRoot: v.workspaceRoot,
		Model:         v.model,
	}
}

func (v turnStartSessionView) isFirstTurn(turnID string) bool {
	return len(v.turnOrder) == 0 && strings.TrimSpace(turnID) != ""
}

func cloneSessionMCPState(state *events.SessionMCPState) *events.SessionMCPState {
	if state == nil {
		return nil
	}
	return &events.SessionMCPState{
		WorkspaceTrusted: state.WorkspaceTrusted,
		Servers:          append([]events.SessionMCPServerPayload(nil), state.Servers...),
		Tools:            append([]events.SessionMCPToolPayload(nil), state.Tools...),
	}
}

func (r *Runtime) syncSessionModelRoute(ctx context.Context, sessionID string, route provider.ModelRoute) error {
	if strings.TrimSpace(route.Primary.ProviderID) == "" || strings.TrimSpace(route.Primary.ModelID) == "" {
		return nil
	}
	_, err := r.Sessions.SetModelRoute(ctx, sessionID, route)
	return err
}

func (r *Runtime) latestSessionSequence(ctx context.Context, sessionID string) (int64, error) {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	return state.LastSequence, nil
}
