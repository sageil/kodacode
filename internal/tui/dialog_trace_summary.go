package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func traceDialogSummarySection(th *theme.Theme, state events.SessionState, turnID string, turn *events.TurnState) string {
	lines := []string{
		dialogSectionStyle(th).Render("Turn Summary"),
		fmt.Sprintf("Turn: %d of %d", sessionToolTurnOrdinal(state, turnID), len(orderedSessionTurnIDs(state))),
		fmt.Sprintf("Status: %s", costDialogTurnStatus(turn)),
		fmt.Sprintf("Model: %s", costDialogTurnModel(turn)),
		traceDialogUsageLine(turn),
	}
	if turn.Config != nil && strings.TrimSpace(turn.Config.AgentID) != "" {
		lines = append(lines, "Agent: "+strings.TrimSpace(turn.Config.AgentID))
	}
	activity := []string{}
	if tools := len(orderedToolCallIDs(turn)); tools > 0 {
		activity = append(activity, pluralize(tools, "tool call"))
	}
	if handoffs := len(orderedHandoffIDs(turn)); handoffs > 0 {
		activity = append(activity, pluralize(handoffs, "handoff"))
	}
	if len(activity) > 0 {
		lines = append(lines, "Activity: "+strings.Join(activity, " | "))
	}
	if prompt := costDialogPromptPreview(turn); prompt != "" {
		lines = append(lines, "Prompt: "+prompt)
	}
	if mix, ok := traceDialogTurnRequestMix(turn); ok {
		lines = append(lines, "Estimated request mix: "+traceDialogRequestMixLabel(mix))
		if dominant := traceDialogDominantRequestDriver(mix); dominant != "" {
			lines = append(lines, "Dominant request driver: "+dominant)
		}
	}
	if savings := costDialogTurnSavings(turn); savings.HasInputSavings() {
		lines = append(lines, costDialogInputSavingsLine(savings, " for this turn"))
		if scope := costDialogSavingsScopeLine(max(turn.ProviderUsage.Attempts, 0), "turn"); scope != "" {
			lines = append(lines, scope)
		}
		if mix := costDialogSavingsMixLine(savings); mix != "" {
			lines = append(lines, mix)
		}
	}
	if savings := costDialogTurnSavings(turn); savings.HasCacheSavings() {
		lines = append(lines, fmt.Sprintf("Estimated cache discounts: %s where cache pricing was known", formatEstimatedCost(savings.EstimatedCacheSavingsCost)))
	}
	if drivers := costDialogTurnDrivers(turn, costDialogPricingUnavailable(turn)); len(drivers) > 0 {
		lines = append(lines, "Likely spend drivers: "+strings.Join(drivers, " • "))
	}
	lines = append(lines, lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(th, "subtext", "#9da8ca"))).
		Render("Trace source: durable runtime provider call records. Failed and successful provider calls are counted. Provider-reported usage is shown when available; cache pricing is applied when known and cost otherwise remains estimated. Request-mix attribution uses the same runtime token estimator used for budgeting when providers do not report per-component splits."))
	return strings.Join(lines, "\n")
}

func traceDialogProviderAttemptSection(th *theme.Theme, turn *events.TurnState) string {
	if turn == nil || len(turn.ProviderAttempts) == 0 {
		return ""
	}
	lines := []string{dialogSectionStyle(th).Render("Provider Calls")}
	for _, attempt := range turn.ProviderAttempts {
		tokenLabel := traceDialogProviderAttemptTokenLabel(attempt)
		parts := []string{
			fmt.Sprintf("%d.%d", max(attempt.Step, 0), max(attempt.Attempt, 0)),
			traceDialogProviderAttemptModelLabel(attempt),
			traceDialogDurationLabel(attempt.DurationMillis),
			tokenLabel,
		}
		if kind := traceDialogProviderAttemptKindLabel(attempt.Kind); kind != "" {
			parts = append(parts, kind)
		}
		switch {
		case attempt.EstimatedInputCost+attempt.EstimatedOutputCost > 0:
			parts = append(parts, formatEstimatedCost(attempt.EstimatedInputCost+attempt.EstimatedOutputCost))
		case costDialogPricingUnavailable(turn):
			parts = append(parts, "pricing unavailable")
		}
		lines = append(lines, strings.Join(parts, " | "))
		if route := traceDialogProviderAttemptRoute(attempt); route != "" {
			lines = append(lines, "   route tries: "+route)
		}
		if result := traceDialogProviderAttemptResult(attempt); result != "" {
			lines = append(lines, "   result: "+result)
		}
		if mix := traceDialogProviderAttemptRequestMix(attempt); mix != "" {
			lines = append(lines, "   request mix: "+mix)
		}
		for _, line := range traceDialogProviderAttemptSavingsLines(attempt) {
			lines = append(lines, "   "+line)
		}
	}
	return strings.Join(lines, "\n")
}

