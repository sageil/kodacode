package sandbox

import (
	"context"

	"github.com/sageil/kodacode/v1/internal/pipeline"
)

// NewMiddleware returns a TurnMiddleware that stores s in the request context.
func NewMiddleware(s *Sandbox) pipeline.TurnMiddleware {
	return func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		ctx = WithContext(ctx, s)
		return next(ctx, req)
	}
}
