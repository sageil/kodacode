package app

import "github.com/sageil/kodacode/internal/provider"

type stepToolAdmission struct {
	Accepted         bool
	CompletionTokens int
}

func admitStepToolCall(batch *stepToolBatch, call stepToolCall) stepToolAdmission {
	return admitStepToolCallWithResolver(batch, call, nil)
}

func admitStepToolCallWithResolver(batch *stepToolBatch, call stepToolCall, resolver stepToolCapabilityResolver) stepToolAdmission {
	if batch == nil {
		return stepToolAdmission{}
	}
	if batch.Len() > 0 && !providerStepToolCallCanJoinPrefixWithResolver(resolver, batch.Calls, call) {
		return stepToolAdmission{}
	}
	completionTokens := provider.EstimateTextTokens(call.ToolName) + provider.EstimateTextTokens(call.Arguments)
	if !batch.AppendCall(call) {
		return stepToolAdmission{CompletionTokens: completionTokens}
	}
	return stepToolAdmission{
		Accepted:         true,
		CompletionTokens: completionTokens,
	}
}
