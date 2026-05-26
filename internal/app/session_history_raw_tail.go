package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

const (
	sessionHistoryProtectedRawTurnCount      = 1
	sessionHistoryPrunableToolResultMinBytes = 512
)

type sessionConversationInputShape struct {
	Inputs        []provider.Input
	RawInputBytes int
	PrunedTurns   []sessionHistoryPrunedTurn
}

type sessionHistoryPrunedCall struct {
	CallID   string
	ToolName string
}

type sessionHistoryPrunedTurn struct {
	TurnID string
	Calls  []sessionHistoryPrunedCall
	Paths  []string
}

func buildSessionConversationInputsWithBudget(
	order []string,
	turns map[string]*replayedSessionTurn,
	compaction *events.SessionHistoryContinuationUpdatedPayload,
	budget sessionHistoryBudget,
) sessionConversationInputShape {
	inputs, rawBytes, prunedTurns := buildSessionReplayInputsWithBudgetForRequest(context.Background(), order, turns, budget, nil, nil)
	conversation := sessionConversation{
		Inputs:       append([]provider.Input(nil), inputs...),
		Continuation: cloneCompactionPayload(compaction),
	}
	applySessionConversationCompactionInput(&conversation, budget.SummaryBudgetBytes)
	return sessionConversationInputShape{
		Inputs:        conversation.Inputs,
		RawInputBytes: rawBytes,
		PrunedTurns:   prunedTurns,
	}
}

func buildSessionReplayInputsWithBudgetForRequest(
	ctx context.Context,
	order []string,
	turns map[string]*replayedSessionTurn,
	budget sessionHistoryBudget,
	pageInSelections map[string]sessionHistoryPageInSelection,
	blobs ToolResultBlobStore,
) ([]provider.Input, int, []sessionHistoryPrunedTurn) {
	replayInputs := make(map[string][]provider.Input, len(order))
	totalBytes := 0
	for _, turnID := range order {
		turn := turns[turnID]
		if turn == nil {
			continue
		}
		inputs := buildSessionTurnReplayInputsForRequest(ctx, blobs, turn, pageInSelections, turnID)
		replayInputs[turnID] = inputs
		totalBytes += replayedInputsBytes(inputs)
	}

	timeline := buildSessionReplayTimeline(order, turns)
	for _, entry := range timeline {
		if entry.TurnID != "" {
			continue
		}
		totalBytes += replayedInputBytes(sessionReplayTimelineNoteInput(entry.Note))
	}

	prunedTurns := make([]sessionHistoryPrunedTurn, 0, len(order))
	if budget.RawTailBudgetBytes > 0 && totalBytes > budget.RawTailBudgetBytes {
		protectedStart := max(len(order)-sessionHistoryProtectedRawTurnCount, 0)
		for index, turnID := range order {
			if totalBytes <= budget.RawTailBudgetBytes || index >= protectedStart {
				break
			}
			if _, exact := pageInSelections[turnID]; exact {
				continue
			}
			inputs := replayInputs[turnID]
			if len(inputs) == 0 {
				continue
			}
			prunedCalls := make([]sessionHistoryPrunedCall, 0, len(inputs))
			for inputIndex, input := range inputs {
				replacement, ok := pruneRetainedRawToolResultInput(turnID, input)
				if !ok {
					continue
				}
				previousBytes := replayedInputBytes(input)
				nextBytes := replayedInputBytes(replacement)
				if nextBytes >= previousBytes {
					continue
				}
				inputs[inputIndex] = replacement
				totalBytes -= previousBytes - nextBytes
				prunedCalls = appendUniqueSessionHistoryPrunedCall(prunedCalls, sessionHistoryPrunedCall{
					CallID:   strings.TrimSpace(input.CallID),
					ToolName: strings.TrimSpace(input.ToolName),
				})
				if totalBytes <= budget.RawTailBudgetBytes {
					break
				}
			}
			replayInputs[turnID] = inputs
			if len(prunedCalls) > 0 {
				turn := turns[turnID]
				prunedTurns = append(prunedTurns, sessionHistoryPrunedTurn{
					TurnID: turnID,
					Calls:  prunedCalls,
					Paths:  appendUniqueValues(nil, turn.WorkspacePaths),
				})
			}
		}
	}

	finalInputs := make([]provider.Input, 0, len(timeline)*2)
	finalBytes := 0
	for _, entry := range timeline {
		if entry.TurnID == "" {
			input := sessionReplayTimelineNoteInput(entry.Note)
			finalInputs = append(finalInputs, input)
			finalBytes += replayedInputBytes(input)
			continue
		}
		for _, input := range replayInputs[entry.TurnID] {
			finalInputs = append(finalInputs, input)
			finalBytes += replayedInputBytes(input)
		}
	}
	return finalInputs, finalBytes, prunedTurns
}

