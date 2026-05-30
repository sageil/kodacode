package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

const (
	branchSummaryAgentID              = "_branch_summary"
	branchSummaryTimeout              = 45 * time.Second
	branchSummaryMaxInputBytes        = 12000
	branchSummaryMaxTurns             = 6
	branchSummaryMaxUserBytes         = 900
	branchSummaryMaxAssistantBytes    = 1400
	branchSummaryMaxContinuationBytes = 3500
	branchSummaryMaxSummaryBytes      = 1600
)

const branchSummaryPrompt = `Summarize this branch of a coding session for a developer choosing between Timeline branches.

Return 2-4 concise bullets. Focus on:
- what changed on this branch,
- current outcome or status,
- key files, decisions, or risks,
- what to do next if the user opens it.

Do not invent facts. Do not mention token counts, model names, or this prompt.`

var ErrBranchSummaryStoreRequired = errors.New("branch summary artifact store is required")

type GenerateBranchSummaryInput struct {
	SessionID string
}

type GenerateBranchSummaryResult struct {
	SessionID        string
	Summary          string
	Model            string
	SourceSequence   int64
	PromptTokens     int
	CompletionTokens int
	Cached           bool
}

func (r *Runtime) GenerateBranchSummary(ctx context.Context, input GenerateBranchSummaryInput) (GenerateBranchSummaryResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return GenerateBranchSummaryResult{}, ErrSessionIDRequired
	}
	if r == nil || r.Sessions == nil || r.Store == nil {
		return GenerateBranchSummaryResult{}, ErrEventStoreRequired
	}
	store, ok := r.Store.(branchSummaryStore)
	if !ok {
		return GenerateBranchSummaryResult{}, ErrBranchSummaryStoreRequired
	}
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return GenerateBranchSummaryResult{}, err
	}
	if state.Branch == nil {
		return GenerateBranchSummaryResult{}, fmt.Errorf("session %s is not a branch", sessionID)
	}
	if artifact, ok, err := store.LoadBranchSummary(ctx, sessionID); err != nil {
		return GenerateBranchSummaryResult{}, err
	} else if ok && strings.TrimSpace(artifact.Summary) != "" && artifact.SourceSequence == state.LastSequence {
		return branchSummaryResultFromArtifact(artifact, true), nil
	}

	turnID := branchSummaryAttributionTurnID(state)
	if turnID == "" {
		return GenerateBranchSummaryResult{}, fmt.Errorf("branch session %s has no completed turns to summarize", sessionID)
	}
	request, result, err := r.requestBranchSummaryText(ctx, state, turnID)
	if recordErr := r.appendBranchSummaryProviderUsage(ctx, sessionID, turnID, request, result.Attempts); recordErr != nil {
		return GenerateBranchSummaryResult{}, recordErr
	}
	if err != nil {
		return GenerateBranchSummaryResult{}, err
	}
	summary := sanitizeBranchSummary(result.Text)
	if summary == "" {
		return GenerateBranchSummaryResult{}, errEmptyUtilityTextResponse
	}
	if refreshed, err := r.Sessions.Snapshot(ctx, sessionID); err == nil {
		state.LastSequence = refreshed.LastSequence
	}
	promptTokens, completionTokens := branchSummaryTokenEstimates(request, result.Attempts)
	artifact := events.BranchSummaryArtifact{
		SessionID:        sessionID,
		SourceSequence:   state.LastSequence,
		Summary:          summary,
		Model:            request.Model.String(),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	if err := store.SaveBranchSummary(ctx, artifact); err != nil {
		return GenerateBranchSummaryResult{}, err
	}
	return branchSummaryResultFromArtifact(artifact, false), nil
}

