package app

import (
	"context"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/workspace"
)

var (
	ErrAssistantCommitOutOfOrder = errors.New("assistant commits must extend prior content")
	ErrMultiplePendingToolCalls  = errors.New("multiple pending tool calls found")
	ErrPendingToolCallNotFound   = errors.New("pending tool call not found")
	ErrPendingRequestIDRequired  = errors.New("pending request id is required")
	ErrPendingRequestNotFound    = errors.New("pending request not found")
	ErrPendingRequestNotResolved = errors.New("pending request is not resolved")
	ErrTurnNotResumable          = errors.New("turn is not resumable")
)

type replayedToolCall struct {
	CallID           string
	ToolName         string
	ToolKind         provider.ToolKind
	Arguments        string
	Execution        *replayedExecution
	DeclaredSequence int64
}

type replayedExecution struct {
	ExecutionID      string
	ToolName         string
	Intent           string
	Effect           string
	CommandPreview   string
	WorkingDirectory string
}

type replayedDelegatedHandoff struct {
	HandoffID    string
	ToolCallID   string
	LastSequence int64
}

type turnReplay struct {
	Conversation               []provider.Input
	AssistantText              string
	WorkState                  turnWorkState
	HistoryReplayAfterSequence int64
	PendingTool                *replayedToolCall
	DelegatedHandoff           *replayedDelegatedHandoff
	PermissionRequest          events.PermissionRequestedPayload
	PermissionDecision         events.PermissionDecision
	PermissionScope            events.PermissionScope
	QuestionRequest            *events.QuestionRequestedPayload
	QuestionAnswer             string
	ExecutionApprovalRequest   *events.ExecutionApprovalRequestedPayload
	ExecutionApprovalDecision  events.ExecutionApprovalDecision
	ExecutionExecPolicy        *events.ExecutionPolicyAmendment
	ExecutionNetworkPolicy     *events.ExecutionNetworkPolicyAmendment
	TemporaryGrants            []workspace.Grant
	TemporaryNetworkTargets    []string
}

func (r *TurnRunner) loadTurnReplay(ctx context.Context, input ResumeTurnInput) (turnReplay, error) {
	if strings.TrimSpace(input.RequestID) == "" {
		return turnReplay{}, ErrPendingRequestIDRequired
	}

	afterSequence := int64(-1)
	checkpoint, err := r.loadLatestSessionHistoryCheckpoint(ctx, input.SessionID)
	if err != nil {
		return turnReplay{}, err
	}
	if checkpoint != nil {
		afterSequence = checkpoint.ThroughSequence
	}
	replayed, err := r.sessions.store.Replay(ctx, events.Query{
		SessionID:     input.SessionID,
		AfterSequence: afterSequence,
		ExcludeTypes:  []events.Type{events.TypePromptCompiled, events.TypeSessionHistoryCheckpoint, events.TypeSessionStateSnapshot},
	})
	if err != nil {
		return turnReplay{}, err
	}
	return buildTurnReplayWithToolResults(ctx, r.sessions.blobs, replayed, input)
}

func buildTurnReplay(replayed []events.Event, input ResumeTurnInput) (turnReplay, error) {
	return buildTurnReplayWithToolResults(context.Background(), nil, replayed, input)
}