func buildSessionTurnReplayInputsForRequest(
	ctx context.Context,
	blobs ToolResultBlobStore,
	turn *replayedSessionTurn,
	pageInSelections map[string]sessionHistoryPageInSelection,
	turnID string,
) []provider.Input {
	if turn == nil {
		return nil
	}
	if selection, exact := pageInSelections[turnID]; exact {
		return buildSelectedSessionTurnReplayInputs(ctx, blobs, turn, selection)
	}
	return append([]provider.Input(nil), turn.replayInputs()...)
}

func buildSelectedSessionTurnReplayInputs(
	ctx context.Context,
	blobs ToolResultBlobStore,
	turn *replayedSessionTurn,
	selection sessionHistoryPageInSelection,
) []provider.Input {
	if selection.FullTurn {
		return appendExactSessionTurnSupplementalEvidence(buildExactSessionTurnReplayInputs(ctx, blobs, turn), turn, selection)
	}
	inputs, ok := buildExactSessionTurnReplayFragments(ctx, blobs, turn, selection)
	if ok {
		return inputs
	}
	return appendExactSessionTurnSupplementalEvidence(buildExactSessionTurnReplayInputs(ctx, blobs, turn), turn, selection)
}

func buildExactSessionTurnReplayInputs(ctx context.Context, blobs ToolResultBlobStore, turn *replayedSessionTurn) []provider.Input {
	if turn == nil || len(turn.Inputs) == 0 {
		return nil
	}
	inputs := make([]provider.Input, 0, len(turn.Inputs))
	for _, input := range turn.Inputs {
		switch input.Kind {
		case provider.InputKindToolCall:
			if exact, ok := exactSessionTurnToolCallInput(turn, input); ok {
				inputs = append(inputs, exact)
			}
		case provider.InputKindToolResult:
			if exact, ok := exactSessionTurnToolResultInput(ctx, blobs, turn, input); ok {
				inputs = append(inputs, exact)
			}
		default:
			inputs = append(inputs, input)
		}
	}
	return inputs
}

func buildExactSessionTurnReplayFragments(
	ctx context.Context,
	blobs ToolResultBlobStore,
	turn *replayedSessionTurn,
	selection sessionHistoryPageInSelection,
) ([]provider.Input, bool) {
	if turn == nil || len(turn.Inputs) == 0 {
		return nil, false
	}
	selectedCallIDs, ambiguous := selectedSessionHistoryPageInCallIDs(turn, selection)
	if ambiguous {
		return nil, false
	}
	inputs := make([]provider.Input, 0, len(turn.Inputs))
	for _, input := range turn.Inputs {
		switch input.Kind {
		case provider.InputKindUserMessage:
			if selection.IncludeUserMessage {
				inputs = append(inputs, input)
			}
		case provider.InputKindAssistantMessage:
			if selection.IncludeAssistantMessages {
				inputs = append(inputs, input)
			}
		case provider.InputKindToolCall:
			if _, ok := selectedCallIDs[input.CallID]; !ok || (!selection.IncludeToolCalls && !selection.IncludeToolResults && !selection.IncludeExecutions) {
				continue
			}
			if exact, ok := exactSessionTurnToolCallInput(turn, input); ok {
				inputs = append(inputs, exact)
			}
			if selection.IncludeExecutions {
				if executionInput, ok := exactSessionTurnExecutionInput(turn, input.CallID); ok {
					inputs = append(inputs, executionInput)
				}
			}
		case provider.InputKindToolResult:
			if !selection.IncludeToolResults {
				continue
			}
			if _, ok := selectedCallIDs[input.CallID]; !ok {
				continue
			}
			if exact, ok := exactSessionTurnToolResultInput(ctx, blobs, turn, input); ok {
				inputs = append(inputs, exact)
			}
		}
	}
	inputs = appendExactSessionTurnRuntimeNotes(inputs, turn, selection)
	inputs = appendExactSessionTurnStatus(inputs, turn, selection)
	return inputs, len(inputs) > 0
}