func (r *Runtime) requestBranchSummaryText(ctx context.Context, state events.SessionState, turnID string) (provider.Request, utilityTextResult, error) {
	candidates := availableUtilityTextCandidates(r.Config.UtilityModel, provider.ModelRoute{}, r.utilityProviderAvailable())
	if len(candidates) == 0 {
		return provider.Request{}, utilityTextResult{}, provider.ErrProviderNotConfigured
	}

	input := renderBranchSummaryInput(state)
	var (
		lastRequest       provider.Request
		lastResult        utilityTextResult
		lastErr           error
		skippedForContext bool
	)
	for _, candidate := range candidates {
		request := provider.Request{
			SessionID:       state.SessionID,
			TurnID:          turnID,
			AgentID:         branchSummaryAgentID,
			Model:           candidate.Ref,
			MaxOutputTokens: requestMaxOutputTokensForModel(r.ModelCatalog, r.Config.ModelOverrides, r.Config.OutputBudgets, candidate.Ref, outputBudgetUtilityText, false),
			Instructions:    branchSummaryPrompt,
			Inputs: []provider.Input{{
				Kind:    provider.InputKindUserMessage,
				Content: input,
			}},
		}
		lastRequest = request
		model := utilityCatalogModelForRef(r.ModelCatalog, candidate.Ref)
		if !utilityCandidateMeetsContext(model, provider.EstimateRequestTokens(request)) {
			skippedForContext = true
			continue
		}
		client, err := r.rawProviderClient(candidate.Ref.ProviderID)
		if err != nil {
			lastErr = err
			continue
		}
		result, err := requestUtilityTextWithAttempts(ctx, client, request, effectiveUtilityTimeout(utilityTimeoutDuration(r.Config.UtilityModelTimeoutSeconds), branchSummaryTimeout), utilityRetryPolicyFromConfig(r.Config))
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
		return lastRequest, lastResult, errors.New("configured utility model does not have enough context for branch summary")
	}
	return lastRequest, lastResult, provider.ErrProviderNotConfigured
}

