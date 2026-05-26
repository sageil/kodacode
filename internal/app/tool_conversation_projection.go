package app

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

type turnProviderRequestProjection struct {
	Request                 provider.Request
	CurrentInputs           []provider.Input
	CurrentInputTokensSaved int
}

type turnRequestOutputBudgetConfig struct {
	Overrides []ModelOverrideConfig
	Budgets   OutputBudgetsConfig
}

func buildTurnProjectedCurrentConversation(state turnLoopState) ([]provider.Input, error) {
	if state.WorkState.NativeContinuation != nil {
		if !nativeToolContinuationContractSupported(state.WorkState.NativeContinuation.Contract) {
			return nil, fmt.Errorf("%w: %s", ErrNativeToolContinuationContractUnsupported, state.WorkState.NativeContinuation.Contract)
		}
		return cloneProviderInputs(state.WorkState.NativeContinuation.Inputs), nil
	}
	inputs := make([]provider.Input, 0, 2)
	if state.UserInput.Kind != "" {
		inputs = append(inputs, cloneProviderInputs([]provider.Input{state.UserInput})...)
	}
	if summaryInput := renderTurnWorkSummaryInput(state.WorkState.Summary); summaryInput != nil {
		inputs = append(inputs, *summaryInput)
	}
	return inputs, nil
}

func persistTurnProviderRequestProjection(state *turnLoopState, projection turnProviderRequestProjection) {
	if state == nil {
		return
	}
	state.Conversation = cloneProviderInputs(projection.CurrentInputs)
	if state.WorkState.NativeContinuation != nil {
		state.WorkState.NativeContinuation.Inputs = cloneProviderInputs(projection.CurrentInputs)
	}
}

func buildTurnBaseProviderRequest(input turnLoopInput, tools []provider.Tool, models modelCatalog, budgetConfig turnRequestOutputBudgetConfig) provider.Request {
	return provider.Request{
		SessionID:         input.SessionID,
		TurnID:            input.TurnID,
		AgentID:           input.AgentID,
		Model:             input.ModelRoute.Primary,
		MaxOutputTokens:   requestMaxOutputTokensForRoute(models, budgetConfig.Overrides, budgetConfig.Budgets, input.ModelRoute, outputBudgetKindForAgent(input.AgentID), input.ThinkingEnabled),
		ThinkingSupported: input.ThinkingSupported,
		ThinkingEnabled:   input.ThinkingEnabled,
		ThinkingMode:      input.ThinkingMode,
		Instructions:      input.Instructions,
		CacheablePrefix:   input.CacheablePrefix,
		DynamicSuffix:     input.DynamicSuffix,
		Tools:             append([]provider.Tool(nil), tools...),
	}
}

func buildTurnProviderRequestProjection(baseRequest provider.Request, history sessionConversation, state turnLoopState) (turnProviderRequestProjection, error) {
	currentInputs, err := buildTurnProjectedCurrentConversation(state)
	if err != nil {
		return turnProviderRequestProjection{}, err
	}
	requestCurrentInputs, currentInputTokensSaved := projectNativeCurrentTurnInputs(currentInputs, state.LatestToolStepStart)
	historyInputs := trimProjectedHistoryOverlap(history.Inputs, requestCurrentInputs)
	historyInputCount := len(historyInputs)
	request := baseRequest
	request.Inputs = make([]provider.Input, 0, historyInputCount+len(requestCurrentInputs))
	request.Inputs = append(request.Inputs, historyInputs...)
	request.Inputs = append(request.Inputs, requestCurrentInputs...)
	request.Instructions, request.DynamicSuffix = applyToolResultVisibilityPrompt(
		baseRequest.Instructions,
		baseRequest.CacheablePrefix,
		baseRequest.DynamicSuffix,
		request.Inputs,
	)
	return turnProviderRequestProjection{
		Request:                 request,
		CurrentInputs:           currentInputs,
		CurrentInputTokensSaved: currentInputTokensSaved,
	}, nil
}

func projectNativeCurrentTurnInputs(inputs []provider.Input, latestToolStepStart int) ([]provider.Input, int) {
	projected := cloneProviderInputs(inputs)
	if latestToolStepStart < 0 || latestToolStepStart >= len(projected) {
		return projected, 0
	}
	savedTokens := 0
	for index, input := range projected {
		if index >= latestToolStepStart || !activeTurnToolResultPrunable(input) {
			continue
		}
		replacement, ok := pruneRetainedRawToolResultInput("active-turn", input)
		if !ok {
			continue
		}
		before := provider.EstimateInputTokens(input)
		after := provider.EstimateInputTokens(replacement)
		if after >= before {
			continue
		}
		projected[index] = replacement
		savedTokens += before - after
	}
	return projected, savedTokens
}

func activeTurnToolResultPrunable(input provider.Input) bool {
	if input.Kind != provider.InputKindToolResult {
		return false
	}
	if strings.TrimSpace(input.Error) != "" {
		return false
	}
	if strings.TrimSpace(input.ToolName) == tool.ReadToolName {
		return false
	}
	return retainedRawToolResultPrunable(input)
}

func trimProjectedHistoryOverlap(historyInputs, currentInputs []provider.Input) []provider.Input {
	maxOverlap := min(len(historyInputs), len(currentInputs))
	for overlap := maxOverlap; overlap > 0; overlap-- {
		if providerInputSlicesEqual(historyInputs[len(historyInputs)-overlap:], currentInputs[:overlap]) {
			return append([]provider.Input(nil), historyInputs[:len(historyInputs)-overlap]...)
		}
	}
	return append([]provider.Input(nil), historyInputs...)
}

func providerInputSlicesEqual(left, right []provider.Input) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !turnInputsEqual([]provider.Input{left[index]}, []provider.Input{right[index]}) {
			return false
		}
	}
	return true
}
