package events

import (
	"errors"
	"strings"
)

type TurnProviderUsageKind string

const (
	TurnProviderUsageKindAgent             TurnProviderUsageKind = "agent"
	TurnProviderUsageKindUtilityCompaction TurnProviderUsageKind = "utility_compaction"
)

type TurnProviderUsageRecordedPayload struct {
	Model                                      string
	Kind                                       string
	RequestedModel                             string
	Step                                       int
	Attempt                                    int
	DurationMillis                             int
	RequestStarted                             bool
	RequestAPIMode                             string
	RequestParallelToolCalls                   bool
	RouteAttempts                              []TurnProviderRouteAttemptPayload
	EstimatedRequestTokens                     int
	EstimatedPromptTokens                      int
	EstimatedConversationTokens                int
	EstimatedToolNameTokens                    int
	EstimatedToolDescriptionTokens             int
	EstimatedToolSchemaTokens                  int
	EstimatedPromptCompactionTokensSaved       int
	EstimatedHistoryCompactionTokensSaved      int
	EstimatedCurrentTurnProjectionTokensSaved  int
	EstimatedToolDescriptionTokensSaved        int
	EstimatedToolSchemaTokensSaved             int
	EstimatedDeterministicContextTokens        int
	EstimatedDeterministicContextOmittedTokens int
	EstimatedInputSavingsCost                  float64
	ToolCount                                  int
	RequestTokenSource                         string
	InputLimitTokens                           int
	EstimatedCompletionTokens                  int
	EstimatedInputCost                         float64
	EstimatedOutputCost                        float64
	Error                                      string
	Retryable                                  bool
	RetrySkippedReason                         string
	FinishReason                               string
	DurableProgress                            bool
	ExecutedTools                              int
	ReusedTools                                int
}

func (TurnProviderUsageRecordedPayload) eventType() Type { return TypeTurnProviderUsageRecorded }

type TurnProviderUsageReportedPayload struct {
	Model                     string
	Kind                      string
	RequestID                 string
	Step                      int
	Attempt                   int
	InputTokens               int
	CacheReadInputTokens      int
	CacheWriteInputTokens     int
	OutputTokens              int
	ReasoningTokens           int
	TotalTokens               int
	EstimatedInputCost        float64
	EstimatedOutputCost       float64
	EstimatedCacheSavingsCost float64
	CachePricingApplied       bool
	CachePricingMissing       bool
}

func (TurnProviderUsageReportedPayload) eventType() Type { return TypeTurnProviderUsageReported }

