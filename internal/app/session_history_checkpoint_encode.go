package app

import (
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func checkpointTurnPayload(turn *replayedSessionTurn) events.SessionHistoryTurnPayload {
	inputs := turn.replayInputs()
	payload := events.SessionHistoryTurnPayload{
		TurnID:              turn.TurnID,
		WorkspacePaths:      append([]string(nil), turn.WorkspacePaths...),
		RuntimeNotes:        checkpointRuntimeNotes(turn.RuntimeNotes),
		AssistantEntries:    make([]events.SessionHistoryAssistantEntryPayload, 0, len(inputs)),
		AnthropicThinking:   make([]events.SessionHistoryAnthropicThinkingPayload, 0, len(inputs)),
		ToolCalls:           make([]events.SessionHistoryToolCallPayload, 0, len(inputs)),
		ToolResults:         make([]events.SessionHistoryToolResultPayload, 0, len(inputs)),
		Executions:          checkpointExecutions(turn.Executions),
		EntryOrder:          make([]events.SessionHistoryEntryPayload, 0, len(inputs)),
		ToolCallCount:       turn.ToolCallCount,
		TerminalStatus:      turn.TerminalStatus,
		TerminalSequence:    turn.TerminalSequence,
		TerminalError:       turn.TerminalError,
		TerminalRetryable:   turn.TerminalRetryable,
		SuccessfulToolCalls: turn.SuccessfulToolCalls,
		FailedToolCalls:     turn.FailedToolCalls,
		UserText:            turn.UserText,
		UserAttachments:     checkpointUserAttachments(turn.UserAttachments),
		AssistantText:       turn.AssistantText,
		ReasoningText:       turn.ReasoningText,
		ToolNames:           append([]string(nil), turn.ToolNames...),
		FailedToolNames:     append([]string(nil), turn.FailedToolNames...),
	}
	reusedCursor := 0
	for _, input := range inputs {
		switch input.Kind {
		case provider.InputKindUserMessage:
			payload.EntryOrder = append(payload.EntryOrder, events.SessionHistoryEntryPayload{
				Kind:  string(input.Kind),
				Index: 0,
			})
		case provider.InputKindAssistantMessage:
			reused := reusedCursor < len(turn.ReusedResults) && input.Content == turn.ReusedResults[reusedCursor]
			if reused {
				reusedCursor++
			}
			payload.AssistantEntries = append(payload.AssistantEntries, events.SessionHistoryAssistantEntryPayload{
				Content: input.Content,
				Reused:  reused,
			})
			payload.EntryOrder = append(payload.EntryOrder, events.SessionHistoryEntryPayload{
				Kind:  string(input.Kind),
				Index: len(payload.AssistantEntries) - 1,
			})
		case provider.InputKindAnthropicThinking:
			if input.AnthropicThinking == nil {
				continue
			}
			payload.AnthropicThinking = append(payload.AnthropicThinking, events.SessionHistoryAnthropicThinkingPayload{
				Type:      input.AnthropicThinking.Type,
				Thinking:  input.AnthropicThinking.Thinking,
				Signature: input.AnthropicThinking.Signature,
				Data:      input.AnthropicThinking.Data,
			})
			payload.EntryOrder = append(payload.EntryOrder, events.SessionHistoryEntryPayload{
				Kind:  string(input.Kind),
				Index: len(payload.AnthropicThinking) - 1,
			})
		case provider.InputKindOpenAIReasoning:
			if len(input.OpenAIReasoningItem) == 0 {
				continue
			}
			payload.OpenAIReasoning = append(payload.OpenAIReasoning, events.SessionHistoryOpenAIReasoningPayload{
				Item: append([]byte(nil), input.OpenAIReasoningItem...),
			})
			payload.EntryOrder = append(payload.EntryOrder, events.SessionHistoryEntryPayload{
				Kind:  string(input.Kind),
				Index: len(payload.OpenAIReasoning) - 1,
			})
		case provider.InputKindToolCall:
			payload.ToolCalls = append(payload.ToolCalls, events.SessionHistoryToolCallPayload{
				CallID:                 input.CallID,
				ToolName:               input.ToolName,
				ToolKind:               string(inputToolKindOrDefault(input.ToolKind)),
				Arguments:              input.Arguments,
				GoogleThoughtSignature: append([]byte(nil), input.GoogleThoughtSignature...),
				OpenAIReasoningContent: input.OpenAIReasoningContent,
			})
			payload.EntryOrder = append(payload.EntryOrder, events.SessionHistoryEntryPayload{
				Kind:  string(input.Kind),
				Index: len(payload.ToolCalls) - 1,
			})
		case provider.InputKindToolResult:
			result := checkpointToolResultForInput(turn, input)
			payload.ToolResults = append(payload.ToolResults, events.SessionHistoryToolResultPayload{
				CallID:              result.CallID,
				ToolName:            result.ToolName,
				ToolKind:            string(inputToolKindOrDefault(result.ToolKind)),
				ReusedFromCallID:    result.ReusedFromCallID,
				ReusedFromSessionID: result.ReusedFromSessionID,
				ReusedFromTurnID:    result.ReusedFromTurnID,
				RetryOfCallID:       result.RetryOfCallID,
				Succeeded:           result.Succeeded,
				Output:              result.Output,
				Error:               result.Error,
				StructuredResult:    cloneStructuredResult(result.StructuredResult),
				OutputBlob:          cloneToolResultBlobRef(result.OutputBlob),
				ErrorBlob:           cloneToolResultBlobRef(result.ErrorBlob),
			})
			payload.EntryOrder = append(payload.EntryOrder, events.SessionHistoryEntryPayload{
				Kind:  string(input.Kind),
				Index: len(payload.ToolResults) - 1,
			})
		}
	}
	return payload
}

func checkpointRuntimeNotes(notes []replayedSessionRuntimeNote) []events.SessionHistoryRuntimeNotePayload {
	if len(notes) == 0 {
		return nil
	}
	payload := make([]events.SessionHistoryRuntimeNotePayload, 0, len(notes))
	for _, note := range notes {
		payload = append(payload, events.SessionHistoryRuntimeNotePayload{
			Sequence: note.Sequence,
			Content:  note.Content,
		})
	}
	return payload
}

func checkpointUserAttachments(attachments []provider.Attachment) []events.SessionHistoryAttachmentPayload {
	if len(attachments) == 0 {
		return nil
	}
	payload := make([]events.SessionHistoryAttachmentPayload, 0, len(attachments))
	for _, attachment := range attachments {
		payload = append(payload, events.SessionHistoryAttachmentPayload{
			Name:     attachment.Name,
			MIMEType: attachment.MIMEType,
			DataURL:  attachment.DataURL,
		})
	}
	return payload
}

func checkpointExecutions(executions map[string]replayedExecution) []events.SessionHistoryExecutionPayload {
	if len(executions) == 0 {
		return nil
	}
	payload := make([]events.SessionHistoryExecutionPayload, 0, len(executions))
	for callID, execution := range executions {
		payload = append(payload, events.SessionHistoryExecutionPayload{
			CallID:           callID,
			ToolName:         execution.ToolName,
			Intent:           execution.Intent,
			Effect:           execution.Effect,
			CommandPreview:   execution.CommandPreview,
			WorkingDirectory: execution.WorkingDirectory,
		})
	}
	sort.Slice(payload, func(i, j int) bool {
		return payload[i].CallID < payload[j].CallID
	})
	return payload
}

func checkpointToolResultForInput(turn *replayedSessionTurn, input provider.Input) replayedToolResult {
	if turn != nil && turn.RawToolResults != nil {
		if raw, ok := turn.RawToolResults[input.CallID]; ok {
			return raw
		}
	}
	return replayedToolResult{
		CallID:              input.CallID,
		ToolName:            input.ToolName,
		ToolKind:            inputToolKindOrDefault(input.ToolKind),
		RetryOfCallID:       input.RetryOfCallID,
		ReusedFromCallID:    input.ReusedFromCallID,
		ReusedFromSessionID: input.ReusedFromSessionID,
		ReusedFromTurnID:    input.ReusedFromTurnID,
		Output:              input.Output,
		Error:               input.Error,
		Succeeded:           strings.TrimSpace(input.Error) == "" && strings.TrimSpace(input.Output) != "",
	}
}
