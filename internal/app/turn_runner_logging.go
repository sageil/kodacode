package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func (r *TurnRunner) logProviderRequestStarted(request provider.Request, providerRequestIndex, attempt int) {
	if r == nil || r.logger == nil || !r.logger.DebugEnabled() {
		return
	}
	inputBytes := 0
	for _, input := range request.Inputs {
		inputBytes += replayedInputBytes(input)
	}
	prepared := provider.PreparePromptRequest(request)
	r.logger.Debug("provider request started",
		"session_id", request.SessionID,
		"turn_id", request.TurnID,
		"agent_id", request.AgentID,
		"provider_request_index", providerRequestIndex,
		"attempt", attempt,
		"requested_model", request.Model.String(),
		"model", request.Model.String(),
		"api_mode", "",
		"parallel_tool_calls", len(request.Tools) > 0,
		"input_count", len(request.Inputs),
		"input_bytes", inputBytes,
		"estimated_request_tokens", provider.EstimateRequestTokens(prepared),
		"tool_count", len(request.Tools),
		"instructions_len", len(request.Instructions),
	)
}

func (r *TurnRunner) providerRawSSEObserver(request provider.Request, providerRequestIndex int) provider.RawSSEObserver {
	if r == nil || r.logger == nil || !r.logger.DebugEnabled() {
		return nil
	}
	return func(frame provider.RawSSEFrame) {
		r.logger.Debug("provider raw sse frame",
			"session_id", request.SessionID,
			"turn_id", request.TurnID,
			"agent_id", request.AgentID,
			"provider_request_index", providerRequestIndex,
			"requested_model", request.Model.String(),
			"model", request.Model.String(),
			"api_mode", frame.APIMode,
			"sse_sequence", frame.Sequence,
			"sse_event", frame.Event,
			"sse_data_bytes", len(frame.Data),
			"sse_data", string(frame.Data),
		)
	}
}

func (r *TurnRunner) logProviderRequestCompleted(request provider.Request, providerRequestIndex, attempt int, result providerRequestAttemptResult, estimatedRequestTokens int) {
	if r == nil || r.logger == nil || !r.logger.DebugEnabled() {
		return
	}
	selectedModel := request.Model
	if model, ok := result.RouteTrace.SelectedModel(); ok {
		selectedModel = model
	}
	args := []any{
		"session_id", request.SessionID,
		"turn_id", request.TurnID,
		"agent_id", request.AgentID,
		"provider_request_index", providerRequestIndex,
		"attempt", attempt,
		"requested_model", request.Model.String(),
		"selected_model", selectedModel.String(),
		"model", selectedModel.String(),
		"api_mode", result.RequestTrace.APIMode,
		"parallel_tool_calls", result.RequestTrace.ParallelToolCalls,
		"input_count", len(request.Inputs),
		"input_bytes", providerRequestInputBytes(request),
		"estimated_request_tokens", estimatedRequestTokens,
		"tool_count", len(request.Tools),
		"instructions_len", len(request.Instructions),
		"duration_millis", result.Duration.Milliseconds(),
		"finish_reason", string(result.FinishReason),
		"request_started", result.RequestStarted,
		"durable_progress", result.DurableProgress,
		"outcome", string(result.Result.Outcome),
		"tool_batch_size", result.Result.ToolBatchSize,
		"executed_tools", result.Result.ExecutedTools,
		"failed_tools", result.Result.FailedTools,
		"reused_tools", result.Result.ReusedTools,
	}
	if result.Error != nil {
		args = append(args, "error_class", providerRequestErrorClass(result.Error))
		args = append(args, providerErrorLogFields(result.Error)...)
	}
	r.logger.Debug("provider request completed", args...)
}

