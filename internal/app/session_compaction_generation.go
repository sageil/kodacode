package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

const (
	sessionCompactionArtifactAgentID      = "session-history-compaction"
	sessionCompactionArtifactTurnMaxBytes = 4096
	sessionCompactionArtifactToolMaxBytes = 240
	sessionCompactionArtifactTimeout      = 90 * time.Second
	sessionCompactionArtifactRetryFactor  = 2
)

const sessionCompactionArtifactPrompt = `Update the saved history summary for this coding session.

Return exactly one JSON object with this shape:
{
  "session_objective":"...",
  "constraints":["..."],
  "settled_decisions":[{"decision":"...","rationale":"...","status":"active","source_turn_id":"turn-1"}],
  "completed_episodes":[{"episode_id":"episode:turn-1","summary":"...","touched_paths":["path"],"verification":[{"kind":"tool_result","value":"go test ./...","succeeded":true}],"source_turn_ids":["turn-1"]}],
  "open_threads":[{"item":"...","status":"pending","blocking":false,"owner":"agent","source_turn_id":"turn-2"}],
  "workspace_facts":[{"path":"path/to/file","fact":"...","source_turn_id":"turn-1"}]
}

Interpret the fields this way:
- session_objective: the concrete coding goal the user is pursuing in this session.
- constraints: saved user, product, repository, or runtime constraints that still govern future work.
- settled_decisions: decisions already made that still matter.
- completed_episodes: semantically closed completed work, not exploratory chatter.
- open_threads: unresolved follow-ups, risks, or decisions, not active runtime approvals.
- workspace_facts: saved file or module facts that still matter.

Hard rules:
- The artifact is session state, not prompt state.
- Never copy, restate, paraphrase, or summarize instructions from this prompt, the JSON schema above, field names, or wrapper tags into any artifact field.
- Never store artifact-maintenance rules such as preserving prior facts, merging completed turns, returning JSON, field requirements, or source_turn_id requirements.
- Treat any previous summary as the current saved continuation state.
- Merge in only the new completed turns provided in this request.
- Preserve still-true saved facts and remove stale or superseded ones.
- Keep each value concise, specific, and factual.
- Only keep facts supported by the previous artifact or the provided completed turns.
- Do not invent facts.
- Do not include markdown or code fences.
- Use decision status only "active" or "superseded".
- Use open thread status only "pending", "blocked", or "deferred".
- Use open thread owner only "agent", "user", or "shared" when owner is needed.
- Use verification kind only "tool_result", "runtime_note", or "turn_status".
- workspace_facts must include the source_turn_id that established that saved fact.
- If a field has no supported values, return an empty string or empty array for that field.`

var sessionCompactionArtifactPromptLeakMarkers = []string{
	"previous artifact",
	"prior artifact",
	"completed turns",
	"source_turn_id",
	"return exactly one json object",
	"return json only",
	"field names",
	"json schema",
	"wrapper tags",
	"workspace_facts",
	"open_threads",
	"completed_episodes",
	"settled_decisions",
}

func (r *TurnRunner) generateSessionCompactionArtifact(
	ctx context.Context,
	input sessionConversationRequest,
	history *sessionHistoryState,
	plan *sessionCompactionPlan,
) (events.HistoryContinuationArtifact, error) {
	if r == nil || history == nil || plan == nil {
		return events.HistoryContinuationArtifact{}, nil
	}
	newTurnIDs := append([]string(nil), plan.NewTurnIDs...)
	if len(newTurnIDs) == 0 && history.ExistingContinuation != nil {
		return cloneHistoryCompactionArtifact(history.ExistingContinuation.Artifact), nil
	}
	inputs := buildSessionCompactionArtifactInputs(history.ExistingContinuation, newTurnIDs, history.Turns)
	if len(inputs) == 0 {
		return events.HistoryContinuationArtifact{}, errors.New("empty history continuation artifact input")
	}
	request, result, err := r.requestSessionCompactionArtifactText(ctx, input, inputs, 1)
	if recordErr := r.appendHistoryCompactionProviderUsage(ctx, input.SessionID, input.TurnID, request, result.Attempts); recordErr != nil {
		return events.HistoryContinuationArtifact{}, recordErr
	}
	if err != nil {
		return events.HistoryContinuationArtifact{}, err
	}
	artifact, err := parseSessionCompactionArtifact(result.Text)
	if err != nil && shouldRetrySessionCompactionArtifactParse(request, err) {
		request, result, err = r.requestSessionCompactionArtifactText(ctx, input, inputs, sessionCompactionArtifactRetryFactor)
		if recordErr := r.appendHistoryCompactionProviderUsage(ctx, input.SessionID, input.TurnID, request, result.Attempts); recordErr != nil {
			return events.HistoryContinuationArtifact{}, recordErr
		}
		if err != nil {
			return events.HistoryContinuationArtifact{}, err
		}
		artifact, err = parseSessionCompactionArtifact(result.Text)
	}
	if err != nil {
		return events.HistoryContinuationArtifact{}, err
	}
	if err := validateGeneratedSessionCompactionArtifact(artifact, history.ExistingContinuation, newTurnIDs, history.CompletedOrder); err != nil {
		return events.HistoryContinuationArtifact{}, err
	}
	return artifact, nil
}

