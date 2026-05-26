package app

import (
	"context"
	"encoding/json"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/textutil"
)

type sessionConversation struct {
	Inputs             []provider.Input
	Pruning            events.ContextPrunedPayload
	Continuation       *events.SessionHistoryContinuationUpdatedPayload
	RequestTokenSource provider.TokenCountSource
}

type sessionHistoryState struct {
	Conversation         sessionConversation
	CompletedOrder       []string
	Turns                map[string]*replayedSessionTurn
	ThroughSequence      int64
	ExistingContinuation *events.SessionHistoryContinuationUpdatedPayload
	PendingCompaction    *sessionCompactionPlan
	EstimatedTokens      int
	RequestTokenSource   provider.TokenCountSource
	CompactedTokens      int
	CompactionSource     provider.TokenCountSource
}

type sessionHistoryCheckpoint struct {
	ThroughSequence int64
	Continuation    *events.SessionHistoryContinuationUpdatedPayload
	CompletedOrder  []string
	Turns           map[string]*replayedSessionTurn
}

type replayedSessionTurn struct {
	TurnID              string
	Inputs              []provider.Input
	RawToolResults      map[string]replayedToolResult
	Executions          map[string]replayedExecution
	WorkspacePaths      []string
	RuntimeNotes        []replayedSessionRuntimeNote
	ToolCallCount       int
	Terminal            bool
	TerminalSequence    int64
	TerminalStatus      string
	TerminalError       string
	TerminalRetryable   bool
	SuccessfulToolCalls int
	FailedToolCalls     int
	committedAssistant  string
	UserText            string
	UserAttachments     []provider.Attachment
	AssistantText       string
	ReasoningText       string
	reasoning           *textutil.StringAccumulator
	ReusedResults       []string
	ToolNames           []string
	FailedToolNames     []string
}

type replayedToolResult struct {
	CallID              string
	ToolName            string
	ToolKind            provider.ToolKind
	ReusedFromCallID    string
	ReusedFromSessionID string
	ReusedFromTurnID    string
	RetryOfCallID       string
	Output              string
	Error               string
	StructuredResult    json.RawMessage
	OutputBlob          *events.ToolResultBlobRef
	ErrorBlob           *events.ToolResultBlobRef
	Succeeded           bool
}

type replayedSessionRuntimeNote struct {
	Sequence int64
	Content  string
}

func (r *TurnRunner) loadSessionHistoryStateForModel(ctx context.Context, sessionID, currentTurnID string, modelRoute provider.ModelRoute) (sessionHistoryState, error) {
	return r.loadSessionHistoryStateForRequest(ctx, sessionConversationRequest{
		SessionID:  sessionID,
		TurnID:     currentTurnID,
		ModelRoute: modelRoute,
	})
}

func (r *TurnRunner) loadSessionHistoryTemplateForRequest(ctx context.Context, input sessionConversationRequest) (sessionHistoryState, bool, int, error) {
	checkpoint, err := r.loadLatestSessionHistoryCheckpoint(ctx, input.SessionID)
	if err != nil {
		return sessionHistoryState{}, false, 0, err
	}

	afterSequence := int64(-1)
	if checkpoint != nil {
		afterSequence = checkpoint.ThroughSequence
	}
	replayed, err := r.sessions.store.Replay(ctx, events.Query{
		SessionID:     input.SessionID,
		AfterSequence: afterSequence,
		ExcludeTypes:  []events.Type{events.TypePromptCompiled, events.TypeSessionHistoryCheckpoint, events.TypeSessionStateSnapshot},
	})
	if err != nil {
		return sessionHistoryState{}, checkpoint != nil, 0, err
	}
	request := input.providerRequest()
	history, err := buildSessionConversationStateWithBudgetAndResolverAndBlobs(
		ctx,
		r.sessions.blobs,
		replayed,
		input.TurnID,
		checkpoint,
		resolveSessionHistoryBudget(input.ModelRoute, r.models, r.sessionConfig),
		request,
		nil,
		sessionHistoryMutationPathResolverForExecutor(r.tools),
	)
	if err != nil {
		return sessionHistoryState{}, checkpoint != nil, len(replayed), err
	}
	return history, checkpoint != nil, len(replayed), nil
}

func (r *TurnRunner) projectSessionHistoryStateForRequest(
	ctx context.Context,
	input sessionConversationRequest,
	template sessionHistoryState,
	checkpointLoaded bool,
	replayedCount int,
) sessionHistoryState {
	request := input.providerRequest()
	history := sessionHistoryState{
		CompletedOrder:       append([]string(nil), template.CompletedOrder...),
		Turns:                template.Turns,
		ThroughSequence:      template.ThroughSequence,
		ExistingContinuation: cloneCompactionPayload(template.ExistingContinuation),
	}
	budget := resolveSessionHistoryBudget(input.ModelRoute, r.models, r.sessionConfig)
	shapeSessionConversationState(&history, request, input.CurrentInputs, budget)
	r.refreshSessionHistoryCompaction(input, &history)
	r.logSessionHistoryPrepared(input.SessionID, input.TurnID, history, checkpointLoaded, replayedCount)
	return history
}

func (r *TurnRunner) loadSessionHistoryStateForRequest(ctx context.Context, input sessionConversationRequest) (sessionHistoryState, error) {
	template, checkpointLoaded, replayedCount, err := r.loadSessionHistoryTemplateForRequest(ctx, input)
	if err != nil {
		return sessionHistoryState{}, err
	}
	return r.projectSessionHistoryStateForRequest(ctx, input, template, checkpointLoaded, replayedCount), nil
}

func (r *TurnRunner) refreshSessionHistoryCompaction(input sessionConversationRequest, history *sessionHistoryState) {
	if r == nil || history == nil || history.Conversation.Continuation == nil {
		return
	}
	budget := resolveSessionHistoryBudget(input.ModelRoute, r.models, r.sessionConfig)
	if sameSessionCompactionArtifact(history.ExistingContinuation, history.Conversation.Continuation) {
		applySessionConversationCompactionInput(&history.Conversation, budget.SummaryBudgetBytes)
		return
	}
	history.Conversation.Continuation = refreshSessionCompactionPayloadMetadata(
		history.Conversation.Continuation,
		history.ExistingContinuation,
		input.ModelRoute,
		budget,
		history.EstimatedTokens,
		history.CompactedTokens,
		history.CompactionSource,
		history.Conversation.Continuation.Attribution.SummarySource,
	)
	applySessionConversationCompactionInput(&history.Conversation, budget.SummaryBudgetBytes)
}

func (r *TurnRunner) loadLatestSessionHistoryCheckpoint(ctx context.Context, sessionID string) (*sessionHistoryCheckpoint, error) {
	event, ok, err := r.sessions.store.Latest(ctx, events.LatestQuery{
		SessionID: sessionID,
		Types:     []events.Type{events.TypeSessionHistoryCheckpoint},
	})
	if err != nil || !ok {
		return nil, err
	}
	payload, ok := event.Payload.(events.SessionHistoryCheckpointPayload)
	if !ok {
		return nil, nil
	}
	return sessionHistoryCheckpointFromPayloadWithBlobs(ctx, r.sessions.blobs, payload), nil
}
