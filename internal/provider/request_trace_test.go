package provider

import (
	"testing"
)

func TestRequestTracedStreamForwardsFinishReasonAndUsage(t *testing.T) {
	stream := withRequestTrace(
		&traceWrappedMetadataStream{
			finishReason: FinishReasonLength,
			usage: UsageReport{
				RequestID:    "request-1",
				InputTokens:  12,
				OutputTokens: 3,
				TotalTokens:  15,
			},
		},
		RequestTrace{APIMode: "chat_completions", ParallelToolCalls: true},
	)

	trace, ok := StreamRequestTrace(stream)
	if !ok || trace.APIMode != "chat_completions" || !trace.ParallelToolCalls {
		t.Fatalf("StreamRequestTrace() = %#v, %v", trace, ok)
	}
	if got := StreamFinishReason(stream); got != FinishReasonLength {
		t.Fatalf("StreamFinishReason() = %q, want %q", got, FinishReasonLength)
	}
	usage, ok := StreamUsageReport(stream)
	if !ok {
		t.Fatal("StreamUsageReport() ok = false")
	}
	if usage.RequestID != "request-1" || usage.InputTokens != 12 || usage.OutputTokens != 3 || usage.TotalTokens != 15 {
		t.Fatalf("usage = %#v", usage)
	}
}

type traceWrappedMetadataStream struct {
	SliceStream
	finishReason FinishReason
	usage        UsageReport
}

func (s *traceWrappedMetadataStream) FinishReason() FinishReason {
	return s.finishReason
}

func (s *traceWrappedMetadataStream) UsageReport() (UsageReport, bool) {
	return s.usage, true
}