func traceDialogUsageLine(turn *events.TurnState) string {
	if turn == nil || turn.ProviderUsage == nil {
		return "Provider usage: not recorded yet."
	}
	usage := turn.ProviderUsage
	tokens := costDialogTokenUsageForTurn(turn)
	parts := []string{
		fmt.Sprintf("%d input", tokens.RequestTokens),
		fmt.Sprintf("%d output", tokens.CompletionTokens),
		assistantRoundtripLabel(max(usage.Steps, 0)),
		providerCallLabel(max(usage.Attempts, 0)),
	}
	parts = append(parts, traceDialogCacheTokenParts(tokens.CacheReadInputTokens, tokens.CacheWriteInputTokens)...)
	if tokens.ReasoningTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking", tokens.ReasoningTokens))
	}
	if tokens.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d total", tokens.TotalTokens))
	}
	switch tokens.Coverage {
	case costDialogUsageReported:
		parts = append(parts, "reported usage")
	case costDialogUsageMixed:
		if tokens.TotalAttempts > 0 {
			parts = append(parts, fmt.Sprintf("mixed usage (%d/%d provider calls reported)", tokens.ReportedAttempts, tokens.TotalAttempts))
		} else {
			parts = append(parts, "mixed usage")
		}
	}
	switch {
	case costDialogPricingUnavailable(turn):
		parts = append(parts, "pricing unavailable")
	case usage.EstimatedInputCost+usage.EstimatedOutputCost > 0:
		parts = append(parts, "estimated "+formatEstimatedCost(usage.EstimatedInputCost+usage.EstimatedOutputCost))
	}
	return "Provider usage: " + strings.Join(parts, " | ")
}

type traceDialogRequestMix struct {
	PromptTokens          int
	ConversationTokens    int
	ToolNameTokens        int
	ToolDescriptionTokens int
	ToolSchemaTokens      int
	ToolCount             int
}

func (m traceDialogRequestMix) ToolSurfaceTokens() int {
	return max(m.ToolNameTokens, 0) + max(m.ToolDescriptionTokens, 0) + max(m.ToolSchemaTokens, 0)
}

func (m traceDialogRequestMix) TotalTokens() int {
	return max(m.PromptTokens, 0) + max(m.ConversationTokens, 0) + m.ToolSurfaceTokens()
}

func traceDialogTurnRequestMix(turn *events.TurnState) (traceDialogRequestMix, bool) {
	if turn == nil || len(turn.ProviderAttempts) == 0 {
		return traceDialogRequestMix{}, false
	}
	mix := traceDialogRequestMix{}
	for _, attempt := range turn.ProviderAttempts {
		attemptMix, ok := traceDialogRequestMixFromAttempt(attempt)
		if !ok {
			continue
		}
		mix.PromptTokens += attemptMix.PromptTokens
		mix.ConversationTokens += attemptMix.ConversationTokens
		mix.ToolNameTokens += attemptMix.ToolNameTokens
		mix.ToolDescriptionTokens += attemptMix.ToolDescriptionTokens
		mix.ToolSchemaTokens += attemptMix.ToolSchemaTokens
		if attemptMix.ToolCount > mix.ToolCount {
			mix.ToolCount = attemptMix.ToolCount
		}
	}
	if mix.TotalTokens() == 0 {
		return traceDialogRequestMix{}, false
	}
	return mix, true
}

func traceDialogRequestMixFromAttempt(attempt events.TurnProviderAttemptState) (traceDialogRequestMix, bool) {
	mix := traceDialogRequestMix{
		PromptTokens:          max(attempt.PromptTokens, 0),
		ConversationTokens:    max(attempt.ConversationTokens, 0),
		ToolNameTokens:        max(attempt.ToolNameTokens, 0),
		ToolDescriptionTokens: max(attempt.ToolDescriptionTokens, 0),
		ToolSchemaTokens:      max(attempt.ToolSchemaTokens, 0),
		ToolCount:             max(attempt.ToolCount, 0),
	}
	if mix.TotalTokens() == 0 && mix.ToolCount == 0 {
		return traceDialogRequestMix{}, false
	}
	return mix, true
}