func (p TurnProviderUsageRecordedPayload) validate() error {
	if err := validateTurnProviderUsageKind(p.Kind); err != nil {
		return err
	}
	if p.Step <= 0 {
		return errors.New("step must be > 0")
	}
	if p.Attempt <= 0 {
		return errors.New("attempt must be > 0")
	}
	if p.DurationMillis < 0 {
		return errors.New("duration_millis must be >= 0")
	}
	if p.EstimatedRequestTokens < 0 {
		return errors.New("estimated_request_tokens must be >= 0")
	}
	if p.EstimatedPromptTokens < 0 {
		return errors.New("estimated_prompt_tokens must be >= 0")
	}
	if p.EstimatedConversationTokens < 0 {
		return errors.New("estimated_conversation_tokens must be >= 0")
	}
	if p.EstimatedToolNameTokens < 0 {
		return errors.New("estimated_tool_name_tokens must be >= 0")
	}
	if p.EstimatedToolDescriptionTokens < 0 {
		return errors.New("estimated_tool_description_tokens must be >= 0")
	}
	if p.EstimatedToolSchemaTokens < 0 {
		return errors.New("estimated_tool_schema_tokens must be >= 0")
	}
	if p.EstimatedPromptCompactionTokensSaved < 0 {
		return errors.New("estimated_prompt_compaction_tokens_saved must be >= 0")
	}
	if p.EstimatedHistoryCompactionTokensSaved < 0 {
		return errors.New("estimated_history_compaction_tokens_saved must be >= 0")
	}
	if p.EstimatedCurrentTurnProjectionTokensSaved < 0 {
		return errors.New("estimated_current_turn_projection_tokens_saved must be >= 0")
	}
	if p.EstimatedToolDescriptionTokensSaved < 0 {
		return errors.New("estimated_tool_description_tokens_saved must be >= 0")
	}
	if p.EstimatedToolSchemaTokensSaved < 0 {
		return errors.New("estimated_tool_schema_tokens_saved must be >= 0")
	}
	if p.EstimatedDeterministicContextTokens < 0 {
		return errors.New("estimated_deterministic_context_tokens must be >= 0")
	}
	if p.EstimatedDeterministicContextOmittedTokens < 0 {
		return errors.New("estimated_deterministic_context_omitted_tokens must be >= 0")
	}
	if p.EstimatedInputSavingsCost < 0 {
		return errors.New("estimated_input_savings_cost must be >= 0")
	}
	if p.ToolCount < 0 {
		return errors.New("tool_count must be >= 0")
	}
	if p.InputLimitTokens < 0 {
		return errors.New("input_limit_tokens must be >= 0")
	}
	if p.EstimatedCompletionTokens < 0 {
		return errors.New("estimated_completion_tokens must be >= 0")
	}
	if p.EstimatedInputCost < 0 {
		return errors.New("estimated_input_cost must be >= 0")
	}
	if p.EstimatedOutputCost < 0 {
		return errors.New("estimated_output_cost must be >= 0")
	}
	if p.ExecutedTools < 0 {
		return errors.New("executed_tools must be >= 0")
	}
	if p.ReusedTools < 0 {
		return errors.New("reused_tools must be >= 0")
	}
	for _, attempt := range p.RouteAttempts {
		if err := attempt.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (p TurnProviderUsageReportedPayload) validate() error {
	if err := validateTurnProviderUsageKind(p.Kind); err != nil {
		return err
	}
	if p.Step <= 0 {
		return errors.New("step must be > 0")
	}
	if p.Attempt <= 0 {
		return errors.New("attempt must be > 0")
	}
	if p.InputTokens < 0 {
		return errors.New("input_tokens must be >= 0")
	}
	if p.CacheReadInputTokens < 0 {
		return errors.New("cache_read_input_tokens must be >= 0")
	}
	if p.CacheWriteInputTokens < 0 {
		return errors.New("cache_write_input_tokens must be >= 0")
	}
	if p.OutputTokens < 0 {
		return errors.New("output_tokens must be >= 0")
	}
	if p.ReasoningTokens < 0 {
		return errors.New("reasoning_tokens must be >= 0")
	}
	if p.TotalTokens < 0 {
		return errors.New("total_tokens must be >= 0")
	}
	if p.EstimatedInputCost < 0 {
		return errors.New("estimated_input_cost must be >= 0")
	}
	if p.EstimatedOutputCost < 0 {
		return errors.New("estimated_output_cost must be >= 0")
	}
	if p.EstimatedCacheSavingsCost < 0 {
		return errors.New("estimated_cache_savings_cost must be >= 0")
	}
	return nil
}

type TurnProviderRouteAttemptPayload struct {
	Model    string
	Selected bool
	Error    string
}

func (p TurnProviderRouteAttemptPayload) validate() error {
	if p.Model == "" {
		return errors.New("route_attempts.model is required")
	}
	return nil
}

type TurnProviderUsageState struct {
	Model               string
	Steps               int
	Attempts            int
	RequestTokens       int
	CompletionTokens    int
	EstimatedInputCost  float64
	EstimatedOutputCost float64
}

type TurnProviderReportedUsageState struct {
	Model                     string
	RequestID                 string
	Steps                     int
	Attempts                  int
	InputTokens               int
	CacheReadInputTokens      int
	CacheWriteInputTokens     int
	OutputTokens              int
	ReasoningTokens           int
	TotalTokens               int
	EstimatedCacheSavingsCost float64
	CachePricingApplied       bool
	CachePricingMissing       bool
}

type TurnContextUsageState struct {
	Tokens int
	Limit  int
	Source string
}

type TurnProviderAttemptState struct {
	Model                             string
	Kind                              string
	RequestedModel                    string
	Step                              int
	Attempt                           int
	DurationMillis                    int
	RequestStarted                    bool
	RequestAPIMode                    string
	RequestParallelToolCalls          bool
	RouteAttempts                     []TurnProviderRouteAttemptState
	RequestTokens                     int
	PromptTokens                      int
	ConversationTokens                int
	ToolNameTokens                    int
	ToolDescriptionTokens             int
	ToolSchemaTokens                  int
	PromptCompactionTokensSaved       int
	HistoryCompactionTokensSaved      int
	CurrentTurnProjectionTokensSaved  int
	ToolDescriptionTokensSaved        int
	ToolSchemaTokensSaved             int
	DeterministicContextTokens        int
	DeterministicContextOmittedTokens int
	EstimatedInputSavingsCost         float64
	ToolCount                         int
	RequestTokenSource                string
	InputLimitTokens                  int
	CompletionTokens                  int
	EstimatedInputCost                float64
	EstimatedOutputCost               float64
	Error                             string
	Retryable                         bool
	RetrySkippedReason                string
	FinishReason                      string
	DurableProgress                   bool
	ExecutedTools                     int
	ReusedTools                       int
	ReportedModel                     string
	ReportedRequestID                 string
	ReportedInputTokens               int
	ReportedCacheReadInputTokens      int
	ReportedCacheWriteInputTokens     int
	ReportedOutputTokens              int
	ReportedReasoningTokens           int
	ReportedTotalTokens               int
	EstimatedCacheSavingsCost         float64
	CachePricingApplied               bool
	CachePricingMissing               bool
}

type TurnProviderRouteAttemptState struct {
	Model    string
	Selected bool
	Error    string
}

func turnProviderAttemptHasReportedUsage(state TurnProviderAttemptState) bool {
	return state.ReportedRequestID != "" ||
		state.ReportedModel != "" ||
		state.ReportedInputTokens > 0 ||
		state.ReportedCacheReadInputTokens > 0 ||
		state.ReportedCacheWriteInputTokens > 0 ||
		state.ReportedOutputTokens > 0 ||
		state.ReportedReasoningTokens > 0 ||
		state.ReportedTotalTokens > 0 ||
		state.CachePricingApplied ||
		state.CachePricingMissing
}

func turnProviderAttemptCountsFullUsage(state TurnProviderAttemptState) bool {
	if turnProviderAttemptHasReportedUsage(state) {
		return true
	}
	if strings.TrimSpace(state.Error) == "" {
		return true
	}
	if state.DurableProgress {
		return true
	}
	if max(state.CompletionTokens, 0) > 0 {
		return true
	}
	if max(state.ExecutedTools, 0) > 0 || max(state.ReusedTools, 0) > 0 {
		return true
	}
	return false
}

func normalizeTurnProviderUsageKind(kind string) string {
	switch TurnProviderUsageKind(strings.TrimSpace(kind)) {
	case "", TurnProviderUsageKindAgent:
		return string(TurnProviderUsageKindAgent)
	case TurnProviderUsageKindUtilityCompaction:
		return string(TurnProviderUsageKindUtilityCompaction)
	default:
		return strings.TrimSpace(kind)
	}
}

func validateTurnProviderUsageKind(kind string) error {
	switch TurnProviderUsageKind(normalizeTurnProviderUsageKind(kind)) {
	case TurnProviderUsageKindAgent, TurnProviderUsageKindUtilityCompaction:
		return nil
	default:
		return errors.New("kind must be agent or utility_compaction")
	}
}

func turnProviderUsageKindMatches(a, b string) bool {
	return normalizeTurnProviderUsageKind(a) == normalizeTurnProviderUsageKind(b)
}

func normalizeTurnProviderAttemptState(state *TurnProviderAttemptState) {
	if state == nil {
		return
	}
	state.Kind = normalizeTurnProviderUsageKind(state.Kind)
	if !state.RequestStarted && strings.TrimSpace(state.Error) != "" {
		clearUnstartedTurnProviderAttemptEstimate(state)
		return
	}
	if turnProviderAttemptCountsFullUsage(*state) {
		return
	}
	if state.RequestStarted {
		state.CompletionTokens = 0
		state.EstimatedOutputCost = 0
		return
	}
	clearUnstartedTurnProviderAttemptEstimate(state)
}

func clearUnstartedTurnProviderAttemptEstimate(state *TurnProviderAttemptState) {
	if state == nil {
		return
	}
	state.RequestTokens = 0
	state.PromptTokens = 0
	state.ConversationTokens = 0
	state.ToolNameTokens = 0
	state.ToolDescriptionTokens = 0
	state.ToolSchemaTokens = 0
	state.PromptCompactionTokensSaved = 0
	state.HistoryCompactionTokensSaved = 0
	state.CurrentTurnProjectionTokensSaved = 0
	state.ToolDescriptionTokensSaved = 0
	state.ToolSchemaTokensSaved = 0
	state.DeterministicContextTokens = 0
	state.DeterministicContextOmittedTokens = 0
	state.ToolCount = 0
	state.CompletionTokens = 0
	state.EstimatedInputCost = 0
	state.EstimatedOutputCost = 0
	state.EstimatedInputSavingsCost = 0
}

func recomputeTurnProviderUsageState(turn *TurnState) {
	if turn == nil || len(turn.ProviderAttempts) == 0 {
		return
	}
	if turn.ProviderUsage == nil {
		turn.ProviderUsage = &TurnProviderUsageState{}
	}
	usage := turn.ProviderUsage
	model := strings.TrimSpace(usage.Model)
	steps := 0
	for index := range turn.ProviderAttempts {
		turn.ProviderAttempts[index].Kind = normalizeTurnProviderUsageKind(turn.ProviderAttempts[index].Kind)
		normalizeTurnProviderAttemptState(&turn.ProviderAttempts[index])
		attempt := turn.ProviderAttempts[index]
		if attempt.Step > steps {
			steps = attempt.Step
		}
		switch {
		case strings.TrimSpace(attempt.ReportedModel) != "":
			model = strings.TrimSpace(attempt.ReportedModel)
		case strings.TrimSpace(attempt.Model) != "":
			model = strings.TrimSpace(attempt.Model)
		case model == "" && strings.TrimSpace(attempt.RequestedModel) != "":
			model = strings.TrimSpace(attempt.RequestedModel)
		}
	}
	usage.Model = model
	usage.Steps = steps
	usage.Attempts = len(turn.ProviderAttempts)
	usage.RequestTokens,
		usage.CompletionTokens,
		_,
		_,
		_,
		_,
		usage.EstimatedInputCost,
		usage.EstimatedOutputCost = turnProviderAttemptUsageTotals(turn.ProviderAttempts)
}

func turnProviderAttemptUsageTotals(states []TurnProviderAttemptState) (requestTokens, completionTokens, cacheReadInputTokens, cacheWriteInputTokens, reasoningTokens, totalTokens int, estimatedInputCost, estimatedOutputCost float64) {
	for _, state := range states {
		estimatedInputCost += state.EstimatedInputCost
		estimatedOutputCost += state.EstimatedOutputCost
		if turnProviderAttemptHasReportedUsage(state) {
			requestTokens += max(state.ReportedInputTokens, 0)
			completionTokens += max(state.ReportedOutputTokens, 0)
			cacheReadInputTokens += max(state.ReportedCacheReadInputTokens, 0)
			cacheWriteInputTokens += max(state.ReportedCacheWriteInputTokens, 0)
			reasoningTokens += max(state.ReportedReasoningTokens, 0)
			total := max(state.ReportedTotalTokens, 0)
			if total == 0 {
				total = max(state.ReportedInputTokens, 0) + max(state.ReportedOutputTokens, 0)
			}
			totalTokens += total
			continue
		}
		requestTokens += max(state.RequestTokens, 0)
		completionTokens += max(state.CompletionTokens, 0)
		totalTokens += max(state.RequestTokens, 0) + max(state.CompletionTokens, 0)
	}
	return requestTokens, completionTokens, cacheReadInputTokens, cacheWriteInputTokens, reasoningTokens, totalTokens, estimatedInputCost, estimatedOutputCost
}

func cloneTurnProviderUsageState(state *TurnProviderUsageState) *TurnProviderUsageState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}

func cloneTurnProviderReportedUsageState(state *TurnProviderReportedUsageState) *TurnProviderReportedUsageState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}

