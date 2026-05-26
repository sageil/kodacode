package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

type turnWorkState struct {
	Summary            turnWorkSummary
	NativeContinuation *turnNativeContinuation
}

type turnWorkSummary struct {
	Objective     string
	Decisions     []string
	TouchedPaths  []string
	CompletedWork []string
	Verification  []string
	Failures      []string
	OpenItems     []string
}

type turnNativeContinuation struct {
	Contract string
	Inputs   []provider.Input
}

var ErrNativeToolContinuationContractUnsupported = errors.New("provider-native tool continuation contract is not implemented for this route")

func cloneTurnWorkState(state turnWorkState) turnWorkState {
	cloned := turnWorkState{
		Summary: turnWorkSummary{
			Objective:     state.Summary.Objective,
			Decisions:     append([]string(nil), state.Summary.Decisions...),
			TouchedPaths:  append([]string(nil), state.Summary.TouchedPaths...),
			CompletedWork: append([]string(nil), state.Summary.CompletedWork...),
			Verification:  append([]string(nil), state.Summary.Verification...),
			Failures:      append([]string(nil), state.Summary.Failures...),
			OpenItems:     append([]string(nil), state.Summary.OpenItems...),
		},
	}
	if state.NativeContinuation != nil {
		cloned.NativeContinuation = &turnNativeContinuation{
			Contract: state.NativeContinuation.Contract,
			Inputs:   cloneProviderInputs(state.NativeContinuation.Inputs),
		}
	}
	return cloned
}

func turnWorkStateFromEventState(state *events.TurnWorkState) turnWorkState {
	if state == nil {
		return turnWorkState{}
	}
	work := turnWorkState{
		Summary: turnWorkSummary{
			Objective:     state.Summary.Objective,
			Decisions:     append([]string(nil), state.Summary.Decisions...),
			TouchedPaths:  append([]string(nil), state.Summary.TouchedPaths...),
			CompletedWork: append([]string(nil), state.Summary.CompletedWork...),
			Verification:  append([]string(nil), state.Summary.Verification...),
			Failures:      append([]string(nil), state.Summary.Failures...),
			OpenItems:     append([]string(nil), state.Summary.OpenItems...),
		},
	}
	if state.NativeContinuation != nil {
		work.NativeContinuation = &turnNativeContinuation{
			Contract: state.NativeContinuation.Contract,
			Inputs:   appendCheckpointInputs(nil, state.NativeContinuation.Slice),
		}
	}
	return work
}

func turnWorkStateFromPayload(payload events.TurnWorkStateUpdatedPayload) *events.TurnWorkState {
	return &events.TurnWorkState{
		Summary: events.TurnWorkStateSummaryState{
			Objective:     payload.Summary.Objective,
			Decisions:     append([]string(nil), payload.Summary.Decisions...),
			TouchedPaths:  append([]string(nil), payload.Summary.TouchedPaths...),
			CompletedWork: append([]string(nil), payload.Summary.CompletedWork...),
			Verification:  append([]string(nil), payload.Summary.Verification...),
			Failures:      append([]string(nil), payload.Summary.Failures...),
			OpenItems:     append([]string(nil), payload.Summary.OpenItems...),
		},
		NativeContinuation: turnNativeContinuationStateFromPayload(payload.NativeContinuation),
	}
}

func turnNativeContinuationStateFromPayload(payload *events.TurnNativeContinuationPayload) *events.TurnNativeContinuationState {
	if payload == nil {
		return nil
	}
	return &events.TurnNativeContinuationState{
		Contract: payload.Contract,
		Slice:    payload.Slice,
	}
}

func turnWorkStatePayload(turnID string, state turnWorkState) events.TurnWorkStateUpdatedPayload {
	payload := events.TurnWorkStateUpdatedPayload{
		Summary: events.TurnWorkStateSummaryPayload{
			Objective:     state.Summary.Objective,
			Decisions:     append([]string(nil), state.Summary.Decisions...),
			TouchedPaths:  append([]string(nil), state.Summary.TouchedPaths...),
			CompletedWork: append([]string(nil), state.Summary.CompletedWork...),
			Verification:  append([]string(nil), state.Summary.Verification...),
			Failures:      append([]string(nil), state.Summary.Failures...),
			OpenItems:     append([]string(nil), state.Summary.OpenItems...),
		},
	}
	if state.NativeContinuation != nil {
		payload.NativeContinuation = &events.TurnNativeContinuationPayload{
			Contract: state.NativeContinuation.Contract,
			Slice:    checkpointTurnPayloadFromInputs(turnID, state.NativeContinuation.Inputs),
		}
	}
	return payload
}

