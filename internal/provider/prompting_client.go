package provider

import "context"

type promptingClient struct {
	next Client
}

func NewPromptingClient(next Client) Client {
	if next == nil {
		return nil
	}
	return promptingClient{next: next}
}

func (c promptingClient) Stream(ctx context.Context, req Request) (Stream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	req = PreparePromptRequest(req)
	return c.next.Stream(ctx, req)
}

func (c promptingClient) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	if err := req.Validate(); err != nil {
		return 0, "", err
	}
	req = PreparePromptRequest(req)
	if counter, ok := c.next.(requestTokenCounter); ok {
		return counter.CountTokens(ctx, req)
	}
	return EstimateRequestTokens(req), TokenCountSourceEstimated, nil
}

func (c promptingClient) UnwrapProviderClient() Client {
	return c.next
}
