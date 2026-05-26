package app

import (
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

type stepAttemptCompleter struct {
	stream           provider.Stream
	streamClosed     *bool
	requestStarted   bool
	durableProgress  *bool
	committedState   *turnLoopState
	completionTokens *int
	toolCallStarted  *bool
	routeTrace       provider.RouteTrace
	startedAt        time.Time
	finishReason     *provider.FinishReason
	requestTrace     provider.RequestTrace
}

type stepAttemptCompleterInput struct {
	Stream           provider.Stream
	StreamClosed     *bool
	RequestStarted   bool
	DurableProgress  *bool
	CommittedState   *turnLoopState
	CompletionTokens *int
	ToolCallStarted  *bool
	RouteTrace       provider.RouteTrace
	StartedAt        time.Time
	FinishReason     *provider.FinishReason
	RequestTrace     provider.RequestTrace
}

func newStepAttemptCompleter(input stepAttemptCompleterInput) stepAttemptCompleter {
	return stepAttemptCompleter{
		stream:           input.Stream,
		streamClosed:     input.StreamClosed,
		requestStarted:   input.RequestStarted,
		durableProgress:  input.DurableProgress,
		committedState:   input.CommittedState,
		completionTokens: input.CompletionTokens,
		toolCallStarted:  input.ToolCallStarted,
		routeTrace:       input.RouteTrace,
		startedAt:        input.StartedAt,
		finishReason:     input.FinishReason,
		requestTrace:     input.RequestTrace,
	}
}

func (f stepAttemptCompleter) Complete(result assistantRoundtripResult, err error) providerRequestAttemptResult {
	f.closeStream()
	var usageReport *provider.UsageReport
	if f.stream != nil {
		if report, ok := provider.StreamUsageReport(f.stream); ok {
			copyReport := report
			usageReport = &copyReport
		}
		if f.finishReason != nil {
			*f.finishReason = provider.StreamFinishReason(f.stream)
		}
	}
	finishReason := f.finishReasonValue()
	if err != nil && finishReason == provider.FinishReasonUnknown {
		finishReason = provider.FinishReasonError
	}
	return providerRequestAttemptResult{
		Result:           result,
		Error:            err,
		RequestStarted:   f.requestStarted,
		DurableProgress:  f.durableProgressValue(),
		CommittedState:   cloneTurnLoopState(f.committedStateValue()),
		CompletionTokens: f.completionTokenValue(),
		ToolCallStarted:  f.toolCallStartedValue(),
		RouteTrace:       f.routeTrace,
		Duration:         time.Since(f.startedAt),
		UsageReport:      usageReport,
		FinishReason:     finishReason,
		RequestTrace:     f.requestTrace,
	}
}

func (f stepAttemptCompleter) closeStream() {
	if f.stream == nil {
		return
	}
	if f.streamClosed != nil {
		if *f.streamClosed {
			return
		}
		*f.streamClosed = true
	}
	_ = f.stream.Close()
}

func (f stepAttemptCompleter) durableProgressValue() bool {
	if f.durableProgress == nil {
		return false
	}
	return *f.durableProgress
}

func (f stepAttemptCompleter) committedStateValue() turnLoopState {
	if f.committedState == nil {
		return turnLoopState{}
	}
	return *f.committedState
}

func (f stepAttemptCompleter) completionTokenValue() int {
	if f.completionTokens == nil {
		return 0
	}
	return *f.completionTokens
}

func (f stepAttemptCompleter) toolCallStartedValue() bool {
	if f.toolCallStarted == nil {
		return false
	}
	return *f.toolCallStarted
}

func (f stepAttemptCompleter) finishReasonValue() provider.FinishReason {
	if f.finishReason == nil {
		return provider.FinishReasonUnknown
	}
	return provider.NormalizeFinishReason(*f.finishReason)
}