func checkpointTurnPayloadFromInputs(turnID string, inputs []provider.Input) events.SessionHistoryTurnPayload {
	payload := events.SessionHistoryTurnPayload{
		TurnID:            strings.TrimSpace(turnID),
		AssistantEntries:  make([]events.SessionHistoryAssistantEntryPayload, 0, len(inputs)),
		AnthropicThinking: make([]events.SessionHistoryAnthropicThinkingPayload, 0, len(inputs)),
		ToolCalls:         make([]events.SessionHistoryToolCallPayload, 0, len(inputs)),
		ToolResults:       make([]events.SessionHistoryToolResultPayload, 0, len(inputs)),
		EntryOrder:        make([]events.SessionHistoryEntryPayload, 0, len(inputs)),
	}
	for _, input := range inputs {
		switch input.Kind {
		case provider.InputKindUserMessage:
			payload.UserText = input.Content
			payload.UserAttachments = checkpointUserAttachments(input.Attachments)
			payload.EntryOrder = append(payload.EntryOrder, events.SessionHistoryEntryPayload{
				Kind:  string(input.Kind),
				Index: 0,
			})
		case provider.InputKindAssistantMessage:
			payload.AssistantEntries = append(payload.AssistantEntries, events.SessionHistoryAssistantEntryPayload{
				Content: input.Content,
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
				Arguments:              input.Arguments,
				GoogleThoughtSignature: append([]byte(nil), input.GoogleThoughtSignature...),
				OpenAIReasoningContent: input.OpenAIReasoningContent,
			})
			payload.ToolCallCount++
			payload.EntryOrder = append(payload.EntryOrder, events.SessionHistoryEntryPayload{
				Kind:  string(input.Kind),
				Index: len(payload.ToolCalls) - 1,
			})
		case provider.InputKindToolResult:
			payload.ToolResults = append(payload.ToolResults, events.SessionHistoryToolResultPayload{
				CallID:              input.CallID,
				ToolName:            input.ToolName,
				ReusedFromCallID:    input.ReusedFromCallID,
				ReusedFromSessionID: input.ReusedFromSessionID,
				ReusedFromTurnID:    input.ReusedFromTurnID,
				RetryOfCallID:       input.RetryOfCallID,
				Succeeded:           strings.TrimSpace(input.Error) == "",
				Output:              input.Output,
				Error:               input.Error,
			})
			payload.EntryOrder = append(payload.EntryOrder, events.SessionHistoryEntryPayload{
				Kind:  string(input.Kind),
				Index: len(payload.ToolResults) - 1,
			})
		}
	}
	return payload
}

func renderTurnWorkSummaryInput(summary turnWorkSummary) *provider.Input {
	sections := make([]string, 0, 6)
	if objective := strings.TrimSpace(summary.Objective); objective != "" {
		sections = append(sections, "Objective:\n- "+objective)
	}
	appendSection := func(title string, values []string) {
		if len(values) == 0 {
			return
		}
		lines := make([]string, 0, len(values)+1)
		lines = append(lines, title+":")
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			lines = append(lines, "- "+value)
		}
		if len(lines) > 1 {
			sections = append(sections, strings.Join(lines, "\n"))
		}
	}
	appendSection("Decisions", summary.Decisions)
	appendSection("Completed Work", summary.CompletedWork)
	appendSection("Verification", summary.Verification)
	appendSection("Failures", summary.Failures)
	appendSection("Open Items", summary.OpenItems)
	appendSection("Touched Paths", summary.TouchedPaths)
	if len(sections) == 0 {
		return nil
	}
	return &provider.Input{
		Kind:    provider.InputKindAssistantMessage,
		Content: "Active turn summary:\n" + strings.Join(sections, "\n\n"),
	}
}

func firstUserConversationInput(inputs []provider.Input, fallbackText string, fallbackAttachments []provider.Attachment) provider.Input {
	for _, input := range inputs {
		if input.Kind != provider.InputKindUserMessage {
			continue
		}
		return cloneProviderInputs([]provider.Input{input})[0]
	}
	if strings.TrimSpace(fallbackText) == "" && len(fallbackAttachments) == 0 {
		return provider.Input{}
	}
	return provider.Input{
		Kind:        provider.InputKindUserMessage,
		Content:     fallbackText,
		Attachments: cloneProviderAttachments(fallbackAttachments),
	}
}

func turnReplayContinuationInputs(history turnReplay) []provider.Input {
	conversation := cloneProviderInputs(history.Conversation)
	if history.WorkState.NativeContinuation == nil || len(history.WorkState.NativeContinuation.Inputs) == 0 {
		return conversation
	}
	native := cloneProviderInputs(history.WorkState.NativeContinuation.Inputs)
	switch {
	case len(conversation) >= len(native) && providerInputSlicesEqual(conversation[:len(native)], native):
		return conversation
	case len(native) >= len(conversation) && providerInputSlicesEqual(native[:len(conversation)], conversation):
		return native
	default:
		return conversation
	}
}

