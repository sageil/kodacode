package app

import (
	"errors"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

type stepAttemptCompleteTestStream struct {
	closed bool
	report provider.UsageReport
}

func (s *stepAttemptCompleteTestStream) Recv() (provider.Event, error) {
	return provider.Event{}, errors.New("unused")
}

func (s *stepAttemptCompleteTestStream) Close() error {
	s.closed = true
	return nil
}

func (s *stepAttemptCompleteTestStream) UsageReport() (provider.UsageReport, bool) {
	return s.report, true
}

func TestStepAttemptCompleterClosesStreamAndCapturesUsage(t *testing.T) {
	stream := &stepAttemptCompleteTestStream{report: provider.UsageReport{RequestID: "request-1", OutputTokens: 7}}
	streamClosed := false
	durableProgress := true
	committedState := turnLoopState{AssistantText: "done"}
	completionTokens := 12
	toolCallStarted := true
	completer := newStepAttemptCompleter(stepAttemptCompleterInput{
		Stream:           stream,
		StreamClosed:     &streamClosed,
		RequestStarted:   true,
		DurableProgress:  &durableProgress,
		CommittedState:   &committedState,
		CompletionTokens: &completionTokens,
		ToolCallStarted:  &toolCallStarted,
		StartedAt:        time.Now().Add(-time.Second),
	})

	result := completer.Complete(assistantRoundtripResult{Outcome: assistantRoundtripOutcomeAssistantDone}, errors.New("boom"))
	if !stream.closed || !streamClosed {
		t.Fatalf("stream closed=%v streamClosed=%v", stream.closed, streamClosed)
	}
	if result.Error == nil || result.Result.Outcome != assistantRoundtripOutcomeAssistantDone {
		t.Fatalf("result = %#v", result)
	}
	if !result.RequestStarted || !result.DurableProgress || !result.ToolCallStarted || result.CompletionTokens != 12 {
		t.Fatalf("attempt result = %#v", result)
	}
	if result.CommittedState.AssistantText != "done" {
		t.Fatalf("CommittedState = %#v", result.CommittedState)
	}
	if result.UsageReport == nil || result.UsageReport.RequestID != "request-1" || result.UsageReport.OutputTokens != 7 {
		t.Fatalf("UsageReport = %#v", result.UsageReport)
	}
	if result.Duration <= 0 {
		t.Fatalf("Duration = %s", result.Duration)
	}
}

func TestStepAttemptCompleterDoesNotCloseStreamTwice(t *testing.T) {
	stream := &stepAttemptCompleteTestStream{}
	streamClosed := true
	completer := newStepAttemptCompleter(stepAttemptCompleterInput{
		Stream:       stream,
		StreamClosed: &streamClosed,
		StartedAt:    time.Now(),
	})

	_ = completer.Complete(assistantRoundtripResult{}, nil)
	if stream.closed {
		t.Fatal("stream was closed after streamClosed was already true")
	}
}
