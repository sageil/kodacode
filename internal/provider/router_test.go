package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRoutedClientUsesPrimaryModelProvider(t *testing.T) {
	openai := &stubClient{stream: NewSliceStream([]Event{{Kind: EventKindAssistantDelta, AssistantDelta: "ok"}})}
	router, err := NewRoutedClient(map[string]Client{
		"openai": openai,
	})
	if err != nil {
		t.Fatalf("NewRoutedClient() error = %v", err)
	}

	stream, err := router.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if len(openai.requests) != 1 || openai.requests[0].Model.String() != "openai/gpt-5" {
		t.Fatalf("requests = %#v", openai.requests)
	}
	trace, ok := StreamRouteTrace(stream)
	if !ok {
		t.Fatal("StreamRouteTrace() ok = false, want true")
	}
	selected, ok := trace.SelectedModel()
	if !ok || selected.String() != "openai/gpt-5" {
		t.Fatalf("SelectedModel = (%q, %t), want openai/gpt-5 true", selected.String(), ok)
	}
}

func TestRoutedClientReturnsRouteErrorForFailedProvider(t *testing.T) {
	openai := &stubClient{err: errors.New("primary failed")}
	router, err := NewRoutedClient(map[string]Client{
		"proxy": openai,
	})
	if err != nil {
		t.Fatalf("NewRoutedClient() error = %v", err)
	}

	_, err = router.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "proxy", ModelID: "primary"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("Stream() error = nil, want route error")
	}
	if len(openai.requests) != 1 || openai.requests[0].Model.String() != "proxy/primary" {
		t.Fatalf("requests = %#v", openai.requests)
	}
	trace, ok := ErrorRouteTrace(err)
	if !ok {
		t.Fatal("ErrorRouteTrace() ok = false, want true")
	}
	if len(trace.Attempts) != 1 {
		t.Fatalf("attempt count = %d, want 1", len(trace.Attempts))
	}
	if trace.Attempts[0].Model.String() != "proxy/primary" || trace.Attempts[0].Selected || trace.Attempts[0].Error == nil {
		t.Fatalf("attempt = %#v", trace.Attempts[0])
	}
}

func TestRoutedClientReportsRouteAttempts(t *testing.T) {
	primary := &stubClient{stream: NewSliceStream([]Event{{Kind: EventKindAssistantDelta, AssistantDelta: "ok"}})}
	router, err := NewRoutedClient(map[string]Client{
		"proxy": primary,
	})
	if err != nil {
		t.Fatalf("NewRoutedClient() error = %v", err)
	}

	var attempts []RouteAttempt
	router.SetRouteObserver(func(attempt RouteAttempt) {
		attempts = append(attempts, attempt)
	})

	stream, err := router.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "proxy", ModelID: "primary"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempt count = %d, want 1", len(attempts))
	}
	if attempts[0].Attempt != 1 || !attempts[0].Selected || attempts[0].Error != nil {
		t.Fatalf("attempt = %#v", attempts[0])
	}
	if attempts[0].Model.String() != "proxy/primary" {
		t.Fatalf("model = %q", attempts[0].Model.String())
	}
}

func TestRoutedClientSanitizesMalformedToolReplayBeforeProvider(t *testing.T) {
	openai := &stubClient{stream: NewSliceStream([]Event{{Kind: EventKindAssistantDelta, AssistantDelta: "ok"}})}
	router, err := NewRoutedClient(map[string]Client{
		"openai": openai,
	})
	if err != nil {
		t.Fatalf("NewRoutedClient() error = %v", err)
	}

	stream, err := router.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "continue"},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["README.md"]`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Error: "`read` failed. JSON ended before the object was complete."},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if len(openai.requests) != 1 {
		t.Fatalf("requests = %#v", openai.requests)
	}
	if len(openai.requests[0].Inputs) != 2 {
		t.Fatalf("inputs = %#v", openai.requests[0].Inputs)
	}
	if openai.requests[0].Inputs[1].Kind != InputKindAssistantMessage {
		t.Fatalf("input[1] = %#v, want assistant summary", openai.requests[0].Inputs[1])
	}
	if got := openai.requests[0].Inputs[1].Content; !strings.Contains(got, "malformed JSON") || !strings.Contains(got, "read tool call") {
		t.Fatalf("input[1].Content = %q", got)
	}
}

func TestRoutedClientAllowsEmptyClientMap(t *testing.T) {
	router, err := NewRoutedClient(nil)
	if err != nil {
		t.Fatalf("NewRoutedClient() error = %v", err)
	}

	_, err = router.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("Stream() error = %v, want ErrProviderNotConfigured", err)
	}
	trace, ok := ErrorRouteTrace(err)
	if !ok {
		t.Fatal("ErrorRouteTrace() ok = false, want true")
	}
	if trace.RequestedModel.String() != "openai/gpt-5" {
		t.Fatalf("RequestedModel = %q", trace.RequestedModel.String())
	}
	if len(trace.Attempts) != 1 || trace.Attempts[0].Model.String() != "openai/gpt-5" {
		t.Fatalf("Attempts = %#v", trace.Attempts)
	}
}

type stubClient struct {
	stream   Stream
	err      error
	requests []Request
}

func (s *stubClient) Stream(_ context.Context, req Request) (Stream, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.stream, nil
}