func (r *TurnRunner) requestSessionCompactionArtifactText(
	ctx context.Context,
	input sessionConversationRequest,
	inputs []provider.Input,
	outputBudgetFactor int,
) (provider.Request, utilityTextResult, error) {
	candidates := availableUtilityTextCandidates(r.utilityModel, input.ModelRoute, r.utilityProviderEnabled)
	if len(candidates) == 0 {
		return provider.Request{}, utilityTextResult{}, provider.ErrProviderNotConfigured
	}

	var (
		lastRequest        provider.Request
		lastResult         utilityTextResult
		lastErr            error
		skippedForContext  bool
		primaryProviderID  = strings.TrimSpace(input.ModelRoute.Primary.ProviderID)
		utilityTimeout     = sessionCompactionArtifactRequestTimeout(r.utilityModelTimeout)
		utilityRetryPolicy = r.utilityRetryPolicy
	)
	for _, candidate := range candidates {
		request := provider.Request{
			SessionID:       input.SessionID,
			TurnID:          input.TurnID,
			AgentID:         sessionCompactionArtifactAgentID,
			Model:           candidate.Ref,
			MaxOutputTokens: sessionCompactionArtifactOutputLimit(r.models, r.modelOverrides, r.outputBudgets, candidate.Ref, outputBudgetFactor),
			Instructions:    sessionCompactionArtifactPrompt,
			Inputs:          sessionCompactionArtifactRequestInputs(inputs, outputBudgetFactor),
		}
		lastRequest = request
		model := utilityCatalogModelForRef(r.models, candidate.Ref)
		requiredContext := provider.EstimateRequestTokens(request)
		if !utilityCandidateMeetsContext(model, requiredContext) {
			skippedForContext = true
			continue
		}
		client, err := r.sessionCompactionUtilityClient(candidate.Ref, primaryProviderID)
		if err != nil {
			lastErr = err
			continue
		}
		result, err := requestUtilityTextWithAttempts(ctx, client, request, utilityTimeout, utilityRetryPolicy)
		lastResult = result
		if err != nil {
			lastErr = err
			continue
		}
		return request, result, nil
	}
	if lastErr != nil {
		return lastRequest, lastResult, lastErr
	}
	if skippedForContext {
		return lastRequest, lastResult, errors.New("no utility model had enough context for session compaction")
	}
	return lastRequest, lastResult, provider.ErrProviderNotConfigured
}

func sessionCompactionArtifactRequestTimeout(configured time.Duration) time.Duration {
	timeout := effectiveUtilityTimeout(configured, sessionCompactionArtifactTimeout)
	if timeout > 0 && timeout < sessionCompactionArtifactTimeout {
		return sessionCompactionArtifactTimeout
	}
	return timeout
}

func (r *TurnRunner) sessionCompactionUtilityClient(ref provider.ModelRef, primaryProviderID string) (provider.Client, error) {
	if r != nil && r.utilityClientFactory != nil {
		return r.utilityClientFactory(ref.ProviderID)
	}
	if r != nil && strings.EqualFold(strings.TrimSpace(ref.ProviderID), primaryProviderID) && r.provider != nil {
		return r.provider, nil
	}
	return nil, provider.ErrProviderNotConfigured
}

