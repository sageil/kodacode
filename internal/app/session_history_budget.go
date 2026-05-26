package app

import "github.com/sageil/kodacode/internal/provider"

const (
	sessionHistoryDefaultCompactionThreshold = 0.80
	sessionHistoryDefaultTargetThreshold     = 0.60
	sessionHistorySummaryBudgetMinTokens     = 256
	sessionHistorySummaryBudgetMaxTokens     = 1024
	sessionHistorySummaryBudgetDivisor       = 64
	sessionHistoryRawTailBudgetMinTokens     = 512
	sessionHistoryRawTailBudgetMaxTokens     = 2048
	sessionHistoryRawTailBudgetDivisor       = 8
)

type sessionHistoryBudget struct {
	InputLimitSource    string
	InputLimitTokens    int
	TriggerTokens       int
	TargetTokens        int
	SummaryBudgetTokens int
	SummaryBudgetBytes  int
	RawTailBudgetTokens int
	RawTailBudgetBytes  int
}

func replayedInputBytes(input provider.Input) int {
	return len(input.Content) +
		len(input.CallID) +
		len(input.ToolName) +
		len(input.Arguments) +
		len(input.Output) +
		len(input.Error) +
		len(input.OpenAIReasoningItem)
}

func replayedTurnInputBytes(turn *replayedSessionTurn) int {
	if turn == nil {
		return 0
	}
	total := 0
	for _, input := range turn.replayInputs() {
		total += replayedInputBytes(input)
	}
	for _, note := range turn.postTerminalRuntimeNotes() {
		total += replayedInputBytes(provider.Input{
			Kind:    provider.InputKindAssistantMessage,
			Content: note.Content,
		})
	}
	return total
}

func replayedTurnsInputBytes(order []string, turns map[string]*replayedSessionTurn) int {
	total := 0
	for _, turnID := range order {
		total += replayedTurnInputBytes(turns[turnID])
	}
	return total
}

func defaultSessionHistoryBudget() sessionHistoryBudget {
	return sessionHistoryBudget{
		SummaryBudgetTokens: byteBudgetToTokenBudget(compactionSummaryBudgetBytes),
		SummaryBudgetBytes:  compactionSummaryBudgetBytes,
	}
}

func resolveSessionHistoryBudget(route provider.ModelRoute, models modelCatalog, config SessionConfig) sessionHistoryBudget {
	budget, ok := resolveModelInputBudgetForRoute(route, models)
	if !ok || budget.InputLimitTokens <= 0 {
		return sessionHistoryBudget{}
	}
	return sessionHistoryBudgetFromInputLimit(budget.InputLimitTokens, budget.Source, config)
}

func sessionHistoryBudgetFromInputLimit(inputLimit int, source string, config SessionConfig) sessionHistoryBudget {
	if inputLimit <= 0 {
		return sessionHistoryBudget{}
	}
	triggerTokens, targetTokens := compactionThresholdTokens(inputLimit, config)

	summaryTokens := inputLimit / sessionHistorySummaryBudgetDivisor
	if summaryTokens < sessionHistorySummaryBudgetMinTokens {
		summaryTokens = sessionHistorySummaryBudgetMinTokens
	}
	if summaryTokens > sessionHistorySummaryBudgetMaxTokens {
		summaryTokens = sessionHistorySummaryBudgetMaxTokens
	}
	if maxSummary := max(targetTokens/4, 1); summaryTokens > maxSummary {
		summaryTokens = maxSummary
	}

	rawTailTokens := inputLimit / sessionHistoryRawTailBudgetDivisor
	if rawTailTokens < sessionHistoryRawTailBudgetMinTokens {
		rawTailTokens = sessionHistoryRawTailBudgetMinTokens
	}
	if rawTailTokens > sessionHistoryRawTailBudgetMaxTokens {
		rawTailTokens = sessionHistoryRawTailBudgetMaxTokens
	}
	if maxRawTail := max(targetTokens/2, 1); rawTailTokens > maxRawTail {
		rawTailTokens = maxRawTail
	}

	return sessionHistoryBudget{
		InputLimitSource:    source,
		InputLimitTokens:    inputLimit,
		TriggerTokens:       triggerTokens,
		TargetTokens:        targetTokens,
		SummaryBudgetTokens: summaryTokens,
		SummaryBudgetBytes:  tokenBudgetToByteBudget(summaryTokens),
		RawTailBudgetTokens: rawTailTokens,
		RawTailBudgetBytes:  tokenBudgetToByteBudget(rawTailTokens),
	}
}

func (b sessionHistoryBudget) resolved() bool {
	return b.InputLimitTokens > 0
}

func normalizedCompactionThresholds(config SessionConfig) (triggerFraction, targetFraction float64) {
	triggerFraction = config.CompactionThreshold
	if triggerFraction <= 0 {
		triggerFraction = sessionHistoryDefaultCompactionThreshold
	}
	targetFraction = config.CompactionTargetThreshold
	if targetFraction <= 0 {
		targetFraction = sessionHistoryDefaultTargetThreshold
	}
	if targetFraction >= triggerFraction {
		targetFraction = triggerFraction - 0.10
	}
	if targetFraction <= 0 {
		targetFraction = triggerFraction / 2
	}
	return triggerFraction, targetFraction
}

func compactionThresholdTokens(inputLimit int, config SessionConfig) (triggerTokens, targetTokens int) {
	if inputLimit <= 0 {
		return 0, 0
	}
	triggerFraction, targetFraction := normalizedCompactionThresholds(config)

	triggerTokens = int(float64(inputLimit) * triggerFraction)
	if triggerTokens <= 0 {
		triggerTokens = inputLimit
	}
	if triggerTokens > inputLimit {
		triggerTokens = inputLimit
	}

	targetTokens = int(float64(inputLimit) * targetFraction)
	if targetTokens <= 0 {
		targetTokens = max(triggerTokens-(inputLimit/10), 1)
	}
	if targetTokens >= triggerTokens {
		targetTokens = max(triggerTokens-(inputLimit/20), 1)
	}
	return triggerTokens, targetTokens
}

func byteBudgetToTokenBudget(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 3) / 4
}

func tokenBudgetToByteBudget(tokens int) int {
	if tokens <= 0 {
		return 0
	}
	return tokens * 4
}