func exactSessionTurnToolCallInput(turn *replayedSessionTurn, input provider.Input) (provider.Input, bool) {
	if turn == nil || strings.TrimSpace(input.CallID) == "" {
		return provider.Input{}, false
	}
	if _, ok := turn.RawToolResults[input.CallID]; !ok {
		return provider.Input{}, false
	}
	return input, true
}

func exactSessionTurnToolResultInput(
	ctx context.Context,
	blobs ToolResultBlobStore,
	turn *replayedSessionTurn,
	input provider.Input,
) (provider.Input, bool) {
	if turn == nil || strings.TrimSpace(input.CallID) == "" {
		return provider.Input{}, false
	}
	raw, ok := turn.RawToolResults[input.CallID]
	if !ok {
		return provider.Input{}, false
	}
	output, errorText := replayToolResultText(ctx, blobs, raw.ToolName, raw.Output, raw.OutputBlob, raw.Error, raw.ErrorBlob, raw.Succeeded)
	exact := providerToolResultInput(raw.CallID, raw.ToolName, raw.ToolKind, output, errorText, raw.Succeeded)
	exact.RetryOfCallID = raw.RetryOfCallID
	exact.ReusedFromCallID = raw.ReusedFromCallID
	exact.ReusedFromSessionID = raw.ReusedFromSessionID
	exact.ReusedFromTurnID = raw.ReusedFromTurnID
	return exact, true
}

func selectedSessionHistoryPageInCallIDs(turn *replayedSessionTurn, selection sessionHistoryPageInSelection) (map[string]struct{}, bool) {
	if turn == nil || (!selection.IncludeToolCalls && !selection.IncludeToolResults && !selection.IncludeExecutions) {
		return nil, false
	}
	selected := make(map[string]struct{})
	if len(selection.CallIDs) > 0 {
		for _, input := range turn.Inputs {
			if input.Kind != provider.InputKindToolCall {
				continue
			}
			callID := strings.TrimSpace(input.CallID)
			if callID == "" || !containsCompactionValue(selection.CallIDs, callID) {
				continue
			}
			selected[callID] = struct{}{}
		}
		return selected, false
	}
	toolCallIDs := sessionHistoryTurnToolCallIDs(turn)
	if len(toolCallIDs) != 1 {
		return nil, len(toolCallIDs) > 1
	}
	selected[toolCallIDs[0]] = struct{}{}
	return selected, false
}

func sessionHistoryTurnToolCallIDs(turn *replayedSessionTurn) []string {
	if turn == nil {
		return nil
	}
	callIDs := make([]string, 0, len(turn.Inputs))
	seen := make(map[string]struct{})
	for _, input := range turn.Inputs {
		if input.Kind != provider.InputKindToolCall {
			continue
		}
		callID := strings.TrimSpace(input.CallID)
		if callID == "" {
			continue
		}
		if _, ok := seen[callID]; ok {
			continue
		}
		seen[callID] = struct{}{}
		callIDs = append(callIDs, callID)
	}
	return callIDs
}

func appendExactSessionTurnSupplementalEvidence(
	inputs []provider.Input,
	turn *replayedSessionTurn,
	selection sessionHistoryPageInSelection,
) []provider.Input {
	if turn == nil {
		return inputs
	}
	selectedCallIDs, _ := selectedSessionHistoryPageInCallIDs(turn, selection)
	if selection.IncludeExecutions {
		for _, input := range turn.Inputs {
			if input.Kind != provider.InputKindToolCall {
				continue
			}
			if _, ok := selectedCallIDs[input.CallID]; !ok {
				continue
			}
			if executionInput, ok := exactSessionTurnExecutionInput(turn, input.CallID); ok {
				inputs = append(inputs, executionInput)
			}
		}
	}
	inputs = appendExactSessionTurnRuntimeNotes(inputs, turn, selection)
	inputs = appendExactSessionTurnStatus(inputs, turn, selection)
	return inputs
}

func appendExactSessionTurnRuntimeNotes(
	inputs []provider.Input,
	turn *replayedSessionTurn,
	selection sessionHistoryPageInSelection,
) []provider.Input {
	if !selection.IncludeRuntimeNotes || turn == nil {
		return inputs
	}
	for _, note := range turn.postTerminalRuntimeNotes() {
		if strings.TrimSpace(note.Content) == "" {
			continue
		}
		inputs = append(inputs, provider.Input{
			Kind:    provider.InputKindAssistantMessage,
			Content: note.Content,
		})
	}
	return inputs
}

