package provider

import "strings"

type RequestTrace struct {
	APIMode           string
	ParallelToolCalls bool
}

type requestTraceCarrier interface {
	RequestTrace() RequestTrace
}

type requestTracedStream struct {
	Stream
	trace RequestTrace
}

func (s requestTracedStream) RequestTrace() RequestTrace {
	return s.trace
}

func (s requestTracedStream) UsageReport() (UsageReport, bool) {
	return StreamUsageReport(s.Stream)
}

func (s requestTracedStream) FinishReason() FinishReason {
	return StreamFinishReason(s.Stream)
}

func withRequestTrace(stream Stream, trace RequestTrace) Stream {
	if stream == nil {
		return nil
	}
	return requestTracedStream{
		Stream: stream,
		trace:  trace,
	}
}

func StreamRequestTrace(stream Stream) (RequestTrace, bool) {
	if stream == nil {
		return RequestTrace{}, false
	}
	carrier, ok := stream.(requestTraceCarrier)
	if !ok {
		return RequestTrace{}, false
	}
	trace := carrier.RequestTrace()
	if strings.TrimSpace(trace.APIMode) == "" && !trace.ParallelToolCalls {
		return RequestTrace{}, false
	}
	return trace, true
}
