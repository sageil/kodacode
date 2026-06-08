package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/workspace"
)

type turnLoopInput struct {
	SessionID                   string
	TurnID                      string
	AgentID                     string
	Instructions                string
	CacheablePrefix             string
	DynamicSuffix               string
	PromptCompactionTokensSaved int
	ModelRoute                  provider.ModelRoute
	ThinkingSupported           bool
	ThinkingEnabled             bool
	ThinkingMode                string
	AllowedTools                []string
	HistoryReplayAfterSequence  int64
	ContinuationReason          string
	State                       turnLoopState
	TemporaryGrants             []workspace.Grant
	TemporaryNetworkTargets     []string
	ResetProviderRequestBudget  bool
	WorkflowBudget              workflowTurnBudget
	HistoryMode                 turnHistoryMode
}

type turnHistoryMode string

const (
	turnHistoryModeDefault         turnHistoryMode = ""
	turnHistoryModeCurrentTurnOnly turnHistoryMode = "current_turn_only"
)

type turnLoopState struct {
	UserInput           provider.Input
	Conversation        []provider.Input
	AssistantText       string
	LatestToolStepStart int
	WorkState           turnWorkState
}

func cloneTurnLoopState(state turnLoopState) turnLoopState {
	var userInput provider.Input
	if state.UserInput.Kind != "" {
		userInput = cloneProviderInputs([]provider.Input{state.UserInput})[0]
	}
	return turnLoopState{
		UserInput:           userInput,
		Conversation:        cloneProviderInputs(state.Conversation),
		AssistantText:       state.AssistantText,
		LatestToolStepStart: state.LatestToolStepStart,
		WorkState:           cloneTurnWorkState(state.WorkState),
	}
}

type assistantRoundtripOutcome string

const (
	assistantRoundtripOutcomeAssistantDone   assistantRoundtripOutcome = "assistant_done"
	assistantRoundtripOutcomeToolResult      assistantRoundtripOutcome = "tool_result"
	assistantRoundtripOutcomePendingExternal assistantRoundtripOutcome = "pending_external"
	assistantRoundtripOutcomeFailed          assistantRoundtripOutcome = "failed"
)

type assistantRoundtripResult struct {
	Outcome             assistantRoundtripOutcome
	ToolBatchSize       int
	ExecutedTools       int
	FailedTools         int
	TaskWorkflowError   string
	ReusedTools         int
	ToolInteractionSigs []string
	PendingRequestID    string
}

type providerRequestAttemptResult struct {
	Result           assistantRoundtripResult
	Error            error
	RequestStarted   bool
	DurableProgress  bool
	CommittedState   turnLoopState
	CompletionTokens int
	ToolCallStarted  bool
	RouteTrace       provider.RouteTrace
	Duration         time.Duration
	UsageReport      *provider.UsageReport
	FinishReason     provider.FinishReason
	RequestTrace     provider.RequestTrace
}

type turnConversationContextState struct {
	SessionPruning      *events.ContextPrunedPayload
	SessionContinuation *events.SessionHistoryContinuationUpdatedPayload
}

func buildTurnHistoryRequest(input turnLoopInput, baseRequest provider.Request, providerTools []provider.Tool, state turnLoopState) (sessionConversationRequest, []provider.Input, error) {
	currentInputs, err := buildTurnProjectedCurrentConversation(state)
	if err != nil {
		return sessionConversationRequest{}, nil, err
	}
	projectedCurrentInputs, _ := projectNativeCurrentTurnInputs(currentInputs, state.LatestToolStepStart)
	return sessionConversationRequest{
		SessionID:       input.SessionID,
		TurnID:          input.TurnID,
		ModelRoute:      input.ModelRoute,
		Instructions:    input.Instructions,
		RequestTemplate: &baseRequest,
		Tools:           providerTools,
		CurrentInputs:   projectedCurrentInputs,
	}, projectedCurrentInputs, nil
}