func buildTurnReplayWithToolResults(ctx context.Context, blobs ToolResultBlobStore, replayed []events.Event, input ResumeTurnInput) (turnReplay, error) {
	history := turnReplay{}

	var (
		callOrder          []string
		callBatches        [][]string
		committedAssistant string
		turnStartSequence  int64 = -1
		requestFound       bool
		requestResolved    bool
		requestSequence    int64
		turnSeen           bool
		userSeen           bool
	)
	calls := make(map[string]replayedToolCall)
	completed := make(map[string]bool)
	handoffs := make(map[string]replayedDelegatedHandoff)
	requests := make(map[string]events.PermissionRequestedPayload)
	for _, event := range replayed {
		if event.TurnID != input.TurnID {
			continue
		}
		turnSeen = true
		if !event.Ephemeral && (turnStartSequence < 0 || event.Sequence < turnStartSequence) {
			turnStartSequence = event.Sequence
		}

		switch payload := event.Payload.(type) {
		case events.UserMessagePayload:
			if !userSeen {
				history.Conversation = append(history.Conversation, provider.Input{
					Kind:        provider.InputKindUserMessage,
					Content:     payload.Content,
					Attachments: attachmentsFromUserMessagePayload(payload.Attachments),
				})
				userSeen = true
			}
		case events.AssistantCommitPayload:
			if !strings.HasPrefix(payload.Content, committedAssistant) {
				return turnReplay{}, ErrAssistantCommitOutOfOrder
			}
			segment := strings.TrimPrefix(payload.Content, committedAssistant)
			if segment != "" {
				history.Conversation = append(history.Conversation, provider.Input{
					Kind:    provider.InputKindAssistantMessage,
					Content: segment,
				})
			}
			committedAssistant = payload.Content
			history.AssistantText = payload.Content
		case events.AnthropicThinkingCommittedPayload:
			history.Conversation = append(history.Conversation, provider.Input{
				Kind: provider.InputKindAnthropicThinking,
				AnthropicThinking: &provider.AnthropicThinkingBlock{
					Type:      payload.Type,
					Thinking:  payload.Thinking,
					Signature: payload.Signature,
					Data:      payload.Data,
				},
			})
		case events.OpenAIReasoningCommittedPayload:
			history.Conversation = append(history.Conversation, providerOpenAIReasoningInput(payload.Item))
		case events.ToolCallDeclaredPayload:
			call := calls[payload.CallID]
			call.CallID = payload.CallID
			call.ToolName = payload.ToolName
			call.ToolKind = provider.ToolKind(payload.ToolKind)
			call.Arguments = payload.Input
			call.DeclaredSequence = event.Sequence
			calls[payload.CallID] = call
			callOrder = append(callOrder, payload.CallID)
			history.Conversation = append(history.Conversation, providerToolCallInputWithContext(
				payload.CallID,
				payload.ToolName,
				provider.ToolKind(payload.ToolKind),
				payload.Input,
				payload.GoogleThoughtSignature,
				payload.OpenAIReasoningContent,
			))
		case events.ToolCallBatchPayload:
			callBatches = append(callBatches, append([]string(nil), payload.CallIDs...))
			history.Conversation = normalizeToolCallBatch(history.Conversation, payload.CallIDs)
		case events.ExecutionDeclaredPayload:
			call := calls[payload.ToolCallID]
			call.CallID = payload.ToolCallID
			call.ToolName = payload.ToolName
			call.Execution = &replayedExecution{
				ExecutionID:      payload.ExecutionID,
				ToolName:         payload.ToolName,
				Intent:           payload.Intent,
				Effect:           payload.Effect,
				CommandPreview:   payload.CommandPreview,
				WorkingDirectory: payload.WorkingDirectory,
			}
			calls[payload.ToolCallID] = call
		case events.ExecutionBackgroundReadyPayload:
			if note := executionBackgroundReadyNote(calls[payload.ToolCallID].Execution, payload); strings.TrimSpace(note) != "" {
				history.Conversation = append(history.Conversation, provider.Input{
					Kind:    provider.InputKindAssistantMessage,
					Content: note,
				})
			}
		case events.ExecutionBackgroundExitedPayload:
			if note := executionBackgroundExitedNote(calls[payload.ToolCallID].Execution, payload); strings.TrimSpace(note) != "" {
				history.Conversation = append(history.Conversation, provider.Input{
					Kind:    provider.InputKindAssistantMessage,
					Content: note,
				})
			}
		case events.ExecutionBackgroundLostPayload:
			if note := executionBackgroundLostNote(calls[payload.ToolCallID].Execution, payload); strings.TrimSpace(note) != "" {
				history.Conversation = append(history.Conversation, provider.Input{
					Kind:    provider.InputKindAssistantMessage,
					Content: note,
				})
			}
		case events.ToolExecEndPayload:
			completed[payload.CallID] = true
			output, errorText := replayToolResultText(ctx, blobs, payload.ToolName, payload.Output, payload.OutputBlob, payload.Error, payload.ErrorBlob, payload.Successful())
			result := providerToolResultInput(payload.CallID, payload.ToolName, provider.ToolKind(payload.ToolKind), output, errorText, payload.Successful())
			result.RetryOfCallID = payload.RetryOfCallID
			result.ReusedFromCallID = payload.ReusedFromCallID
			result.ReusedFromSessionID = payload.ReusedFromSessionID
			result.ReusedFromTurnID = payload.ReusedFromTurnID
			history.Conversation = append(history.Conversation, result)
		case events.TurnWorkStateUpdatedPayload:
			history.WorkState = turnWorkStateFromEventState(turnWorkStateFromPayload(payload))
		case events.AgentHandoffPayload:
			handoff := handoffs[payload.HandoffID]
			handoff.HandoffID = payload.HandoffID
			handoff.ToolCallID = payload.ToolCallID
			handoff.LastSequence = event.Sequence
			handoffs[payload.HandoffID] = handoff
			if payload.HandoffID == input.RequestID {
				requestFound = true
				requestResolved = true
				requestSequence = event.Sequence
				copyHandoff := handoff
				history.DelegatedHandoff = &copyHandoff
			}
		case events.AgentResultPayload:
			handoff := handoffs[payload.HandoffID]
			handoff.HandoffID = payload.HandoffID
			handoff.LastSequence = event.Sequence
			handoffs[payload.HandoffID] = handoff
			if payload.HandoffID == input.RequestID {
				requestFound = true
				requestResolved = true
				requestSequence = event.Sequence
				copyHandoff := handoff
				history.DelegatedHandoff = &copyHandoff
			}
		case events.PermissionRequestedPayload:
			requests[payload.RequestID] = payload
			if payload.RequestID == input.RequestID {
				requestFound = true
				requestSequence = event.Sequence
				history.PermissionRequest = payload
			}
		case events.ExecutionApprovalRequestedPayload:
			request := payload
			requests[request.RequestID] = events.PermissionRequestedPayload{
				Kind:             events.PermissionRequestKindExecution,
				RequestID:        request.RequestID,
				ExecutionID:      request.ExecutionID,
				ToolCallID:       request.ToolCallID,
				WorkingDirectory: request.WorkingDirectory,
				ToolName:         request.ToolName,
				Command:          request.Command,
				Reason:           request.Reason,
			}
			if request.RequestID == input.RequestID {
				requestFound = true
				requestSequence = event.Sequence
				requestCopy := request
				history.ExecutionApprovalRequest = &requestCopy
			}
		case events.QuestionRequestedPayload:
			if payload.QuestionID == input.RequestID {
				requestFound = true
				requestSequence = event.Sequence
				requestCopy := payload
				history.QuestionRequest = &requestCopy
			}
		case events.PermissionResolvedPayload:
			if payload.Decision == events.PermissionDecisionApproved && payload.Scope == events.PermissionScopeOnce {
				request := requests[payload.RequestID]
				if strings.TrimSpace(request.ExecutionID) != "" {
					history.TemporaryGrants = appendReplayTemporaryGrants(history.TemporaryGrants, replayExecutionTemporaryGrants(request, calls[request.ToolCallID]))
				} else if request.Kind == events.PermissionRequestKindNetwork {
					history.TemporaryNetworkTargets = uniqueStrings(append(history.TemporaryNetworkTargets, request.Path))
				} else {
					history.TemporaryGrants = appendReplayTemporaryGrants(history.TemporaryGrants, replayTemporaryGrants(request, calls[request.ToolCallID]))
				}
			}
			if payload.RequestID == input.RequestID {
				requestResolved = true
				history.PermissionDecision = payload.Decision
				history.PermissionScope = payload.Scope
			}
		case events.ExecutionApprovalResolvedPayload:
			if payload.Decision == events.ExecutionApprovalDecisionAccept {
				request := requests[payload.RequestID]
				history.TemporaryGrants = appendReplayTemporaryGrants(history.TemporaryGrants, replayExecutionTemporaryGrants(request, calls[request.ToolCallID]))
			}
			if payload.RequestID == input.RequestID {
				requestResolved = true
				history.ExecutionApprovalDecision = payload.Decision
				history.ExecutionExecPolicy = payload.AppliedExecPolicy
				history.ExecutionNetworkPolicy = payload.AppliedNetworkPolicy
			}
		case events.QuestionAnsweredPayload:
			if payload.QuestionID == input.RequestID {
				requestResolved = true
				history.QuestionAnswer = payload.Answer
			}
		case events.TurnDonePayload, events.TurnCanceledPayload, events.TurnErrorPayload:
			return turnReplay{}, ErrTurnNotResumable
		}
	}

	if !turnSeen {
		return turnReplay{}, ErrTurnNotResumable
	}
	history.HistoryReplayAfterSequence = turnStartSequence - 1
	if !userSeen && (strings.TrimSpace(input.UserText) != "" || len(input.Attachments) > 0) {
		history.Conversation = prependUserInput(history.Conversation, input.UserText, input.Attachments)
	}
	history.Conversation = normalizeToolCallBatches(history.Conversation, callBatches)
	history.Conversation = normalizePendingToolConversation(history.Conversation)
	if !requestFound {
		return turnReplay{}, ErrPendingRequestNotFound
	}
	if !requestResolved && history.DelegatedHandoff == nil {
		return turnReplay{}, ErrPendingRequestNotResolved
	}

	pendingToolCallID := history.PermissionRequest.ToolCallID
	if history.ExecutionApprovalRequest != nil {
		pendingToolCallID = history.ExecutionApprovalRequest.ToolCallID
	} else if history.QuestionRequest != nil {
		pendingToolCallID = history.QuestionRequest.ToolCallID
		if strings.TrimSpace(pendingToolCallID) == "" {
			return history, nil
		}
	} else if history.DelegatedHandoff != nil {
		pendingToolCallID = history.DelegatedHandoff.ToolCallID
	}
	pending, err := findPendingToolCall(callOrder, calls, completed, requestSequence, pendingToolCallID)
	if err != nil {
		return turnReplay{}, err
	}
	history.PendingTool = pending
	return history, nil
}