func cloneTurnContextUsageState(state *TurnContextUsageState) *TurnContextUsageState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}

func DisplayTurnContextUsage(turn *TurnState) (TurnContextUsageState, bool) {
	if turn == nil {
		return TurnContextUsageState{}, false
	}
	if usage, ok := turnContextUsageStateValue(turn.ContextUsage); ok {
		return usage, true
	}
	if attempt := LatestAgentProviderAttempt(turn); attempt != nil {
		if usage, ok := turnContextUsageStateFromProviderAttempt(*attempt); ok {
			return usage, true
		}
	}
	if usage, ok := turnContextUsageStateFromContinuation(turn.Continuation); ok {
		return usage, true
	}
	return TurnContextUsageState{}, false
}

func LatestAgentProviderAttempt(turn *TurnState) *TurnProviderAttemptState {
	if turn == nil {
		return nil
	}
	for index := len(turn.ProviderAttempts) - 1; index >= 0; index-- {
		attempt := &turn.ProviderAttempts[index]
		if normalizeTurnProviderUsageKind(attempt.Kind) == string(TurnProviderUsageKindUtilityCompaction) {
			continue
		}
		return attempt
	}
	return nil
}

func normalizeTurnContextUsageState(turn *TurnState) {
	if turn == nil {
		return
	}
	if usage, ok := DisplayTurnContextUsage(turn); ok {
		turn.ContextUsage = &usage
		return
	}
	turn.ContextUsage = nil
}