func traceDialogProviderAttemptRequestMix(attempt events.TurnProviderAttemptState) string {
	mix, ok := traceDialogRequestMixFromAttempt(attempt)
	if !ok {
		return ""
	}
	return traceDialogRequestMixLabel(mix)
}

func traceDialogRequestMixLabel(mix traceDialogRequestMix) string {
	parts := make([]string, 0, 4)
	if mix.PromptTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d prompt", mix.PromptTokens))
	}
	if mix.ConversationTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d conversation", mix.ConversationTokens))
	}
	if toolSurface := mix.ToolSurfaceTokens(); toolSurface > 0 {
		label := fmt.Sprintf("%d tool surface", toolSurface)
		details := make([]string, 0, 4)
		if mix.ToolSchemaTokens > 0 {
			details = append(details, fmt.Sprintf("%d schema", mix.ToolSchemaTokens))
		}
		if mix.ToolDescriptionTokens > 0 {
			details = append(details, fmt.Sprintf("%d descriptions", mix.ToolDescriptionTokens))
		}
		if mix.ToolNameTokens > 0 {
			details = append(details, fmt.Sprintf("%d names", mix.ToolNameTokens))
		}
		if mix.ToolCount > 0 {
			details = append(details, pluralize(mix.ToolCount, "tool"))
		}
		if len(details) > 0 {
			label += " (" + strings.Join(details, " | ") + ")"
		}
		parts = append(parts, label)
	}
	if len(parts) == 0 {
		return "0 tokens"
	}
	return strings.Join(parts, " | ")
}

func traceDialogProviderAttemptSavingsLines(attempt events.TurnProviderAttemptState) []string {
	savings := costDialogSavings{
		PromptCompactionTokensSaved:      attempt.PromptCompactionTokensSaved,
		HistoryCompactionTokensSaved:     attempt.HistoryCompactionTokensSaved,
		CurrentTurnProjectionTokensSaved: attempt.CurrentTurnProjectionTokensSaved,
		ToolDescriptionTokensSaved:       attempt.ToolDescriptionTokensSaved,
		ToolSchemaTokensSaved:            attempt.ToolSchemaTokensSaved,
		EstimatedInputSavingsCost:        attempt.EstimatedInputSavingsCost,
		EstimatedCacheSavingsCost:        attempt.EstimatedCacheSavingsCost,
	}
	lines := []string{}
	if savings.HasInputSavings() {
		line := fmt.Sprintf("input savings: %d avoided input tokens", savings.InputTokensSaved())
		if savings.EstimatedInputSavingsCost > 0 {
			line = fmt.Sprintf("input savings: %s from %d avoided input tokens", formatEstimatedCost(savings.EstimatedInputSavingsCost), savings.InputTokensSaved())
		}
		lines = append(lines, line)
		if mix := costDialogSavingsMixLabel(savings); mix != "" {
			lines = append(lines, "savings mix: "+mix)
		}
	}
	if savings.HasCacheSavings() {
		lines = append(lines, fmt.Sprintf("cache discount: %s", formatEstimatedCost(savings.EstimatedCacheSavingsCost)))
	}
	return lines
}

func traceDialogDominantRequestDriver(mix traceDialogRequestMix) string {
	prompt := max(mix.PromptTokens, 0)
	conversation := max(mix.ConversationTokens, 0)
	toolSurface := mix.ToolSurfaceTokens()
	switch {
	case toolSurface == 0 && conversation == 0 && prompt == 0:
		return ""
	case toolSurface >= prompt && toolSurface >= conversation && toolSurface > 0:
		return "tool surface"
	case conversation >= prompt && conversation > 0:
		return "conversation replay"
	case prompt > 0:
		return "instructions"
	default:
		return ""
	}
}

func traceDialogProviderAttemptModelLabel(attempt events.TurnProviderAttemptState) string {
	model := strings.TrimSpace(attempt.Model)
	requested := strings.TrimSpace(attempt.RequestedModel)
	switch {
	case model != "" && requested != "" && model != requested:
		return requested + " -> " + model
	case model != "":
		return model
	case requested != "":
		return requested
	default:
		return "provider call"
	}
}

func traceDialogProviderAttemptKindLabel(kind string) string {
	switch events.TurnProviderUsageKind(strings.TrimSpace(kind)) {
	case "", events.TurnProviderUsageKindAgent:
		return ""
	case events.TurnProviderUsageKindUtilityCompaction:
		return "utility compaction"
	default:
		return strings.TrimSpace(kind)
	}
}