func (r *TurnRunner) appendHistoryCompactionProviderUsage(
	ctx context.Context,
	sessionID string,
	turnID string,
	request provider.Request,
	attempts []utilityTextAttempt,
) error {
	if r == nil || len(attempts) == 0 {
		return nil
	}
	for _, attempt := range attempts {
		attemptNumber := max(attempt.Attempt, 1)
		attributedModel := providerUsageModelForTrace(request.Model, attempt.RouteTrace)
		attributedRequest := request
		attributedRequest.Model = attributedModel
		attributedRequest = provider.PreparePromptRequest(attributedRequest)
		requestBreakdown := provider.EstimateRequestTokenBreakdown(attributedRequest)
		completionTokens := provider.EstimateTextTokens(attempt.Text)
		usage := estimateTurnProviderUsage(r.models, attributedModel, requestBreakdown.TotalTokens, completionTokens)
		model := utilityCatalogModelForRef(r.models, attributedModel)
		if err := r.appendTurnProviderUsageRecorded(ctx, sessionID, turnID, events.TurnProviderUsageRecordedPayload{
			Model:                          attributedModel.String(),
			Kind:                           string(events.TurnProviderUsageKindUtilityCompaction),
			RequestedModel:                 request.Model.String(),
			Step:                           1,
			Attempt:                        attemptNumber,
			DurationMillis:                 int(attempt.Duration.Milliseconds()),
			RequestStarted:                 attempt.RequestStarted,
			RouteAttempts:                  providerRouteAttemptPayloads(attempt.RouteTrace),
			EstimatedRequestTokens:         usage.RequestTokens,
			EstimatedPromptTokens:          requestBreakdown.PromptTokens,
			EstimatedConversationTokens:    requestBreakdown.ConversationTokens,
			EstimatedToolNameTokens:        requestBreakdown.ToolNameTokens,
			EstimatedToolDescriptionTokens: requestBreakdown.ToolDescriptionTokens,
			EstimatedToolSchemaTokens:      requestBreakdown.ToolSchemaTokens,
			ToolCount:                      requestBreakdown.ToolCount,
			RequestTokenSource:             string(provider.TokenCountSourceEstimated),
			InputLimitTokens:               max(effectiveUtilityContextSize(model), 0),
			EstimatedCompletionTokens:      usage.CompletionTokens,
			EstimatedInputCost:             usage.InputCost,
			EstimatedOutputCost:            usage.OutputCost,
			Error:                          errorString(attempt.Error),
			Retryable:                      provider.RetryHintForError(attempt.Error).Retryable,
		}); err != nil {
			return err
		}
		if report := attempt.UsageReport; report != nil {
			reportedModel := providerReportedModelRef(attributedModel, report.Model)
			reportedUsage := estimateReportedTurnProviderUsage(r.models, reportedModel, *report, usage)
			cacheSavingsCost := estimateReportedCacheSavingsCost(r.models, reportedModel, *report)
			if err := r.appendTurnProviderUsageReported(ctx, sessionID, turnID, events.TurnProviderUsageReportedPayload{
				Model:                     providerReportedModelLabel(attributedModel, report.Model),
				Kind:                      string(events.TurnProviderUsageKindUtilityCompaction),
				RequestID:                 strings.TrimSpace(report.RequestID),
				Step:                      1,
				Attempt:                   attemptNumber,
				InputTokens:               max(report.InputTokens, 0),
				CacheReadInputTokens:      max(report.CacheReadInputTokens, 0),
				CacheWriteInputTokens:     max(report.CacheWriteInputTokens, 0),
				OutputTokens:              max(report.OutputTokens, 0),
				ReasoningTokens:           max(report.ReasoningTokens, 0),
				TotalTokens:               max(report.TotalTokens, 0),
				EstimatedInputCost:        reportedUsage.InputCost,
				EstimatedOutputCost:       reportedUsage.OutputCost,
				EstimatedCacheSavingsCost: cacheSavingsCost,
				CachePricingApplied:       reportedUsage.CachePricingApplied,
				CachePricingMissing:       reportedUsage.CachePricingMissing,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func sessionCompactionArtifactOutputLimit(models modelCatalog, overrides []ModelOverrideConfig, budgets OutputBudgetsConfig, ref provider.ModelRef, factor int) int {
	if factor <= 1 {
		return requestMaxOutputTokensForModel(models, overrides, budgets, ref, outputBudgetSessionCompaction, false)
	}
	ceiling := modelMaxOutputTokenCeilingForModel(models, ref)
	budget := outputBudgetForRequest(overrides, budgets, ref, outputBudgetSessionCompaction, false)
	return clampOutputTokenBudget(budget*factor, ceiling)
}

func sessionCompactionArtifactRequestInputs(inputs []provider.Input, outputBudgetFactor int) []provider.Input {
	requestInputs := append([]provider.Input(nil), inputs...)
	if outputBudgetFactor <= 1 {
		return requestInputs
	}
	return append(requestInputs, provider.Input{
		Kind: provider.InputKindUserMessage,
		Content: "The previous history artifact response was invalid or truncated. " +
			"Replace it with exactly one complete JSON object now. " +
			"Do not explain, apologize, use markdown, or include text outside the JSON object.",
	})
}

func buildSessionCompactionArtifactInputs(
	existing *events.SessionHistoryContinuationUpdatedPayload,
	newTurnIDs []string,
	turns map[string]*replayedSessionTurn,
) []provider.Input {
	inputs := make([]provider.Input, 0, len(newTurnIDs)+2)
	if previous := renderSessionCompactionPreviousArtifact(existing); previous != "" {
		inputs = append(inputs, provider.Input{
			Kind:    provider.InputKindAssistantMessage,
			Content: previous,
		})
	}
	for _, turnID := range newTurnIDs {
		if transcript := renderSessionCompactionTurnTranscript(turnID, turns[turnID]); transcript != "" {
			inputs = append(inputs, provider.Input{
				Kind:    provider.InputKindAssistantMessage,
				Content: transcript,
			})
		}
	}
	if len(inputs) == 0 {
		return nil
	}
	inputs = append(inputs, provider.Input{
		Kind: provider.InputKindUserMessage,
		Content: "Update the saved history summary using only the prior summary above, if present, " +
			"and the new completed turns above.",
	})
	return inputs
}

func renderSessionCompactionPreviousArtifact(existing *events.SessionHistoryContinuationUpdatedPayload) string {
	if existing == nil {
		return ""
	}
	artifact := normalizeHistoryCompactionArtifact(existing.Artifact)
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return ""
	}
	return "<previous-history-artifact>\n" + string(data) + "\n</previous-history-artifact>"
}

func renderSessionCompactionTurnTranscript(turnID string, turn *replayedSessionTurn) string {
	if turn == nil {
		return ""
	}
	lines := []string{fmt.Sprintf("<completed-turn id=%q>", turnID)}
	for _, input := range turn.replayInputs() {
		switch input.Kind {
		case provider.InputKindUserMessage:
			if text := compactOutcomeSingleLine(input.Content, sessionCompactionArtifactToolMaxBytes); text != "" {
				lines = append(lines, "User: "+text)
			}
		case provider.InputKindAssistantMessage:
			if text := compactOutcomeSingleLine(input.Content, sessionCompactionArtifactToolMaxBytes); text != "" {
				lines = append(lines, "Assistant: "+text)
			}
		case provider.InputKindAnthropicThinking:
			if input.AnthropicThinking != nil {
				if text := compactOutcomeSingleLine(input.AnthropicThinking.Thinking, sessionCompactionArtifactToolMaxBytes); text != "" {
					lines = append(lines, "Thinking: "+text)
				}
			}
		case provider.InputKindToolCall:
			text := compactOutcomeSingleLine(input.Arguments, sessionCompactionArtifactToolMaxBytes)
			lines = append(lines, fmt.Sprintf("Tool call %s: %s", input.ToolName, text))
		case provider.InputKindToolResult:
			text := compactOutcomeSingleLine(strings.TrimSpace(input.Output), sessionCompactionArtifactToolMaxBytes)
			if text == "" {
				text = compactOutcomeSingleLine(strings.TrimSpace(input.Error), sessionCompactionArtifactToolMaxBytes)
			}
			lines = append(lines, fmt.Sprintf("Tool result %s: %s", input.ToolName, text))
		}
	}
	for _, note := range turn.postTerminalRuntimeNotes() {
		if text := compactOutcomeSingleLine(note.Content, sessionCompactionArtifactToolMaxBytes); text != "" {
			lines = append(lines, "Runtime note: "+text)
		}
	}
	if status := strings.TrimSpace(renderCompactionTurnStatus(turn)); status != "" && status != "completed" {
		lines = append(lines, "Turn status: "+status)
	}
	for _, path := range appendBoundedOutcomeValues(nil, turn.WorkspacePaths, compactionSummaryFileLimit, compactionSummaryWorkspacePathBytes) {
		lines = append(lines, "Workspace path: "+path)
	}
	lines = append(lines, "</completed-turn>")
	return truncateUTF8Bytes(strings.Join(lines, "\n"), sessionCompactionArtifactTurnMaxBytes)
}

func parseSessionCompactionArtifact(raw string) (events.HistoryContinuationArtifact, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return events.HistoryContinuationArtifact{}, errors.New("empty history continuation artifact response")
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return events.HistoryContinuationArtifact{}, errors.New("history continuation artifact response missing json object")
	}
	trimmed = trimmed[start : end+1]
	var artifact events.HistoryContinuationArtifact
	if err := json.Unmarshal([]byte(trimmed), &artifact); err != nil {
		return events.HistoryContinuationArtifact{}, err
	}
	artifact = normalizeHistoryCompactionArtifact(artifact)
	if err := validateSessionCompactionArtifactOutput(artifact); err != nil {
		return events.HistoryContinuationArtifact{}, err
	}
	if historyContinuationArtifactEmpty(artifact) {
		return events.HistoryContinuationArtifact{}, errors.New("history continuation artifact is empty")
	}
	return artifact, nil
}

func shouldRetrySessionCompactionArtifactParse(request provider.Request, err error) bool {
	if err == nil || provider.EffectiveMaxOutputTokens(request) <= 0 {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "unexpected end of json input")
}

func validateSessionCompactionArtifactOutput(artifact events.HistoryContinuationArtifact) error {
	if err := validateSessionCompactionArtifactPromptLeak("session_objective", artifact.SessionObjective); err != nil {
		return err
	}
	for index, value := range artifact.Constraints {
		if err := validateSessionCompactionArtifactPromptLeak(fmt.Sprintf("constraints[%d]", index), value); err != nil {
			return err
		}
	}
	for index, decision := range artifact.SettledDecisions {
		if err := validateSessionCompactionArtifactPromptLeak(fmt.Sprintf("settled_decisions[%d].decision", index), decision.Decision); err != nil {
			return err
		}
		if err := validateSessionCompactionArtifactPromptLeak(fmt.Sprintf("settled_decisions[%d].rationale", index), decision.Rationale); err != nil {
			return err
		}
	}
	for index, episode := range artifact.CompletedEpisodes {
		if err := validateSessionCompactionArtifactPromptLeak(fmt.Sprintf("completed_episodes[%d].summary", index), episode.Summary); err != nil {
			return err
		}
		for verificationIndex, verification := range episode.Verification {
			if err := validateSessionCompactionArtifactPromptLeak(fmt.Sprintf("completed_episodes[%d].verification[%d].value", index, verificationIndex), verification.Value); err != nil {
				return err
			}
		}
	}
	for index, thread := range artifact.OpenThreads {
		if err := validateSessionCompactionArtifactPromptLeak(fmt.Sprintf("open_threads[%d].item", index), thread.Item); err != nil {
			return err
		}
	}
	for index, fact := range artifact.WorkspaceFacts {
		if err := validateSessionCompactionArtifactPromptLeak(fmt.Sprintf("workspace_facts[%d].fact", index), fact.Fact); err != nil {
			return err
		}
	}
	for index, hint := range artifact.PageInHints {
		if err := validateSessionCompactionArtifactPromptLeak(fmt.Sprintf("page_in_hints[%d].when", index), hint.When); err != nil {
			return err
		}
	}
	return nil
}

func validateSessionCompactionArtifactPromptLeak(field, value string) error {
	if promptLeak := sessionCompactionArtifactPromptLeak(value); promptLeak != "" {
		return fmt.Errorf("history continuation artifact %s contains prompt instruction text: %s", field, promptLeak)
	}
	return nil
}

func sessionCompactionArtifactPromptLeak(value string) string {
	lower := strings.ToLower(compactOutcomeSingleLine(value, sessionCompactionArtifactTurnMaxBytes))
	for _, marker := range sessionCompactionArtifactPromptLeakMarkers {
		if strings.Contains(lower, marker) {
			return marker
		}
	}
	return ""
}

func validateGeneratedSessionCompactionArtifact(
	artifact events.HistoryContinuationArtifact,
	existing *events.SessionHistoryContinuationUpdatedPayload,
	newTurnIDs []string,
	completedOrder []string,
) error {
	allowed := allowedSessionCompactionArtifactSourceTurnIDs(existing, newTurnIDs, completedOrder)
	if len(allowed) == 0 {
		return nil
	}
	if err := validateSessionCompactionArtifactSourceTurnID(allowed, func(yield func(string, string)) {
		for index, decision := range artifact.SettledDecisions {
			yield(fmt.Sprintf("settled_decisions[%d].source_turn_id", index), decision.SourceTurnID)
		}
	}); err != nil {
		return err
	}
	if err := validateSessionCompactionArtifactSourceTurnID(allowed, func(yield func(string, string)) {
		for index, episode := range artifact.CompletedEpisodes {
			for sourceIndex, sourceTurnID := range episode.SourceTurnIDs {
				yield(fmt.Sprintf("completed_episodes[%d].source_turn_ids[%d]", index, sourceIndex), sourceTurnID)
			}
		}
	}); err != nil {
		return err
	}
	if err := validateSessionCompactionArtifactSourceTurnID(allowed, func(yield func(string, string)) {
		for index, thread := range artifact.OpenThreads {
			yield(fmt.Sprintf("open_threads[%d].source_turn_id", index), thread.SourceTurnID)
		}
	}); err != nil {
		return err
	}
	if err := validateSessionCompactionArtifactSourceTurnID(allowed, func(yield func(string, string)) {
		for index, fact := range artifact.WorkspaceFacts {
			yield(fmt.Sprintf("workspace_facts[%d].source_turn_id", index), fact.SourceTurnID)
		}
	}); err != nil {
		return err
	}
	return validateSessionCompactionArtifactSourceTurnID(allowed, func(yield func(string, string)) {
		for index, hint := range artifact.PageInHints {
			for sourceIndex, sourceTurnID := range hint.SourceTurnIDs {
				yield(fmt.Sprintf("page_in_hints[%d].source_turn_ids[%d]", index, sourceIndex), sourceTurnID)
			}
		}
	})
}

func validateSessionCompactionArtifactSourceTurnID(
	allowed map[string]struct{},
	visit func(func(string, string)),
) error {
	var invalidField, invalidTurnID string
	visit(func(field, turnID string) {
		if invalidField != "" {
			return
		}
		turnID = strings.TrimSpace(turnID)
		if turnID == "" {
			return
		}
		if _, ok := allowed[turnID]; ok {
			return
		}
		invalidField = field
		invalidTurnID = turnID
	})
	if invalidField != "" {
		return fmt.Errorf("history continuation artifact %s cites unsupported source_turn_id %q", invalidField, invalidTurnID)
	}
	return nil
}

func allowedSessionCompactionArtifactSourceTurnIDs(existing *events.SessionHistoryContinuationUpdatedPayload, newTurnIDs, completedOrder []string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(newTurnIDs)+8)
	for _, turnID := range sanitizeCompactionTurnOrder(newTurnIDs) {
		allowed[turnID] = struct{}{}
	}
	if existing == nil {
		return allowed
	}
	if prefix := compactedPrefixTurnOrder(completedOrder, existing); len(prefix) > 0 {
		for _, turnID := range prefix {
			allowed[turnID] = struct{}{}
		}
	} else {
		addHistoryCompactionArtifactSourceTurnIDs(allowed, existing.Artifact)
	}
	return allowed
}

func addHistoryCompactionArtifactSourceTurnIDs(allowed map[string]struct{}, artifact events.HistoryContinuationArtifact) {
	add := func(turnID string) {
		turnID = strings.TrimSpace(turnID)
		if turnID != "" {
			allowed[turnID] = struct{}{}
		}
	}
	for _, decision := range artifact.SettledDecisions {
		add(decision.SourceTurnID)
	}
	for _, episode := range artifact.CompletedEpisodes {
		for _, turnID := range episode.SourceTurnIDs {
			add(turnID)
		}
	}
	for _, thread := range artifact.OpenThreads {
		add(thread.SourceTurnID)
	}
	for _, fact := range artifact.WorkspaceFacts {
		add(fact.SourceTurnID)
	}
	for _, hint := range artifact.PageInHints {
		for _, turnID := range hint.SourceTurnIDs {
			add(turnID)
		}
	}
}