func turnContextUsageStateValue(state *TurnContextUsageState) (TurnContextUsageState, bool) {
	if state == nil {
		return TurnContextUsageState{}, false
	}
	tokens := max(state.Tokens, 0)
	limit := max(state.Limit, 0)
	if tokens <= 0 || limit <= 0 {
		return TurnContextUsageState{}, false
	}
	source := strings.TrimSpace(state.Source)
	if source == "" {
		source = "estimated"
	}
	return TurnContextUsageState{
		Tokens: tokens,
		Limit:  limit,
		Source: source,
	}, true
}

func turnContextUsageStateFromProviderAttempt(attempt TurnProviderAttemptState) (TurnContextUsageState, bool) {
	if normalizeTurnProviderUsageKind(attempt.Kind) == string(TurnProviderUsageKindUtilityCompaction) {
		return TurnContextUsageState{}, false
	}
	limit := max(attempt.InputLimitTokens, 0)
	if limit <= 0 {
		return TurnContextUsageState{}, false
	}
	if turnProviderAttemptHasReportedUsage(attempt) {
		tokens := max(attempt.ReportedInputTokens, 0)
		if tokens > 0 {
			return TurnContextUsageState{
				Tokens: tokens,
				Limit:  limit,
				Source: "exact",
			}, true
		}
	}
	tokens := max(attempt.RequestTokens, 0)
	if tokens <= 0 {
		return TurnContextUsageState{}, false
	}
	return TurnContextUsageState{
		Tokens: tokens,
		Limit:  limit,
		Source: normalizedTurnContextUsageSource(attempt.RequestTokenSource),
	}, true
}