func (r *Runtime) appendBranchSummaryProviderUsage(
	ctx context.Context,
	sessionID string,
	turnID string,
	request provider.Request,
	attempts []utilityTextAttempt,
) error {
	if r == nil || r.Sessions == nil || len(attempts) == 0 {
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
		usage := estimateTurnProviderUsage(r.ModelCatalog, attributedModel, requestBreakdown.TotalTokens, completionTokens)
		model := utilityCatalogModelForRef(r.ModelCatalog, attributedModel)
		if _, err := r.Sessions.append(ctx, events.Draft{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeTurnProviderUsageRecorded,
			Payload: events.TurnProviderUsageRecordedPayload{
				Model:                          attributedModel.String(),
				Kind:                           string(events.TurnProviderUsageKindUtilityBranchSummary),
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
			},
		}); err != nil {
			return err
		}
		if report := attempt.UsageReport; report != nil {
			reportedModel := providerReportedModelRef(attributedModel, report.Model)
			reportedUsage := estimateReportedTurnProviderUsage(r.ModelCatalog, reportedModel, *report, usage)
			cacheSavingsCost := estimateReportedCacheSavingsCost(r.ModelCatalog, reportedModel, *report)
			if _, err := r.Sessions.append(ctx, events.Draft{
				SessionID: sessionID,
				TurnID:    turnID,
				Type:      events.TypeTurnProviderUsageReported,
				Payload: events.TurnProviderUsageReportedPayload{
					Model:                     providerReportedModelLabel(attributedModel, report.Model),
					Kind:                      string(events.TurnProviderUsageKindUtilityBranchSummary),
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
				},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderBranchSummaryInput(state events.SessionState) string {
	var builder strings.Builder
	builder.WriteString("Branch session: ")
	builder.WriteString(state.SessionID)
	builder.WriteString("\nTitle: ")
	builder.WriteString(strings.TrimSpace(state.Title))
	if state.Branch != nil {
		builder.WriteString("\nParent session: ")
		builder.WriteString(state.Branch.ParentSessionID)
		builder.WriteString("\nParent turn: ")
		builder.WriteString(state.Branch.ParentTurnID)
	}
	if continuation := latestBranchContinuationSummary(state); continuation != "" {
		builder.WriteString("\n\nExisting history summary:\n")
		builder.WriteString(truncateBranchSummaryBytes(continuation, branchSummaryMaxContinuationBytes))
	}
	builder.WriteString("\n\nRecent final turns:\n")
	for _, turnID := range recentFinalBranchTurnIDs(state, branchSummaryMaxTurns) {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		builder.WriteString("\nTurn ")
		builder.WriteString(turnID)
		builder.WriteString(" [")
		builder.WriteString(string(turn.Status))
		builder.WriteString("]")
		if len(turn.ToolCallOrder) > 0 {
			fmt.Fprintf(&builder, " %d tools", len(turn.ToolCallOrder))
		}
		if text := strings.TrimSpace(turn.UserText); text != "" {
			builder.WriteString("\nUser: ")
			builder.WriteString(truncateBranchSummaryBytes(flattenBranchSummaryText(text), branchSummaryMaxUserBytes))
		}
		if text := strings.TrimSpace(turn.AssistantText); text != "" {
			builder.WriteString("\nAssistant: ")
			builder.WriteString(truncateBranchSummaryBytes(flattenBranchSummaryText(text), branchSummaryMaxAssistantBytes))
		}
		if text := strings.TrimSpace(turn.Error); text != "" {
			builder.WriteString("\nError: ")
			builder.WriteString(truncateBranchSummaryBytes(flattenBranchSummaryText(text), 600))
		}
		builder.WriteString("\n")
		if builder.Len() >= branchSummaryMaxInputBytes {
			break
		}
	}
	return truncateBranchSummaryBytes(builder.String(), branchSummaryMaxInputBytes)
}

func latestBranchContinuationSummary(state events.SessionState) string {
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turn := state.Turns[state.TurnOrder[idx]]
		if turn != nil && turn.Continuation != nil && strings.TrimSpace(turn.Continuation.RenderedSummary) != "" {
			return strings.TrimSpace(turn.Continuation.RenderedSummary)
		}
	}
	return ""
}

func recentFinalBranchTurnIDs(state events.SessionState, limit int) []string {
	if limit <= 0 {
		return nil
	}
	reversed := make([]string, 0, limit)
	for idx := len(state.TurnOrder) - 1; idx >= 0 && len(reversed) < limit; idx-- {
		turnID := state.TurnOrder[idx]
		turn := state.Turns[turnID]
		if !branchSummaryTurnFinal(turn) {
			continue
		}
		reversed = append(reversed, turnID)
	}
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return reversed
}

func branchSummaryTurnFinal(turn *events.TurnState) bool {
	return turn != nil && turn.Status != events.TurnStatusRunning && turn.CompletedAtSeq > 0
}

func branchSummaryAttributionTurnID(state events.SessionState) string {
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turnID := state.TurnOrder[idx]
		if branchSummaryTurnFinal(state.Turns[turnID]) {
			return turnID
		}
	}
	return ""
}

func branchSummaryTokenEstimates(request provider.Request, attempts []utilityTextAttempt) (int, int) {
	attributedRequest := provider.PreparePromptRequest(request)
	promptTokens := provider.EstimateRequestTokenBreakdown(attributedRequest).TotalTokens
	completionTokens := 0
	for _, attempt := range attempts {
		if strings.TrimSpace(attempt.Text) != "" {
			completionTokens = provider.EstimateTextTokens(attempt.Text)
		}
	}
	return max(promptTokens, 0), max(completionTokens, 0)
}

func branchSummaryResultFromArtifact(artifact events.BranchSummaryArtifact, cached bool) GenerateBranchSummaryResult {
	return GenerateBranchSummaryResult{
		SessionID:        artifact.SessionID,
		Summary:          strings.TrimSpace(artifact.Summary),
		Model:            strings.TrimSpace(artifact.Model),
		SourceSequence:   artifact.SourceSequence,
		PromptTokens:     max(artifact.PromptTokens, 0),
		CompletionTokens: max(artifact.CompletionTokens, 0),
		Cached:           cached,
	}
}

func sanitizeBranchSummary(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return truncateBranchSummaryBytes(text, branchSummaryMaxSummaryBytes)
}

func flattenBranchSummaryText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func truncateBranchSummaryBytes(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	suffix := "..."
	if limit <= len(suffix) {
		return text[:limit]
	}
	budget := limit - len(suffix)
	var builder strings.Builder
	for _, r := range text {
		next := string(r)
		if builder.Len()+len(next) > budget {
			break
		}
		builder.WriteString(next)
	}
	return strings.TrimSpace(builder.String()) + suffix
}
