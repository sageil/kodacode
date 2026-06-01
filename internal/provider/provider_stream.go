package provider

import (
	"context"
	"io"
)

type Stream interface {
	Recv() (Event, error)
	Close() error
}

type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonLength        FinishReason = "length"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonError         FinishReason = "error"
	FinishReasonUnknown       FinishReason = "unknown"
)

type UsageReport struct {
	RequestID             string
	Model                 string
	InputTokens           int
	CacheReadInputTokens  int
	CacheWriteInputTokens int
	OutputTokens          int
	ReasoningTokens       int
	TotalTokens           int
}

type TokenCountSource string

const (
	TokenCountSourceEstimated TokenCountSource = "estimated"
	TokenCountSourceExact     TokenCountSource = "exact"
)

type usageReportCarrier interface {
	UsageReport() (UsageReport, bool)
}

type finishReasonCarrier interface {
	FinishReason() FinishReason
}

type requestTokenCounter interface {
	CountTokens(context.Context, Request) (int, TokenCountSource, error)
}

type Client interface {
	Stream(context.Context, Request) (Stream, error)
}

func CountRequestTokens(ctx context.Context, client Client, req Request) (int, TokenCountSource, error) {
	req = sanitizeMalformedToolReplayRequest(req)
	estimatedRequest := PreparePromptRequest(req)
	estimated := EstimateRequestTokens(estimatedRequest)
	req = NormalizePromptRequest(req)
	req = normalizeConversationToolCallIDs(req)
	if client == nil {
		return estimated, TokenCountSourceEstimated, nil
	}
	counter, ok := client.(requestTokenCounter)
	if !ok {
		return estimated, TokenCountSourceEstimated, nil
	}
	tokens, source, err := counter.CountTokens(ctx, req)
	if err != nil {
		return estimated, TokenCountSourceEstimated, err
	}
	if tokens < 0 {
		tokens = 0
	}
	if source == "" {
		source = TokenCountSourceExact
	}
	return tokens, source, nil
}

func StreamUsageReport(stream Stream) (UsageReport, bool) {
	if stream == nil {
		return UsageReport{}, false
	}
	reporter, ok := stream.(usageReportCarrier)
	if !ok {
		return UsageReport{}, false
	}
	return reporter.UsageReport()
}

func StreamFinishReason(stream Stream) FinishReason {
	if stream == nil {
		return FinishReasonUnknown
	}
	carrier, ok := stream.(finishReasonCarrier)
	if !ok {
		return FinishReasonUnknown
	}
	return NormalizeFinishReason(carrier.FinishReason())
}

func NormalizeFinishReason(reason FinishReason) FinishReason {
	switch reason {
	case FinishReasonStop, FinishReasonToolCalls, FinishReasonLength, FinishReasonContentFilter, FinishReasonError:
		return reason
	default:
		return FinishReasonUnknown
	}
}

type SliceStream struct {
	events       []Event
	index        int
	finishReason FinishReason
}

func NewSliceStream(events []Event) *SliceStream {
	copied := make([]Event, len(events))
	copy(copied, events)
	return &SliceStream{events: copied}
}

func NewSliceStreamWithFinishReason(events []Event, reason FinishReason) *SliceStream {
	stream := NewSliceStream(events)
	stream.finishReason = NormalizeFinishReason(reason)
	return stream
}

func (s *SliceStream) Recv() (Event, error) {
	if s.index >= len(s.events) {
		return Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *SliceStream) Close() error {
	if s == nil {
		return nil
	}
	s.index = len(s.events)
	return nil
}

func (s *SliceStream) FinishReason() FinishReason {
	if s == nil {
		return FinishReasonUnknown
	}
	return NormalizeFinishReason(s.finishReason)
}