func turnContextUsageStateFromContinuation(continuation *HistoryContinuationState) (TurnContextUsageState, bool) {
	if continuation == nil {
		return TurnContextUsageState{}, false
	}
	if continuation.InputBudget == nil {
		return TurnContextUsageState{}, false
	}
	tokens := max(continuation.InputBudget.ConsolidatedRequestTokens, 0)
	limit := max(continuation.InputBudget.InputLimitTokens, 0)
	if tokens <= 0 || limit <= 0 {
		return TurnContextUsageState{}, false
	}
	source := strings.TrimSpace(continuation.Attribution.MeasurementSource)
	if source == "" {
		source = "estimated"
	}
	return TurnContextUsageState{
		Tokens: tokens,
		Limit:  limit,
		Source: normalizedTurnContextUsageSource(source),
	}, true
}

func normalizedTurnContextUsageSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "estimated"
	}
	return source
}

func providerAttemptStateFromPayload(payload TurnProviderUsageRecordedPayload) TurnProviderAttemptState {
	attempt := TurnProviderAttemptState{
		Model:                             payload.Model,
		Kind:                              normalizeTurnProviderUsageKind(payload.Kind),
		RequestedModel:                    payload.RequestedModel,
		Step:                              payload.Step,
		Attempt:                           payload.Attempt,
		DurationMillis:                    payload.DurationMillis,
		RequestStarted:                    payload.RequestStarted,
		RequestAPIMode:                    strings.TrimSpace(payload.RequestAPIMode),
		RequestParallelToolCalls:          payload.RequestParallelToolCalls,
		RequestTokens:                     payload.EstimatedRequestTokens,
		PromptTokens:                      payload.EstimatedPromptTokens,
		ConversationTokens:                payload.EstimatedConversationTokens,
		ToolNameTokens:                    payload.EstimatedToolNameTokens,
		ToolDescriptionTokens:             payload.EstimatedToolDescriptionTokens,
		ToolSchemaTokens:                  payload.EstimatedToolSchemaTokens,
		PromptCompactionTokensSaved:       payload.EstimatedPromptCompactionTokensSaved,
		HistoryCompactionTokensSaved:      payload.EstimatedHistoryCompactionTokensSaved,
		CurrentTurnProjectionTokensSaved:  payload.EstimatedCurrentTurnProjectionTokensSaved,
		ToolDescriptionTokensSaved:        payload.EstimatedToolDescriptionTokensSaved,
		ToolSchemaTokensSaved:             payload.EstimatedToolSchemaTokensSaved,
		DeterministicContextTokens:        payload.EstimatedDeterministicContextTokens,
		DeterministicContextOmittedTokens: payload.EstimatedDeterministicContextOmittedTokens,
		EstimatedInputSavingsCost:         payload.EstimatedInputSavingsCost,
		ToolCount:                         payload.ToolCount,
		RequestTokenSource:                payload.RequestTokenSource,
		InputLimitTokens:                  payload.InputLimitTokens,
		CompletionTokens:                  payload.EstimatedCompletionTokens,
		EstimatedInputCost:                payload.EstimatedInputCost,
		EstimatedOutputCost:               payload.EstimatedOutputCost,
		Error:                             payload.Error,
		Retryable:                         payload.Retryable,
		RetrySkippedReason:                strings.TrimSpace(payload.RetrySkippedReason),
		FinishReason:                      strings.TrimSpace(payload.FinishReason),
		DurableProgress:                   payload.DurableProgress,
		ExecutedTools:                     payload.ExecutedTools,
		ReusedTools:                       payload.ReusedTools,
	}
	if len(payload.RouteAttempts) > 0 {
		attempt.RouteAttempts = make([]TurnProviderRouteAttemptState, 0, len(payload.RouteAttempts))
		for _, route := range payload.RouteAttempts {
			attempt.RouteAttempts = append(attempt.RouteAttempts, TurnProviderRouteAttemptState(route))
		}
	}
	normalizeTurnProviderAttemptState(&attempt)
	return attempt
}