func appendExactSessionTurnStatus(
	inputs []provider.Input,
	turn *replayedSessionTurn,
	selection sessionHistoryPageInSelection,
) []provider.Input {
	if !selection.IncludeTurnStatus || turn == nil {
		return inputs
	}
	status := strings.TrimSpace(renderCompactionTurnStatus(turn))
	if status == "" || status == "completed" {
		return inputs
	}
	return append(inputs, provider.Input{
		Kind:    provider.InputKindAssistantMessage,
		Content: "Turn status: " + status,
	})
}

func exactSessionTurnExecutionInput(turn *replayedSessionTurn, callID string) (provider.Input, bool) {
	if turn == nil || strings.TrimSpace(callID) == "" {
		return provider.Input{}, false
	}
	execution := turn.execution(callID)
	if execution == nil {
		return provider.Input{}, false
	}
	command := strings.TrimSpace(execution.CommandPreview)
	if command == "" {
		command = strings.TrimSpace(execution.ToolName)
	}
	if command == "" {
		command = "unknown"
	}
	directory := strings.TrimSpace(execution.WorkingDirectory)
	content := fmt.Sprintf(`Exact execution for call %s: command %q.`, callID, command)
	if directory != "" {
		content += " Working directory: " + directory + "."
	}
	return provider.Input{
		Kind:    provider.InputKindAssistantMessage,
		Content: content,
	}, true
}

func appendUniqueSessionHistoryPrunedCall(existing []sessionHistoryPrunedCall, value sessionHistoryPrunedCall) []sessionHistoryPrunedCall {
	value.CallID = strings.TrimSpace(value.CallID)
	value.ToolName = strings.TrimSpace(value.ToolName)
	if value.CallID == "" || value.ToolName == "" {
		return existing
	}
	for _, current := range existing {
		if current.CallID == value.CallID {
			return existing
		}
	}
	return append(existing, value)
}

func replayedInputsBytes(inputs []provider.Input) int {
	total := 0
	for _, input := range inputs {
		total += replayedInputBytes(input)
	}
	return total
}

func sessionReplayTimelineNoteInput(note replayedSessionRuntimeNote) provider.Input {
	return provider.Input{
		Kind:    provider.InputKindAssistantMessage,
		Content: note.Content,
	}
}

func pruneRetainedRawToolResultInput(turnID string, input provider.Input) (provider.Input, bool) {
	if !retainedRawToolResultPrunable(input) {
		return provider.Input{}, false
	}
	resultBytes := len(input.Output) + len(input.Error)
	if resultBytes < sessionHistoryPrunableToolResultMinBytes {
		return provider.Input{}, false
	}
	copyInput := input
	placeholder := retainedRawToolResultPlaceholder(turnID, input)
	switch {
	case strings.TrimSpace(copyInput.Error) != "":
		copyInput.Output = ""
		copyInput.Error = placeholder
	case strings.TrimSpace(copyInput.Output) != "":
		copyInput.Output = placeholder
		copyInput.Error = ""
	default:
		copyInput.Output = placeholder
	}
	return copyInput, true
}

func retainedRawToolResultPrunable(input provider.Input) bool {
	if input.Kind != provider.InputKindToolResult {
		return false
	}
	switch strings.TrimSpace(input.ToolName) {
	case tool.BashToolName,
		tool.DiagnosticsToolName,
		tool.LocateToolName,
		tool.ReadToolName,
		tool.SearchToolName,
		tool.SymbolsToolName,
		tool.TestToolName,
		tool.WebFetchToolName,
		tool.WebSearchToolName:
		return true
	default:
		return false
	}
}

func retainedRawToolResultPlaceholder(turnID string, input provider.Input) string {
	kind := "output"
	if strings.TrimSpace(input.Error) != "" {
		kind = "error"
	}
	return fmt.Sprintf(
		"[older retained %s from %s %s %s pruned for prompt budget]",
		kind,
		normalizeCompactionArtifactValue(input.ToolName),
		normalizeCompactionArtifactValue(turnID),
		normalizeCompactionArtifactValue(input.CallID),
	)
}
