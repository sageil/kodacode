package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func sessionHistoryCheckpointFromPayload(payload events.SessionHistoryCheckpointPayload) *sessionHistoryCheckpoint {
	return sessionHistoryCheckpointFromPayloadWithBlobs(context.Background(), nil, payload)
}

func sessionHistoryCheckpointFromPayloadWithBlobs(ctx context.Context, blobs ToolResultBlobStore, payload events.SessionHistoryCheckpointPayload) *sessionHistoryCheckpoint {
	checkpoint := &sessionHistoryCheckpoint{
		ThroughSequence: payload.ThroughSequence,
		Continuation:    cloneCompactionPayload(payload.Continuation),
		CompletedOrder:  append([]string(nil), payload.CompletedTurnIDs...),
		Turns:           make(map[string]*replayedSessionTurn, len(payload.Turns)),
	}
	for _, turn := range payload.Turns {
		replayed := replayedTurnFromCheckpointWithBlobs(ctx, blobs, turn)
		checkpoint.Turns[replayed.TurnID] = replayed
	}
	return checkpoint
}

func replayedTurnFromCheckpoint(payload events.SessionHistoryTurnPayload) *replayedSessionTurn {
	return replayedTurnFromCheckpointWithBlobs(context.Background(), nil, payload)
}

func replayedTurnFromCheckpointWithBlobs(ctx context.Context, blobs ToolResultBlobStore, payload events.SessionHistoryTurnPayload) *replayedSessionTurn {
	inputs := make([]provider.Input, 0, len(payload.EntryOrder))
	reusedResults := make([]string, 0, len(payload.AssistantEntries))
	for _, entry := range payload.AssistantEntries {
		if entry.Reused {
			reusedResults = append(reusedResults, entry.Content)
		}
	}
	inputs = normalizePendingToolConversation(appendCheckpointInputsWithBlobs(ctx, blobs, inputs, payload))
	return &replayedSessionTurn{
		TurnID:              payload.TurnID,
		Inputs:              inputs,
		RawToolResults:      rawToolResultsFromCheckpoint(payload),
		Executions:          executionsFromCheckpoint(payload),
		WorkspacePaths:      sessionHistoryMutationPathsFromCheckpoint(payload),
		RuntimeNotes:        sessionRuntimeNotesFromCheckpoint(payload.RuntimeNotes),
		ToolCallCount:       payload.ToolCallCount,
		Terminal:            true,
		TerminalSequence:    payload.TerminalSequence,
		TerminalStatus:      payload.TerminalStatus,
		TerminalError:       payload.TerminalError,
		TerminalRetryable:   payload.TerminalRetryable,
		SuccessfulToolCalls: payload.SuccessfulToolCalls,
		FailedToolCalls:     payload.FailedToolCalls,
		UserText:            payload.UserText,
		UserAttachments:     attachmentsFromCheckpoint(payload.UserAttachments),
		AssistantText:       payload.AssistantText,
		ReasoningText:       payload.ReasoningText,
		ReusedResults:       reusedResults,
		ToolNames:           append([]string(nil), payload.ToolNames...),
		FailedToolNames:     append([]string(nil), payload.FailedToolNames...),
	}
}

func appendCheckpointInputs(inputs []provider.Input, payload events.SessionHistoryTurnPayload) []provider.Input {
	return appendCheckpointInputsWithBlobs(context.Background(), nil, inputs, payload)
}