func (r *TurnRunner) executeTurnLoop(ctx context.Context, input turnLoopInput) (RunTurnResult, error) {
	state := cloneTurnLoopState(input.State)
	providerRequestIndex, err := r.lastAgentProviderRequestIndex(ctx, input.SessionID, input.TurnID)
	if err != nil {
		return RunTurnResult{}, err
	}
	providerRequestBudgetBase := 0
	if input.ResetProviderRequestBudget {
		providerRequestBudgetBase = providerRequestIndex
	}
	repeatedToolState := repeatedToolLoopState{}
	turnContext, err := r.loadTurnConversationContextState(ctx, input.SessionID, input.TurnID)
	if err != nil {
		return RunTurnResult{}, err
	}
	lastSessionContext := turnSessionConversationState{
		Pruning:             turnContext.SessionPruning,
		Continuation:        turnContext.SessionContinuation,
		DurablePruning:      cloneContextPrunedPayload(turnContext.SessionPruning),
		DurableContinuation: cloneCompactionPayload(turnContext.SessionContinuation),
	}
	var providerTools []provider.Tool
	var toolSurface providerToolSurface
	if r.tools != nil {
		toolSurface = r.tools.providerToolSurfaceAllowedForModel(input.AllowedTools, input.ModelRoute.Primary)
		providerTools = append([]provider.Tool(nil), toolSurface.Tools...)
	}
	baseRequest := buildTurnBaseProviderRequest(input, providerTools, r.models, turnRequestOutputBudgetConfig{
		Overrides: r.modelOverrides,
		Budgets:   r.outputBudgets,
	})
	var historyTemplate sessionHistoryState
	var checkpointLoaded bool
	var replayedCount int
	if input.HistoryMode != turnHistoryModeCurrentTurnOnly {
		historyTemplate, checkpointLoaded, replayedCount, err = r.loadSessionHistoryTemplateForRequest(ctx, sessionConversationRequest{
			SessionID:       input.SessionID,
			TurnID:          input.TurnID,
			ModelRoute:      input.ModelRoute,
			Instructions:    input.Instructions,
			RequestTemplate: &baseRequest,
			Tools:           providerTools,
		})
		if err != nil {
			return RunTurnResult{}, err
		}
	}
	providerRequestLimitDisabled, err := r.sessionProviderRequestLimitDisabled(ctx, input.SessionID)
	if err != nil {
		return RunTurnResult{}, err
	}
	var preparedHistory sessionConversation
	var preparedSessionContext turnSessionConversationState
	var preparedProjection turnProviderRequestProjection
	var preparedInputs []provider.Input
	var prepared bool

	for {
		providerRequestIndex++
		if err := r.enforceBudgetLimit(ctx, input.SessionID, input.WorkflowBudget); err != nil {
			return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, err)
		}
		if limit := r.maxTurnProviderRequestsPerTurn(input.WorkflowBudget); !providerRequestLimitDisabled && limit > 0 && providerRequestIndex-providerRequestBudgetBase > limit {
			requestID, err := r.requestProviderRequestLimitQuestion(ctx, input.SessionID, input.TurnID)
			if err != nil {
				return RunTurnResult{}, err
			}
			return RunTurnResult{
				Status:           TurnRunStatusPending,
				PendingRequestID: requestID,
			}, nil
		}
		assistantCharsBeforeRoundtrip := len(state.AssistantText)
		historyRequest, projectedCurrentInputs, err := buildTurnHistoryRequest(input, baseRequest, providerTools, state)
		if err != nil {
			if errors.Is(err, ErrNativeToolContinuationContractUnsupported) {
				return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, err)
			}
			return RunTurnResult{}, err
		}
		var history sessionConversation
		var sessionContext turnSessionConversationState
		var projection turnProviderRequestProjection
		inputsSignature := cloneProviderInputs(projectedCurrentInputs)
		if prepared && turnInputsEqual(projectedCurrentInputs, preparedInputs) {
			history = preparedHistory
			sessionContext = cloneTurnSessionConversationState(preparedSessionContext)
			projection = preparedProjection
		} else if input.HistoryMode == turnHistoryModeCurrentTurnOnly {
			sessionContext = cloneTurnSessionConversationState(lastSessionContext)
			projection, err = buildTurnProviderRequestProjection(baseRequest, sessionConversation{}, state)
			if err != nil {
				if errors.Is(err, ErrNativeToolContinuationContractUnsupported) {
					return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, err)
				}
				return RunTurnResult{}, err
			}
		} else {
			history, sessionContext, err = r.prepareTurnConversationHistory(ctx, historyRequest, lastSessionContext, historyTemplate, checkpointLoaded, replayedCount)
			if err != nil {
				return RunTurnResult{}, err
			}
			projection, err = buildTurnProviderRequestProjection(baseRequest, history, state)
			if err != nil {
				if errors.Is(err, ErrNativeToolContinuationContractUnsupported) {
					return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, err)
				}
				return RunTurnResult{}, err
			}
		}
		request := projection.Request
		budgetDecision := requestInputBudgetDecisionForProviderRequest(ctx, r.provider, request, r.models, r.sessionConfig)
		deterministicContextTokens := deterministicContextPacketTokensFromProviderRequest(request)
		deterministicContextOmittedTokens := 0
		if requestHasDeterministicContextPacket(request) && (budgetDecision.LimitFailure != nil || budgetDecision.Pressure != nil) {
			if omittedRequest, ok := omitDeterministicContextPacketFromProviderRequest(request); ok {
				deterministicContextOmittedTokens = deterministicContextTokens
				deterministicContextTokens = 0
				request = omittedRequest
				projection.Request = omittedRequest
				budgetDecision = requestInputBudgetDecisionForProviderRequest(ctx, r.provider, request, r.models, r.sessionConfig)
			}
		}
		if budgetDecision.LimitFailure != nil {
			if shouldRollOverTurnForInputLimit(providerRequestIndex, sessionContext, state) {
				return r.rollOverTurnForInputLimit(ctx, input, sessionContext, &historyTemplate)
			}
			if appendErr := r.appendContextCompactionFailed(ctx, input.SessionID, input.TurnID, budgetDecision.LimitFailure.Payload); appendErr != nil {
				return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, errors.Join(budgetDecision.LimitFailure, appendErr))
			}
			return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, budgetDecision.LimitFailure)
		}
		if budgetDecision.Pressure != nil && shouldRollOverTurnForInputLimit(providerRequestIndex, sessionContext, state) {
			return r.rollOverTurnForInputLimit(ctx, input, sessionContext, &historyTemplate)
		}
		if err := request.Validate(); err != nil {
			return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, err)
		}
		if err := r.commitTurnSessionConversationState(ctx, input.SessionID, input.TurnID, &sessionContext); err != nil {
			return RunTurnResult{}, err
		}
		lastSessionContext = cloneTurnSessionConversationState(sessionContext)
		preparedHistory = history
		preparedSessionContext = cloneTurnSessionConversationState(sessionContext)
		preparedProjection = projection
		preparedInputs = inputsSignature
		prepared = true
		attribution := buildTurnProviderRequestAttribution(input.PromptCompactionTokensSaved, toolSurface, history.Continuation, projection.CurrentInputTokensSaved)
		attribution.DeterministicContextTokens = deterministicContextTokens
		attribution.DeterministicContextOmittedTokens = deterministicContextOmittedTokens
		persistTurnProviderRequestProjection(&state, projection)
		inputLimitTokens := 0
		if budget, ok := resolveModelInputBudgetForRequest(request, r.models); ok {
			inputLimitTokens = budget.InputLimitTokens
		}
		roundtrip, err := r.runProviderRequest(ctx, request, attribution, input.SessionID, input.TurnID, providerRequestIndex, inputLimitTokens, append([]workspace.Grant(nil), input.TemporaryGrants...), append([]string(nil), input.TemporaryNetworkTargets...), input.WorkflowBudget, &state)
		if err != nil {
			if provider.IsInputLimitExceeded(err) {
				if shouldRollOverTurnForInputLimit(providerRequestIndex, sessionContext, state) {
					return r.rollOverTurnForInputLimit(ctx, input, sessionContext, &historyTemplate)
				}
				limitFailure := providerInputLimitExceededFailure(request, r.models, r.sessionConfig)
				if limitFailure != nil {
					if appendErr := r.appendContextCompactionFailed(ctx, input.SessionID, input.TurnID, limitFailure.Payload); appendErr != nil {
						return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, errors.Join(err, appendErr))
					}
					return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, err)
				}
				return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, err)
			}
			args := []any{
				"session_id", input.SessionID,
				"turn_id", input.TurnID,
				"provider_request_index", providerRequestIndex,
			}
			args = append(args, providerErrorLogFields(err)...)
			r.logger.Error("provider request failed", err, args...)
			return RunTurnResult{}, err
		}
		assistantDeltaChars := len(state.AssistantText) - assistantCharsBeforeRoundtrip
		repeatedToolState = nextRepeatedToolLoopState(repeatedToolState, roundtrip)
		r.logProviderRoundtripResult(input.SessionID, input.TurnID, input.AgentID, providerRequestIndex, roundtrip, len(state.AssistantText), assistantDeltaChars, repeatedToolState)
		if roundtrip.Outcome != assistantRoundtripOutcomeFailed {
			loopOpen := roundtrip.Outcome == assistantRoundtripOutcomeToolResult || roundtrip.Outcome == assistantRoundtripOutcomePendingExternal
			workState, err := r.buildTurnWorkState(ctx, input.SessionID, input.TurnID, state.UserInput, state.Conversation, request, loopOpen)
			if err != nil {
				return RunTurnResult{}, err
			}
			state.WorkState = workState
			if err := r.appendTurnWorkStateUpdated(ctx, input.SessionID, input.TurnID, state.WorkState); err != nil {
				return RunTurnResult{}, err
			}
		}
		if roundtrip.Outcome == assistantRoundtripOutcomePendingExternal {
			return RunTurnResult{
				Status:           TurnRunStatusPending,
				PendingRequestID: roundtrip.PendingRequestID,
			}, nil
		}
		if roundtrip.Outcome == assistantRoundtripOutcomeFailed {
			return RunTurnResult{Status: TurnRunStatusFailed}, nil
		}
		if roundtrip.Outcome == assistantRoundtripOutcomeAssistantDone {
			if shouldRetryPromiseOnlyContextContinuation(input.ContinuationReason, providerRequestIndex, roundtrip, state.AssistantText) {
				state.WorkState.Summary.OpenItems = appendUniqueValues(state.WorkState.Summary.OpenItems, []string{contextContinuationPromiseRetryOpenItem})
				if err := r.appendTurnWorkStateUpdated(ctx, input.SessionID, input.TurnID, state.WorkState); err != nil {
					return RunTurnResult{}, err
				}
				prepared = false
				continue
			}
			break
		}
		if roundtrip.ExecutedTools == 0 {
			if roundtrip.ReusedTools > 0 {
				if repeatedToolState.Repeated() {
					requestID, err := r.requestLoopResolutionQuestion(ctx, input.SessionID, input.TurnID)
					if err != nil {
						return RunTurnResult{}, err
					}
					return RunTurnResult{
						Status:           TurnRunStatusPending,
						PendingRequestID: requestID,
					}, nil
				}
				continue
			}
			break
		}
		if repeatedToolState.Repeated() {
			requestID, err := r.requestLoopResolutionQuestion(ctx, input.SessionID, input.TurnID)
			if err != nil {
				return RunTurnResult{}, err
			}
			return RunTurnResult{
				Status:           TurnRunStatusPending,
				PendingRequestID: requestID,
			}, nil
		}
	}

	if _, err := r.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeTurnDone,
		Payload:   events.TurnDonePayload{},
	}); err != nil {
		return RunTurnResult{}, err
	}
	if input.HistoryMode != turnHistoryModeCurrentTurnOnly {
		if err := r.appendSessionHistoryCheckpoint(ctx, input.SessionID, input.ModelRoute, lastSessionContext.Continuation, &historyTemplate, input.HistoryReplayAfterSequence); err != nil {
			return RunTurnResult{}, err
		}
	}
	return RunTurnResult{Status: TurnRunStatusCompleted}, nil
}