func (r *TurnRunner) appendTurnWorkStateUpdated(ctx context.Context, sessionID, turnID string, state turnWorkState) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnWorkStateUpdated,
		Payload:   turnWorkStatePayload(turnID, state),
	})
	return err
}

func nativeContinuationContractForRequest(request provider.Request) string {
	if provider.RequiresOpenAIReasoningContentReplay(request) {
		return "openai_reasoning_tool_loop"
	}
	canonical := provider.CanonicalProviderID(request.Model.ProviderID)
	switch {
	case canonical == "anthropic":
		return "anthropic_tool_loop"
	case canonical == "google":
		return "google_tool_loop"
	case nativeToolContinuationUsesOpenAITransport(request.Model.ProviderID):
		return "openai_tool_loop"
	case canonical == "":
		return "unknown_tool_loop"
	default:
		return strings.ReplaceAll(canonical, "-", "_") + "_tool_loop"
	}
}

func nativeToolContinuationUsesOpenAITransport(providerID string) bool {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return false
	}
	canonical := provider.CanonicalProviderID(providerID)
	switch canonical {
	case "anthropic", "google":
		return false
	case "openai":
		return true
	}
	// The runtime only has dedicated native continuation serializers for
	// Anthropic and Google. Non-experimental routes that are not one of those
	// families are routed through the OpenAI/OpenAI-compatible transport.
	if _, experimental := experimentalProviderRegistration(providerID); experimental {
		return false
	}
	return true
}

func nativeToolContinuationContractSupported(contract string) bool {
	switch strings.TrimSpace(contract) {
	case "anthropic_tool_loop", "openai_tool_loop", "google_tool_loop":
		return true
	default:
		return false
	}
}

func (r *TurnRunner) buildTurnWorkState(ctx context.Context, sessionID, turnID string, userInput provider.Input, currentConversation []provider.Input, request provider.Request, loopOpen bool) (turnWorkState, error) {
	work := turnWorkState{}
	err := r.sessions.Inspect(ctx, sessionID, func(state events.SessionState) error {
		turn := state.Turns[turnID]
		if turn == nil {
			return nil
		}
		work.Summary = r.buildTurnWorkSummary(state, turn, userInput)
		if loopOpen {
			work.NativeContinuation = &turnNativeContinuation{
				Contract: nativeContinuationContractForRequest(request),
				Inputs:   cloneProviderInputs(currentConversation),
			}
		}
		return nil
	})
	if err != nil {
		return turnWorkState{}, err
	}
	return work, nil
}

func (r *TurnRunner) buildTurnWorkSummary(state events.SessionState, turn *events.TurnState, userInput provider.Input) turnWorkSummary {
	if turn == nil {
		return turnWorkSummary{}
	}
	summary := turnWorkSummary{}
	if turn.WorkState != nil {
		summary = turnWorkStateFromEventState(turn.WorkState).Summary
	}
	objective := strings.TrimSpace(turn.UserText)
	if objective == "" {
		objective = strings.TrimSpace(summary.Objective)
	}
	if objective == "" {
		objective = strings.TrimSpace(userInput.Content)
	}
	summary.Objective = objective
	if decision := sessionCompactionDecisionCandidate(turn.AssistantText); decision != "" {
		summary.Decisions = appendUniqueValues(summary.Decisions, []string{decision})
	}
	completed, next := extractCompactionAssistantFacts(turn.AssistantText)
	summary.CompletedWork = appendUniqueValues(summary.CompletedWork, completed)
	if next != "" {
		summary.OpenItems = appendUniqueValues(summary.OpenItems, []string{next})
	}
	summary.TouchedPaths = appendUniqueValues(summary.TouchedPaths, r.turnWorkTouchedPaths(state, turn))
	summary.CompletedWork = appendUniqueValues(summary.CompletedWork, r.turnWorkCompletedWork(turn))
	summary.Verification = appendUniqueValues(summary.Verification, r.turnWorkVerification(turn))
	summary.Failures = appendUniqueValues(summary.Failures, r.turnWorkFailures(state, turn))
	return summary
}

