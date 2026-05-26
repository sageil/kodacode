package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrProviderRouteIDRequired   = errors.New("provider route id is required")
	ErrProviderRouteClientNeeded = errors.New("provider route client is required")
	ErrProviderNotConfigured     = errors.New("provider not configured")
)

type RoutedClient struct {
	clients   map[string]Client
	onAttempt func(RouteAttempt)
}

type RouteTrace struct {
	RequestedModel ModelRef
	Attempts       []RouteAttempt
}

type RouteAttempt struct {
	SessionID string
	TurnID    string
	AgentID   string
	Attempt   int
	Model     ModelRef
	Selected  bool
	Error     error
}

func (t RouteTrace) SelectedModel() (ModelRef, bool) {
	for _, attempt := range t.Attempts {
		if attempt.Selected {
			return attempt.Model, true
		}
	}
	return ModelRef{}, false
}

type routeTraceCarrier interface {
	RouteTrace() RouteTrace
}

type routedStream struct {
	Stream
	trace RouteTrace
}

func (s *routedStream) RouteTrace() RouteTrace {
	if s == nil {
		return RouteTrace{}
	}
	return cloneRouteTrace(s.trace)
}

func (s *routedStream) RequestTrace() RequestTrace {
	if s == nil {
		return RequestTrace{}
	}
	trace, _ := StreamRequestTrace(s.Stream)
	return trace
}

func (s *routedStream) UsageReport() (UsageReport, bool) {
	if s == nil {
		return UsageReport{}, false
	}
	return StreamUsageReport(s.Stream)
}

func (s *routedStream) FinishReason() FinishReason {
	if s == nil {
		return FinishReasonUnknown
	}
	return StreamFinishReason(s.Stream)
}

type RouteError struct {
	trace  RouteTrace
	causes []error
}

func (e *RouteError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.causes) == 0 {
		return "provider route failed"
	}
	return errors.Join(e.causes...).Error()
}

func (e *RouteError) Unwrap() []error {
	if e == nil || len(e.causes) == 0 {
		return nil
	}
	return append([]error(nil), e.causes...)
}

func (e *RouteError) RouteTrace() RouteTrace {
	if e == nil {
		return RouteTrace{}
	}
	return cloneRouteTrace(e.trace)
}

func StreamRouteTrace(stream Stream) (RouteTrace, bool) {
	if stream == nil {
		return RouteTrace{}, false
	}
	carrier, ok := stream.(routeTraceCarrier)
	if !ok {
		return RouteTrace{}, false
	}
	return carrier.RouteTrace(), true
}

func ErrorRouteTrace(err error) (RouteTrace, bool) {
	if err == nil {
		return RouteTrace{}, false
	}
	var carrier routeTraceCarrier
	if !errors.As(err, &carrier) {
		return RouteTrace{}, false
	}
	return carrier.RouteTrace(), true
}

func NewRoutedClient(clients map[string]Client) (*RoutedClient, error) {
	out := make(map[string]Client, len(clients))
	for id, client := range clients {
		name := strings.TrimSpace(id)
		if name == "" {
			return nil, ErrProviderRouteIDRequired
		}
		if client == nil {
			return nil, fmt.Errorf("%w: %s", ErrProviderRouteClientNeeded, name)
		}
		out[name] = client
	}
	return &RoutedClient{clients: out}, nil
}

func (c *RoutedClient) SetRouteObserver(observer func(RouteAttempt)) {
	if c == nil {
		return
	}
	c.onAttempt = observer
}

func (c *RoutedClient) Stream(ctx context.Context, req Request) (Stream, error) {
	req = sanitizeMalformedToolReplayRequest(req)
	if err := req.Validate(); err != nil {
		return nil, err
	}
	attempt := RouteAttempt{
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		AgentID:   req.AgentID,
		Attempt:   1,
		Model:     req.Model,
	}
	client, ok := c.clients[req.Model.ProviderID]
	if !ok {
		attempt.Error = ErrProviderNotConfigured
		c.emitAttempt(attempt)
		return nil, &RouteError{
			trace: RouteTrace{
				RequestedModel: req.Model,
				Attempts:       []RouteAttempt{attempt},
			},
			causes: []error{fmt.Errorf("%s: %w", req.Model.String(), ErrProviderNotConfigured)},
		}
	}
	stream, err := client.Stream(ctx, req)
	if err != nil {
		attempt.Error = err
		c.emitAttempt(attempt)
		return nil, &RouteError{
			trace: RouteTrace{
				RequestedModel: req.Model,
				Attempts:       []RouteAttempt{attempt},
			},
			causes: []error{fmt.Errorf("%s: %w", req.Model.String(), err)},
		}
	}
	attempt.Selected = true
	c.emitAttempt(attempt)
	return &routedStream{
		Stream: stream,
		trace: RouteTrace{
			RequestedModel: req.Model,
			Attempts:       []RouteAttempt{attempt},
		},
	}, nil
}

func (c *RoutedClient) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	req = sanitizeMalformedToolReplayRequest(req)
	if err := req.Validate(); err != nil {
		return 0, "", err
	}
	client, ok := c.clients[req.Model.ProviderID]
	if !ok {
		return 0, "", ErrProviderNotConfigured
	}
	if counter, ok := client.(requestTokenCounter); ok {
		return counter.CountTokens(ctx, req)
	}
	return EstimateRequestTokens(req), TokenCountSourceEstimated, nil
}

func (c *RoutedClient) emitAttempt(attempt RouteAttempt) {
	if c == nil || c.onAttempt == nil {
		return
	}
	c.onAttempt(attempt)
}

func cloneRouteTrace(trace RouteTrace) RouteTrace {
	cloned := trace
	cloned.Attempts = append([]RouteAttempt(nil), trace.Attempts...)
	return cloned
}