func appendCheckpointInputsWithBlobs(ctx context.Context, blobs ToolResultBlobStore, inputs []provider.Input, payload events.SessionHistoryTurnPayload) []provider.Input {
	for _, entry := range payload.EntryOrder {
		switch entry.Kind {
		case string(provider.InputKindUserMessage):
			inputs = append(inputs, provider.Input{
				Kind:        provider.InputKindUserMessage,
				Content:     payload.UserText,
				Attachments: attachmentsFromCheckpoint(payload.UserAttachments),
			})
		case string(provider.InputKindAssistantMessage):
			if entry.Index < len(payload.AssistantEntries) {
				inputs = append(inputs, provider.Input{
					Kind:    provider.InputKindAssistantMessage,
					Content: payload.AssistantEntries[entry.Index].Content,
				})
			}
		case string(provider.InputKindAnthropicThinking):
			if entry.Index < len(payload.AnthropicThinking) {
				block := payload.AnthropicThinking[entry.Index]
				inputs = append(inputs, provider.Input{
					Kind: provider.InputKindAnthropicThinking,
					AnthropicThinking: &provider.AnthropicThinkingBlock{
						Type:      block.Type,
						Thinking:  block.Thinking,
						Signature: block.Signature,
						Data:      block.Data,
					},
				})
			}
		case string(provider.InputKindOpenAIReasoning):
			if entry.Index < len(payload.OpenAIReasoning) {
				inputs = append(inputs, providerOpenAIReasoningInput(payload.OpenAIReasoning[entry.Index].Item))
			}
		case string(provider.InputKindToolCall):
			call := payload.ToolCalls[entry.Index]
			inputs = append(inputs, provider.Input{
				Kind:                   provider.InputKindToolCall,
				GoogleThoughtSignature: append([]byte(nil), call.GoogleThoughtSignature...),
				OpenAIReasoningContent: call.OpenAIReasoningContent,
				CallID:                 call.CallID,
				ToolName:               call.ToolName,
				ToolKind:               provider.ToolKind(call.ToolKind),
				Arguments:              call.Arguments,
			})
		case string(provider.InputKindToolResult):
			result := payload.ToolResults[entry.Index]
			inputs = append(inputs, replayedToolResultInput(ctx, blobs, replayedToolResultFromCheckpointPayload(result)))
		}
	}
	return inputs
}

func rawToolResultsFromCheckpoint(payload events.SessionHistoryTurnPayload) map[string]replayedToolResult {
	if len(payload.ToolResults) == 0 {
		return nil
	}
	results := make(map[string]replayedToolResult, len(payload.ToolResults))
	for _, result := range payload.ToolResults {
		results[result.CallID] = replayedToolResultFromCheckpointPayload(result)
	}
	return results
}

func executionsFromCheckpoint(payload events.SessionHistoryTurnPayload) map[string]replayedExecution {
	if len(payload.Executions) == 0 {
		return nil
	}
	executions := make(map[string]replayedExecution, len(payload.Executions))
	for _, execution := range payload.Executions {
		executions[execution.CallID] = replayedExecution{
			ToolName:         execution.ToolName,
			Intent:           execution.Intent,
			Effect:           execution.Effect,
			CommandPreview:   execution.CommandPreview,
			WorkingDirectory: execution.WorkingDirectory,
		}
	}
	return executions
}

func replayedToolResultFromCheckpointPayload(result events.SessionHistoryToolResultPayload) replayedToolResult {
	return replayedToolResult{
		CallID:              result.CallID,
		ToolName:            result.ToolName,
		ToolKind:            provider.ToolKind(result.ToolKind),
		ReusedFromCallID:    result.ReusedFromCallID,
		ReusedFromSessionID: result.ReusedFromSessionID,
		ReusedFromTurnID:    result.ReusedFromTurnID,
		RetryOfCallID:       result.RetryOfCallID,
		Output:              result.Output,
		Error:               result.Error,
		StructuredResult:    cloneStructuredResult(result.StructuredResult),
		OutputBlob:          cloneToolResultBlobRef(result.OutputBlob),
		ErrorBlob:           cloneToolResultBlobRef(result.ErrorBlob),
		Succeeded:           result.Successful(),
	}
}

func replayedToolResultInput(ctx context.Context, blobs ToolResultBlobStore, result replayedToolResult) provider.Input {
	output, errorText := replayToolResultText(ctx, blobs, result.ToolName, result.Output, result.OutputBlob, result.Error, result.ErrorBlob, result.Succeeded)
	input := providerToolResultInput(result.CallID, result.ToolName, result.ToolKind, output, errorText, result.Succeeded)
	input.RetryOfCallID = result.RetryOfCallID
	input.ReusedFromCallID = result.ReusedFromCallID
	input.ReusedFromSessionID = result.ReusedFromSessionID
	input.ReusedFromTurnID = result.ReusedFromTurnID
	return input
}

func attachmentsFromCheckpoint(attachments []events.SessionHistoryAttachmentPayload) []provider.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	converted := make([]provider.Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		converted = append(converted, provider.Attachment{
			Name:     attachment.Name,
			MIMEType: attachment.MIMEType,
			DataURL:  attachment.DataURL,
		})
	}
	return converted
}