const contextContinuationPromiseRetryOpenItem = "The context-limit continuation acknowledged the handoff without doing work. Continue the in-progress task now by using available tools, or provide a final answer only if no work remains."

func shouldRetryPromiseOnlyContextContinuation(reason string, providerRequestIndex int, roundtrip assistantRoundtripResult, assistantText string) bool {
	if strings.TrimSpace(reason) != events.TurnContinuationReasonContextLimit {
		return false
	}
	if providerRequestIndex != 1 || roundtrip.Outcome != assistantRoundtripOutcomeAssistantDone || roundtrip.ToolBatchSize != 0 || roundtrip.ExecutedTools != 0 || roundtrip.ReusedTools != 0 {
		return false
	}
	return isPromiseOnlyContinuationText(assistantText)
}

func (r *TurnRunner) lastAgentProviderRequestIndex(ctx context.Context, sessionID, turnID string) (int, error) {
	if r == nil || r.sessions == nil {
		return 0, nil
	}
	state, err := r.sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	turn := state.Turns[turnID]
	if turn == nil {
		return 0, nil
	}
	latest := 0
	for _, attempt := range turn.ProviderAttempts {
		if strings.TrimSpace(attempt.Kind) == string(events.TurnProviderUsageKindUtilityCompaction) {
			continue
		}
		if attempt.Step > latest {
			latest = attempt.Step
		}
	}
	return latest, nil
}

