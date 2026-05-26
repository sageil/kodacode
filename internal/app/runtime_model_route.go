package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

func (r *Runtime) SetSessionModelRoute(ctx context.Context, sessionID string, route provider.ModelRoute) error {
	if r == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return r.syncSessionModelRoute(ctx, sessionID, route)
}

func (r *Runtime) ValidateModelRoute(route provider.ModelRoute) error {
	if !hasConfiguredModelRoute(route) {
		return ErrModelSelectionRequired
	}
	if err := route.Validate(); err != nil {
		return err
	}
	return r.Config.validateModelRouteReference(route)
}