func cloneTurnProviderAttemptStates(states []TurnProviderAttemptState) []TurnProviderAttemptState {
	if len(states) == 0 {
		return nil
	}
	cloned := make([]TurnProviderAttemptState, 0, len(states))
	for _, state := range states {
		copyState := state
		copyState.RouteAttempts = append([]TurnProviderRouteAttemptState(nil), state.RouteAttempts...)
		cloned = append(cloned, copyState)
	}
	return cloned
}

func mergeTurnProviderAttemptReportedUsage(states []TurnProviderAttemptState, payload TurnProviderUsageReportedPayload) []TurnProviderAttemptState {
	if len(states) == 0 {
		return states
	}
	for index := range states {
		if states[index].Step != payload.Step ||
			states[index].Attempt != payload.Attempt ||
			!turnProviderUsageKindMatches(states[index].Kind, payload.Kind) {
			continue
		}
		states[index].ReportedModel = payload.Model
		states[index].ReportedRequestID = payload.RequestID
		states[index].ReportedInputTokens = payload.InputTokens
		states[index].ReportedCacheReadInputTokens = payload.CacheReadInputTokens
		states[index].ReportedCacheWriteInputTokens = payload.CacheWriteInputTokens
		states[index].ReportedOutputTokens = payload.OutputTokens
		states[index].ReportedReasoningTokens = payload.ReasoningTokens
		states[index].ReportedTotalTokens = payload.TotalTokens
		states[index].EstimatedInputCost = payload.EstimatedInputCost
		states[index].EstimatedOutputCost = payload.EstimatedOutputCost
		states[index].EstimatedCacheSavingsCost = payload.EstimatedCacheSavingsCost
		states[index].CachePricingApplied = payload.CachePricingApplied
		states[index].CachePricingMissing = payload.CachePricingMissing
		break
	}
	return states
}