func isPromiseOnlyContinuationText(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = strings.ReplaceAll(normalized, "’", "'")
	normalized = strings.Join(strings.Fields(normalized), " ")
	if normalized == "" {
		return false
	}
	if len(strings.Fields(normalized)) > 80 {
		return false
	}
	for _, marker := range []string{
		"i'll continue",
		"i will continue",
		"i'll proceed",
		"i will proceed",
		"i'll resume",
		"i will resume",
		"continuing now",
		"proceeding now",
		"resuming now",
		"continue and finish",
		"continue with",
		"proceed with",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func shouldRollOverTurnForInputLimit(providerRequestIndex int, sessionContext turnSessionConversationState, state turnLoopState) bool {
	if sessionContext.CompactionFailed {
		return false
	}
	return providerRequestIndex > 1 || state.WorkState.NativeContinuation != nil
}

func (r *TurnRunner) rollOverTurnForInputLimit(ctx context.Context, input turnLoopInput, sessionContext turnSessionConversationState, historyTemplate *sessionHistoryState) (RunTurnResult, error) {
	if err := r.commitTurnSessionConversationState(ctx, input.SessionID, input.TurnID, &sessionContext); err != nil {
		return RunTurnResult{}, err
	}
	if err := r.appendTurnDone(ctx, input.SessionID, input.TurnID); err != nil {
		return RunTurnResult{}, err
	}
	if err := r.appendSessionHistoryCheckpoint(ctx, input.SessionID, input.ModelRoute, sessionContext.Continuation, historyTemplate, input.HistoryReplayAfterSequence); err != nil {
		return RunTurnResult{}, err
	}
	return RunTurnResult{
		Status:         TurnRunStatusRolled,
		RolloverReason: TurnRolloverReasonContextLimit,
	}, nil
}

func (r *TurnRunner) enforceBudgetLimit(ctx context.Context, sessionID string, workflowBudget workflowTurnBudget) error {
	if r == nil || r.sessions == nil {
		return nil
	}
	status, err := r.sessions.BudgetStatus(ctx, sessionID, r.sessionConfig)
	if err != nil {
		return err
	}
	if err := status.ExceededError(); err != nil {
		return err
	}
	return r.enforceWorkflowBudgetLimit(ctx, sessionID, workflowBudget)
}
