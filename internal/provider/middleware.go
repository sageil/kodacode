package provider

import (
	"context"
	"errors"
)

var (
	ErrProviderMiddlewareRequired        = errors.New("provider middleware is required")
	ErrProviderMiddlewareHandlerRequired = errors.New("provider middleware handler is required")
)

type ProviderHandler interface {
	Stream(context.Context, Request) (Stream, error)
	CountTokens(context.Context, Request) (int, TokenCountSource, error)
}

type Middleware func(ProviderHandler) ProviderHandler

type middlewareClient struct {
	next    Client
	handler ProviderHandler
}

type clientHandler struct {
	client Client
}

type providerClientUnwrapper interface {
	UnwrapProviderClient() Client
}

func WrapClient(client Client, middleware ...Middleware) (Client, error) {
	if client == nil || len(middleware) == 0 {
		return client, nil
	}

	handler := ProviderHandler(clientHandler{client: client})
	applied := false
	for index := len(middleware) - 1; index >= 0; index-- {
		current := middleware[index]
		if current == nil {
			return nil, ErrProviderMiddlewareRequired
		}
		handler = current(handler)
		if handler == nil {
			return nil, ErrProviderMiddlewareHandlerRequired
		}
		applied = true
	}
	if !applied {
		return client, nil
	}
	return middlewareClient{
		next:    client,
		handler: handler,
	}, nil
}

func UnwrapClient(client Client) Client {
	for client != nil {
		unwrapper, ok := client.(providerClientUnwrapper)
		if !ok {
			return client
		}
		next := unwrapper.UnwrapProviderClient()
		if next == nil || next == client {
			return client
		}
		client = next
	}
	return nil
}

func (c middlewareClient) Stream(ctx context.Context, req Request) (Stream, error) {
	return c.handler.Stream(ctx, req)
}

func (c middlewareClient) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	return c.handler.CountTokens(ctx, req)
}

func (c middlewareClient) UnwrapProviderClient() Client {
	return c.next
}

func (h clientHandler) Stream(ctx context.Context, req Request) (Stream, error) {
	return h.client.Stream(ctx, req)
}

func (h clientHandler) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	if counter, ok := h.client.(requestTokenCounter); ok {
		return counter.CountTokens(ctx, req)
	}
	return EstimateRequestTokens(req), TokenCountSourceEstimated, nil
}
