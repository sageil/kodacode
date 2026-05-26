package app

import (
	"context"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

type sessionConversationRequest struct {
	SessionID       string
	TurnID          string
	ModelRoute      provider.ModelRoute
	Instructions    string
	RequestTemplate *provider.Request
	Tools           []provider.Tool
	CurrentInputs   []provider.Input
}

type turnSessionConversationState struct {
	Pruning             *events.ContextPrunedPayload
	Continuation        *events.SessionHistoryContinuationUpdatedPayload
	DurablePruning      *events.ContextPrunedPayload
	DurableContinuation *events.SessionHistoryContinuationUpdatedPayload
	CompactionFailed    bool
}

func (r sessionConversationRequest) providerRequest() provider.Request {
	if r.RequestTemplate != nil {
		request := *r.RequestTemplate
		request.Inputs = nil
		return request
	}
	return provider.Request{
		SessionID:       r.SessionID,
		TurnID:          r.TurnID,
		AgentID:         "session-history",
		Model:           r.ModelRoute.Primary,
		MaxOutputTokens: 0,
		Instructions:    r.Instructions,
		Tools:           append([]provider.Tool(nil), r.Tools...),
	}
}

func (r *TurnRunner) prepareTurnConversationHistory(
	ctx context.Context,
	input sessionConversationRequest,
	existing turnSessionConversationState,
	template sessionHistoryState,
	checkpointLoaded bool,
	replayedCount int,
) (sessionConversation, turnSessionConversationState, error) {
	state := cloneTurnSessionConversationState(existing)
	effectiveTemplate := template
	if sessionCompactionAhead(template.CompletedOrder, state.Continuation, template.ExistingContinuation) {
		effectiveTemplate.ExistingContinuation = cloneCompactionPayload(state.Continuation)
	}
	history := r.projectSessionHistoryStateForRequest(ctx, input, effectiveTemplate, checkpointLoaded, replayedCount)
	if state.DurableContinuation == nil {
		state.DurableContinuation = cloneCompactionPayload(history.ExistingContinuation)
	}
	budget := resolveSessionHistoryBudget(input.ModelRoute, r.models, r.sessionConfig)
	summaryBudgetBytes := budget.SummaryBudgetBytes
	if history.Conversation.Continuation == nil && state.Continuation != nil {
		history.Conversation.Continuation = cloneCompactionPayload(state.Continuation)
		applySessionConversationCompactionInput(&history.Conversation, summaryBudgetBytes)
	}

	if plan := history.PendingCompaction; plan != nil {
		projectedPayload := buildSessionCompactionProjectionPayload(history.ExistingContinuation, plan, history.Turns, budget.SummaryBudgetBytes)
		seedTurnSessionCompactionFromHistory(&state, history.ExistingContinuation, projectedPayload)
		switch {
		case state.CompactionFailed:
			fallback := currentSessionCompactionFallback(state, &history)
			restoreSessionCompactionFallback(&history, fallback, budget)
			state.Continuation = cloneCompactionPayload(fallback)
		case sameSessionCompactionScope(state.Continuation, projectedPayload):
			reused := reuseSessionCompactionArtifact(state.Continuation, projectedPayload)
			reuseReason := "same_scope_reuse_durable_artifact"
			if sameSessionCompactionArtifact(state.Continuation, projectedPayload) {
				reuseReason = "exact_artifact_match"
			}
			r.logSessionCompactionReuse(input.SessionID, input.TurnID, reuseReason, reused)
			history.Conversation.Continuation = reused
			applySessionConversationCompactionInput(&history.Conversation, summaryBudgetBytes)
			state.Continuation = cloneCompactionPayload(reused)
		default:
			if err := r.realizeSessionHistoryCompaction(ctx, input, &history, plan); err != nil {
				fallback := currentSessionCompactionFallback(state, &history)
				restoreSessionCompactionFallback(&history, fallback, budget)
				state.Continuation = cloneCompactionPayload(fallback)
				state.CompactionFailed = true
				break
			}
			state.Continuation = cloneCompactionPayload(history.Conversation.Continuation)
		}
		if state.Continuation == nil {
			state.Continuation = cloneCompactionPayload(history.Conversation.Continuation)
		}
	} else if state.Continuation == nil {
		state.Continuation = cloneCompactionPayload(history.Conversation.Continuation)
	}
	state.Pruning = cloneContextPrunedPayload(&history.Conversation.Pruning)
	return history.Conversation, state, nil
}

func cloneTurnSessionConversationState(state turnSessionConversationState) turnSessionConversationState {
	return turnSessionConversationState{
		Pruning:             cloneContextPrunedPayload(state.Pruning),
		Continuation:        cloneCompactionPayload(state.Continuation),
		DurablePruning:      cloneContextPrunedPayload(state.DurablePruning),
		DurableContinuation: cloneCompactionPayload(state.DurableContinuation),
		CompactionFailed:    state.CompactionFailed,
	}
}

func (r *TurnRunner) commitTurnSessionConversationState(ctx context.Context, sessionID, turnID string, state *turnSessionConversationState) error {
	if r == nil || state == nil {
		return nil
	}
	if state.Continuation != nil && !sameSessionCompactionArtifact(state.DurableContinuation, state.Continuation) {
		if err := r.appendSessionHistoryContinuationUpdated(ctx, sessionID, turnID, *state.Continuation); err != nil {
			return err
		}
	}
	if state.Pruning != nil && !sameSessionPruningArtifact(state.DurablePruning, state.Pruning) {
		if err := r.appendContextPruned(ctx, sessionID, turnID, *state.Pruning); err != nil {
			return err
		}
	}
	if state.Continuation != nil || state.DurableContinuation == nil {
		state.DurableContinuation = cloneCompactionPayload(state.Continuation)
	}
	if state.Pruning != nil || state.DurablePruning == nil {
		state.DurablePruning = cloneContextPrunedPayload(state.Pruning)
	}
	return nil
}

func currentSessionCompactionFallback(state turnSessionConversationState, history *sessionHistoryState) *events.SessionHistoryContinuationUpdatedPayload {
	if state.Continuation != nil {
		return cloneCompactionPayload(state.Continuation)
	}
	if history != nil && history.ExistingContinuation != nil {
		return cloneCompactionPayload(history.ExistingContinuation)
	}
	return nil
}

func restoreSessionCompactionFallback(history *sessionHistoryState, fallback *events.SessionHistoryContinuationUpdatedPayload, budget sessionHistoryBudget) {
	if history == nil {
		return
	}
	rawTokens := history.EstimatedTokens
	rawOrder := sessionHistoryRawOrder(history.CompletedOrder, fallback)
	inputShape := buildSessionConversationInputsWithBudget(rawOrder, history.Turns, fallback, budget)
	inputs, rawInputBytes := inputShape.Inputs, inputShape.RawInputBytes
	preserveSessionCompactionMeasurement(history, fallback, rawTokens, history.RequestTokenSource)
	applySessionConversationShape(history, rawOrder, inputs, rawInputBytes, fallback, budget.SummaryBudgetBytes)
}

func seedTurnSessionCompactionFromHistory(state *turnSessionConversationState, existing, payload *events.SessionHistoryContinuationUpdatedPayload) {
	if state == nil || state.Continuation != nil {
		return
	}
	if !sameSessionCompactionScope(existing, payload) {
		return
	}
	state.Continuation = cloneCompactionPayload(payload)
}

func reuseSessionCompactionArtifact(summarySource, payload *events.SessionHistoryContinuationUpdatedPayload) *events.SessionHistoryContinuationUpdatedPayload {
	reused := cloneCompactionPayload(payload)
	if reused == nil {
		return nil
	}
	if !sameSessionCompactionScope(summarySource, reused) {
		return reused
	}
	if summarySource == nil {
		return reused
	}
	reused.Artifact = cloneHistoryCompactionArtifact(summarySource.Artifact)
	reused.ArtifactVersion = summarySource.ArtifactVersion
	reused.RendererVersion = summarySource.RendererVersion
	reused.RenderedSummary = strings.TrimSpace(summarySource.RenderedSummary)
	if source := strings.TrimSpace(summarySource.Attribution.SummarySource); source != "" {
		reused.Attribution.SummarySource = source
	}
	return reused
}

func pruningPayloadFromState(state *events.PruningState) *events.ContextPrunedPayload {
	if state == nil {
		return nil
	}
	return &events.ContextPrunedPayload{
		PriorTurns:          state.PriorTurns,
		PriorInputBytes:     state.PriorInputBytes,
		RawPriorTurns:       state.RawPriorTurns,
		RawInputBytes:       state.RawInputBytes,
		CompactedPriorTurns: state.CompactedPriorTurns,
		CompactedInputBytes: state.CompactedInputBytes,
		OmittedPriorTurns:   state.OmittedPriorTurns,
		OmittedInputBytes:   state.OmittedInputBytes,
	}
}

func continuationPayloadFromState(state *events.HistoryContinuationState) *events.SessionHistoryContinuationUpdatedPayload {
	return cloneCompactionPayload(state)
}

func cloneContextPrunedPayload(payload *events.ContextPrunedPayload) *events.ContextPrunedPayload {
	if payload == nil {
		return nil
	}
	copyPayload := *payload
	return &copyPayload
}

func sameSessionPruningArtifact(a, b *events.ContextPrunedPayload) bool {
	switch {
	case a == nil || b == nil:
		return a == b
	default:
		return a.PriorTurns == b.PriorTurns &&
			a.PriorInputBytes == b.PriorInputBytes &&
			a.RawPriorTurns == b.RawPriorTurns &&
			a.RawInputBytes == b.RawInputBytes &&
			a.CompactedPriorTurns == b.CompactedPriorTurns &&
			a.CompactedInputBytes == b.CompactedInputBytes &&
			a.OmittedPriorTurns == b.OmittedPriorTurns &&
			a.OmittedInputBytes == b.OmittedInputBytes
	}
}

func sameSessionCompactionScope(a, b *events.SessionHistoryContinuationUpdatedPayload) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return strings.TrimSpace(a.UpdateReason) == strings.TrimSpace(b.UpdateReason) &&
		a.ConsolidatedTurnCount == b.ConsolidatedTurnCount &&
		a.NewlyConsolidatedTurnCount == b.NewlyConsolidatedTurnCount &&
		strings.TrimSpace(a.FrontierTurnID) == strings.TrimSpace(b.FrontierTurnID)
}

func sameSessionCompactionArtifact(a, b *events.SessionHistoryContinuationUpdatedPayload) bool {
	if !sameSessionCompactionScope(a, b) {
		return false
	}
	if a == nil || b == nil {
		return a == b
	}
	return sameHistoryCompactionArtifact(a.Artifact, b.Artifact)
}

func sessionCompactionAhead(order []string, current, baseline *events.SessionHistoryContinuationUpdatedPayload) bool {
	return sessionCompactionPrefixCount(order, current) > sessionCompactionPrefixCount(order, baseline)
}

func sameNormalizedStringSet(a, b []string) bool {
	left := appendUniqueValues(nil, a)
	right := appendUniqueValues(nil, b)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}