func (r *TurnRunner) logProviderRoundtripResult(sessionID, turnID, agentID string, providerRequestIndex int, result assistantRoundtripResult, assistantChars int, assistantDeltaChars int, repeatedToolState repeatedToolLoopState) {
	if r == nil || r.logger == nil || !r.logger.DebugEnabled() {
		return
	}
	r.logger.Debug("assistant roundtrip completed",
		"session_id", sessionID,
		"turn_id", turnID,
		"agent_id", agentID,
		"provider_request_index", providerRequestIndex,
		"outcome", string(result.Outcome),
		"tool_batch_size", result.ToolBatchSize,
		"executed_tools", result.ExecutedTools,
		"failed_tools", result.FailedTools,
		"reused_tools", result.ReusedTools,
		"tool_interactions", len(result.ToolInteractionSigs),
		"repeated_tool_signature", repeatedToolState.Match,
		"pending_request_id", result.PendingRequestID,
		"assistant_chars", assistantChars,
		"assistant_delta_chars", assistantDeltaChars,
	)
}

func providerRequestInputBytes(request provider.Request) int {
	inputBytes := 0
	for _, input := range request.Inputs {
		inputBytes += replayedInputBytes(input)
	}
	return inputBytes
}

func providerRequestErrorClass(err error) string {
	if err == nil {
		return ""
	}
	fields := providerErrorLogFields(err)
	for i := 0; i+1 < len(fields); i += 2 {
		key, _ := fields[i].(string)
		if strings.TrimSpace(key) == "provider_status" {
			return "provider_status"
		}
	}
	return "error"
}

func (r *TurnRunner) logSessionHistoryPrepared(sessionID, turnID string, history sessionHistoryState, checkpointPresent bool, replayedEventCount int) {
	if r == nil {
		return
	}
	args := []any{
		"session_id", sessionID,
		"turn_id", turnID,
		"checkpoint_present", checkpointPresent,
		"replayed_event_count", replayedEventCount,
		"completed_turns", len(history.CompletedOrder),
		"estimated_tokens", history.EstimatedTokens,
		"compacted_tokens", history.CompactedTokens,
	}
	args = append(args, sessionHistoryPruningLogArgs(history.Conversation.Pruning)...)
	args = append(args, sessionHistoryContinuationPayloadLogArgs(history.Conversation.Continuation)...)
	r.logger.Debug("session history prepared", args...)
}

func (r *TurnRunner) logSessionCompactionReuse(sessionID, turnID, reason string, payload *events.SessionHistoryContinuationUpdatedPayload) {
	if r == nil {
		return
	}
	args := []any{
		"session_id", sessionID,
		"turn_id", turnID,
		"reason", reason,
	}
	args = append(args, sessionHistoryContinuationPayloadLogArgs(payload)...)
	r.logger.Debug("session compaction artifact reused", args...)
}

func (r *TurnRunner) logContextPruned(sessionID, turnID string, pruning events.ContextPrunedPayload) {
	if r == nil {
		return
	}
	args := []any{
		"session_id", sessionID,
		"turn_id", turnID,
	}
	args = append(args, sessionHistoryPruningLogArgs(pruning)...)
	r.logger.Debug("session history pruned appended", args...)
}

func (r *TurnRunner) logContextCompactionStarted(sessionID, turnID string, compaction events.ContextCompactionStartedPayload) {
	if r == nil {
		return
	}
	r.logger.Debug("session history compaction started appended",
		"session_id", sessionID,
		"turn_id", turnID,
		"scope", compaction.Scope,
		"input_limit_tokens", compaction.InputLimitTokens,
		"trigger_tokens", compaction.TriggerTokens,
		"target_tokens", compaction.TargetTokens,
		"estimated_request_tokens", compaction.EstimatedRequestTokens,
	)
}

func (r *TurnRunner) logContextCompactionFailed(sessionID, turnID string, compaction events.ContextCompactionFailedPayload) {
	if r == nil {
		return
	}
	r.logger.Debug("session history compaction failed appended",
		"session_id", sessionID,
		"turn_id", turnID,
		"scope", compaction.Scope,
		"reason", compaction.Reason,
		"detail", compaction.Detail,
		"input_limit_tokens", compaction.InputLimitTokens,
		"trigger_tokens", compaction.TriggerTokens,
		"target_tokens", compaction.TargetTokens,
		"estimated_request_tokens", compaction.EstimatedRequestTokens,
	)
}

