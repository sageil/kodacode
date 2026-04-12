package sandbox

import "context"

type contextKey struct{}

// WithContext stores s in ctx for retrieval by middleware downstream.
func WithContext(ctx context.Context, s *Sandbox) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// FromContext retrieves the Sandbox stored by WithContext.
// Returns nil if none was stored.
func FromContext(ctx context.Context) *Sandbox {
	s, _ := ctx.Value(contextKey{}).(*Sandbox)
	return s
}