func traceDialogProviderAttemptRoute(attempt events.TurnProviderAttemptState) string {
	if len(attempt.RouteAttempts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(attempt.RouteAttempts))
	for _, route := range attempt.RouteAttempts {
		label := strings.TrimSpace(route.Model)
		if label == "" {
			continue
		}
		switch {
		case route.Selected:
			label += " selected"
		case route.Error != "":
			label += " failed (" + traceDialogPreview(route.Error, 80) + ")"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " | ")
}

func traceDialogProviderAttemptResult(attempt events.TurnProviderAttemptState) string {
	parts := make([]string, 0, 4)
	if errorText := traceDialogPreview(attempt.Error, 180); errorText != "" {
		label := "error"
		if attempt.Retryable {
			label = "retryable error"
		}
		parts = append(parts, label+" "+errorText)
	}
	if attempt.DurableProgress {
		parts = append(parts, "durable progress")
	}
	if attempt.ExecutedTools > 0 {
		parts = append(parts, traceDialogToolActivityLabel(attempt.ExecutedTools, "executed"))
	}
	if attempt.ReusedTools > 0 {
		parts = append(parts, traceDialogToolActivityLabel(attempt.ReusedTools, "reused"))
	}
	if reason := strings.TrimSpace(attempt.FinishReason); reason != "" {
		parts = append(parts, "finish "+strings.ReplaceAll(reason, "_", " "))
	}
	if reason := strings.TrimSpace(attempt.RetrySkippedReason); reason != "" {
		parts = append(parts, "retry skipped: "+strings.ReplaceAll(reason, "_", " "))
	}
	if traceDialogHasCacheActivity(attempt.ReportedCacheReadInputTokens, attempt.ReportedCacheWriteInputTokens) {
		switch {
		case attempt.CachePricingApplied && attempt.CachePricingMissing:
			parts = append(parts, "cache pricing partial")
		case attempt.CachePricingApplied:
			parts = append(parts, "cache pricing applied")
		case attempt.CachePricingMissing:
			parts = append(parts, "cache pricing unavailable")
		}
	}
	return strings.Join(parts, " | ")
}

func traceDialogProviderAttemptTokenLabel(attempt events.TurnProviderAttemptState) string {
	input := max(attempt.RequestTokens, 0)
	output := max(attempt.CompletionTokens, 0)
	parts := []string{}
	if attempt.ReportedInputTokens > 0 || attempt.ReportedOutputTokens > 0 || attempt.ReportedTotalTokens > 0 || traceDialogHasCacheActivity(attempt.ReportedCacheReadInputTokens, attempt.ReportedCacheWriteInputTokens) {
		input = max(attempt.ReportedInputTokens, 0)
		output = max(attempt.ReportedOutputTokens, 0)
		parts = append(parts, fmt.Sprintf("%d input", input), fmt.Sprintf("%d output", output))
		parts = append(parts, traceDialogCacheTokenParts(attempt.ReportedCacheReadInputTokens, attempt.ReportedCacheWriteInputTokens)...)
		if attempt.ReportedReasoningTokens > 0 {
			parts = append(parts, fmt.Sprintf("%d thinking", attempt.ReportedReasoningTokens))
		}
		total := max(attempt.ReportedTotalTokens, 0)
		if total == 0 {
			total = input + output
		}
		if total > 0 {
			parts = append(parts, fmt.Sprintf("%d total", total))
		}
		parts = append(parts, "reported")
		return strings.Join(parts, " | ")
	}
	return strings.Join([]string{
		fmt.Sprintf("%d input", input),
		fmt.Sprintf("%d output", output),
	}, " | ")
}

func traceDialogCacheTokenParts(readTokens, writeTokens int) []string {
	parts := make([]string, 0, 2)
	if read := max(readTokens, 0); read > 0 {
		parts = append(parts, fmt.Sprintf("%d cache-read", read))
	}
	if write := max(writeTokens, 0); write > 0 {
		parts = append(parts, fmt.Sprintf("%d cache-write", write))
	}
	return parts
}

func traceDialogHasCacheActivity(readTokens, writeTokens int) bool {
	return max(readTokens, 0) > 0 || max(writeTokens, 0) > 0
}

func traceDialogDurationLabel(durationMillis int) string {
	switch {
	case durationMillis <= 0:
		return "0 ms"
	case durationMillis < 1000:
		return fmt.Sprintf("%d ms", durationMillis)
	default:
		return fmt.Sprintf("%.2fs", float64(durationMillis)/1000)
	}
}

func traceDialogToolActivityLabel(count int, suffix string) string {
	if count == 1 {
		return fmt.Sprintf("1 tool %s", suffix)
	}
	return fmt.Sprintf("%d tools %s", count, suffix)
}
