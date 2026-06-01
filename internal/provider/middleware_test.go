package provider

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
)

type middlewareTestClient struct {
	events []string
	stream Stream
	tokens int
	source TokenCountSource
}

func (c *middlewareTestClient) Stream(context.Context, Request) (Stream, error) {
	c.events = append(c.events, "client stream")
	return c.stream, nil
}

func (c *middlewareTestClient) CountTokens(context.Context, Request) (int, TokenCountSource, error) {
	c.events = append(c.events, "client count")
	return c.tokens, c.source, nil
}

type middlewareTestHandler struct {
	streamFunc func(context.Context, Request) (Stream, error)
	countFunc  func(context.Context, Request) (int, TokenCountSource, error)
}

func (h middlewareTestHandler) Stream(ctx context.Context, req Request) (Stream, error) {
	return h.streamFunc(ctx, req)
}

func (h middlewareTestHandler) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	return h.countFunc(ctx, req)
}

func TestWrapClientAppliesMiddlewareAroundStreamAndCountTokens(t *testing.T) {
	client := &middlewareTestClient{
		stream: NewSliceStream([]Event{{Kind: EventKindAssistantDelta, AssistantDelta: "ok"}}),
		tokens: 17,
		source: TokenCountSourceExact,
	}
	first := recordingMiddleware(&client.events, "first")
	second := recordingMiddleware(&client.events, "second")

	wrapped, err := WrapClient(client, first, second)
	if err != nil {
		t.Fatalf("WrapClient() error = %v", err)
	}

	stream, err := wrapped.Stream(context.Background(), middlewareTestRequest())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.AssistantDelta != "ok" {
		t.Fatalf("event delta = %q, want ok", event.AssistantDelta)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}

	counter, ok := wrapped.(requestTokenCounter)
	if !ok {
		t.Fatal("wrapped client does not expose CountTokens")
	}
	tokens, source, err := counter.CountTokens(context.Background(), middlewareTestRequest())
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	if tokens != 17 || source != TokenCountSourceExact {
		t.Fatalf("CountTokens() = (%d, %q), want (17, exact)", tokens, source)
	}

	want := []string{
		"first stream before",
		"second stream before",
		"client stream",
		"second stream after",
		"first stream after",
		"first count before",
		"second count before",
		"client count",
		"second count after",
		"first count after",
	}
	if !reflect.DeepEqual(client.events, want) {
		t.Fatalf("events = %#v, want %#v", client.events, want)
	}
}

func TestWrapClientRejectsInvalidMiddleware(t *testing.T) {
	_, err := WrapClient(&middlewareTestClient{}, nil)
	if !errors.Is(err, ErrProviderMiddlewareRequired) {
		t.Fatalf("WrapClient() error = %v, want ErrProviderMiddlewareRequired", err)
	}

	_, err = WrapClient(&middlewareTestClient{}, func(ProviderHandler) ProviderHandler {
		return nil
	})
	if !errors.Is(err, ErrProviderMiddlewareHandlerRequired) {
		t.Fatalf("WrapClient() error = %v, want ErrProviderMiddlewareHandlerRequired", err)
	}
}

func TestUnwrapClientReturnsOriginalClient(t *testing.T) {
	client := &middlewareTestClient{}
	wrapped, err := WrapClient(client, recordingMiddleware(&client.events, "first"))
	if err != nil {
		t.Fatalf("WrapClient() error = %v", err)
	}
	if got := UnwrapClient(wrapped); got != client {
		t.Fatalf("UnwrapClient() = %#v, want original client", got)
	}
}

func recordingMiddleware(events *[]string, name string) Middleware {
	return func(next ProviderHandler) ProviderHandler {
		return middlewareTestHandler{
			streamFunc: func(ctx context.Context, req Request) (Stream, error) {
				*events = append(*events, name+" stream before")
				stream, err := next.Stream(ctx, req)
				*events = append(*events, name+" stream after")
				return stream, err
			},
			countFunc: func(ctx context.Context, req Request) (int, TokenCountSource, error) {
				*events = append(*events, name+" count before")
				tokens, source, err := next.CountTokens(ctx, req)
				*events = append(*events, name+" count after")
				return tokens, source, err
			},
		}
	}
}

func middlewareTestRequest() Request {
	return Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "engineer",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "respond",
		Inputs: []Input{{
			Kind:    InputKindUserMessage,
			Content: "hello",
		}},
	}
}