func (r *TurnRunner) turnWorkTouchedPaths(state events.SessionState, turn *events.TurnState) []string {
	if turn == nil {
		return nil
	}
	paths := make([]string, 0, len(turn.ToolCallOrder))
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil {
			continue
		}
		if call.WriteMutation != nil {
			paths = appendUniqueValues(paths, []string{call.WriteMutation.Path})
		}
		for _, mutation := range call.WriteMutations {
			paths = appendUniqueValues(paths, []string{mutation.Path})
		}
		for _, resource := range call.ObservedResources {
			paths = appendUniqueValues(paths, []string{resource.Path})
		}
		if r == nil || r.tools == nil {
			continue
		}
		tl := r.tools.tools[call.ToolName]
		if tl == nil {
			continue
		}
		resolved, ok := resolvedToolPathsFromState(state, tl, json.RawMessage(call.Input))
		if ok {
			paths = appendUniqueValues(paths, resolved)
		}
	}
	return paths
}

func (r *TurnRunner) turnWorkCompletedWork(turn *events.TurnState) []string {
	if turn == nil {
		return nil
	}
	out := make([]string, 0, 4)
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil || !call.Completed || !call.Succeeded {
			continue
		}
		if call.WriteMutation != nil {
			action := "Updated"
			if !call.WriteMutation.Existed {
				action = "Created"
			}
			out = appendUniqueValues(out, []string{fmt.Sprintf("%s %s", action, call.WriteMutation.Path)})
		}
		for _, mutation := range call.WriteMutations {
			action := "Updated"
			if !mutation.Existed {
				action = "Created"
			}
			out = appendUniqueValues(out, []string{fmt.Sprintf("%s %s", action, mutation.Path)})
		}
	}
	return out
}

func (r *TurnRunner) turnWorkVerification(turn *events.TurnState) []string {
	if turn == nil {
		return nil
	}
	out := make([]string, 0, 4)
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil || !call.Completed || !call.Succeeded {
			continue
		}
		switch strings.TrimSpace(call.ToolName) {
		case "bash":
			if call.Execution != nil && strings.TrimSpace(call.Execution.CommandPreview) != "" {
				out = appendUniqueValues(out, []string{fmt.Sprintf("Ran %s", call.Execution.CommandPreview)})
			}
		case "diagnostics", "git_status", "git_diff", "git_show":
			out = appendUniqueValues(out, []string{fmt.Sprintf("%s succeeded", call.ToolName)})
		}
	}
	return out
}

func (r *TurnRunner) turnWorkFailures(state events.SessionState, turn *events.TurnState) []string {
	if turn == nil {
		return nil
	}
	type failureSummary struct {
		Display string
		Count   int
	}
	failures := make(map[string]failureSummary)
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil || !call.Completed || call.Succeeded {
			continue
		}
		key := providerStepFailedToolFingerprint(r.tools, call.ToolName, call.Input, call.FailureClass)
		if strings.TrimSpace(key) == "" {
			key = strings.TrimSpace(call.ToolName)
		}
		summary := failures[key]
		if summary.Display == "" {
			summary.Display = r.turnWorkFailureDisplay(state, call)
		}
		summary.Count++
		failures[key] = summary
	}
	if len(failures) == 0 {
		return nil
	}
	out := make([]string, 0, len(failures))
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil || !call.Completed || call.Succeeded {
			continue
		}
		key := providerStepFailedToolFingerprint(r.tools, call.ToolName, call.Input, call.FailureClass)
		if strings.TrimSpace(key) == "" {
			key = strings.TrimSpace(call.ToolName)
		}
		summary, ok := failures[key]
		if !ok || strings.TrimSpace(summary.Display) == "" {
			continue
		}
		display := summary.Display
		if summary.Count > 1 {
			display = fmt.Sprintf("%s (%dx)", display, summary.Count)
		}
		out = appendUniqueValues(out, []string{display})
		delete(failures, key)
	}
	return out
}

func (r *TurnRunner) turnWorkFailureDisplay(state events.SessionState, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	subject := strings.TrimSpace(call.ToolName)
	if call.WriteMutation != nil && strings.TrimSpace(call.WriteMutation.Path) != "" {
		subject = fmt.Sprintf("%s %s", subject, call.WriteMutation.Path)
	} else if r != nil && r.tools != nil {
		if tl := r.tools.tools[call.ToolName]; tl != nil {
			if resolved, ok := resolvedToolPathsFromState(state, tl, json.RawMessage(call.Input)); ok && len(resolved) > 0 {
				subject = fmt.Sprintf("%s %s", subject, strings.Join(resolved, ", "))
			}
		}
	}
	if call.Execution != nil && strings.TrimSpace(call.Execution.CommandPreview) != "" {
		subject = fmt.Sprintf("%s %s", subject, call.Execution.CommandPreview)
	}
	failureClass := strings.TrimSpace(call.FailureClass)
	if failureClass == "" {
		failureClass = "failed"
	}
	return fmt.Sprintf("%s %s", subject, failureClass)
}