func (r *TurnRunner) logSessionHistoryContinuationUpdated(sessionID, turnID string, continuation events.SessionHistoryContinuationUpdatedPayload) {
	if r == nil {
		return
	}
	args := []any{
		"session_id", sessionID,
		"turn_id", turnID,
	}
	args = append(args, sessionHistoryContinuationPayloadLogArgs(&continuation)...)
	r.logger.Debug("session history continuation updated appended", args...)
}

func (r *TurnRunner) logSessionHistoryCheckpoint(sessionID string, payload events.SessionHistoryCheckpointPayload, rawTurnCount int) {
	if r == nil {
		return
	}
	args := []any{
		"session_id", sessionID,
		"through_sequence", payload.ThroughSequence,
		"completed_turns", len(payload.CompletedTurnIDs),
		"raw_turns", rawTurnCount,
	}
	args = append(args, sessionHistoryContinuationPayloadLogArgs(payload.Continuation)...)
	r.logger.Debug("session history checkpoint appended", args...)
}

func sessionHistoryPruningLogArgs(pruning events.ContextPrunedPayload) []any {
	return []any{
		"prior_turns", pruning.PriorTurns,
		"raw_prior_turns", pruning.RawPriorTurns,
		"compacted_prior_turns", pruning.CompactedPriorTurns,
		"omitted_prior_turns", pruning.OmittedPriorTurns,
		"prior_input_bytes", pruning.PriorInputBytes,
		"raw_input_bytes", pruning.RawInputBytes,
		"compacted_input_bytes", pruning.CompactedInputBytes,
		"omitted_input_bytes", pruning.OmittedInputBytes,
	}
}

func sessionHistoryContinuationPayloadLogArgs(payload *events.SessionHistoryContinuationUpdatedPayload) []any {
	if payload == nil {
		return []any{"continuation_present", false}
	}
	summary := payload.RenderedSummary
	return []any{
		"continuation_present", true,
		"update_reason", payload.UpdateReason,
		"model", payload.Attribution.Model,
		"input_limit_source", payload.Attribution.InputLimitSource,
		"measurement_source", payload.Attribution.MeasurementSource,
		"summary_source", payload.Attribution.SummarySource,
		"input_limit_tokens", continuationInputLimitTokens(payload),
		"trigger_tokens", continuationTriggerTokens(payload),
		"target_tokens", continuationTargetTokens(payload),
		"estimated_request_tokens", continuationEstimatedRequestTokens(payload),
		"consolidated_request_tokens", continuationCompactedRequestTokens(payload),
		"frontier_turn_id", payload.FrontierTurnID,
		"consolidated_turn_count", payload.ConsolidatedTurnCount,
		"newly_consolidated_turn_count", payload.NewlyConsolidatedTurnCount,
		"workspace_paths", historyContinuationArtifactPaths(payload.Artifact),
		"summary_len", len(summary),
		"summary_preview", compactionLogPreview(summary),
	}
}

func compactionLogPreview(text string) string {
	text = singleLineCompact(text)
	if text == "" {
		return ""
	}
	return truncateUTF8Bytes(text, 240)
}

func (r *TurnRunner) logToolDeclared(sessionID, turnID, callID, toolName string, arguments string) {
	r.logger.Debug("tool declared",
		"session_id", sessionID,
		"turn_id", turnID,
		"tool_call_id", callID,
		"tool_name", toolName,
		"arguments", arguments,
	)
}

func (r *TurnRunner) logAssistantCommit(sessionID, turnID, kind, content string) {
	if r == nil || r.logger == nil {
		return
	}
	r.logger.Debug("assistant text committed",
		"session_id", sessionID,
		"turn_id", turnID,
		"kind", kind,
		"content_len", len(content),
		"content_preview", compactionLogPreview(content),
	)
}
