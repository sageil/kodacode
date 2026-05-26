package app

import (
	"context"
	"strings"
)

func (r *Runtime) SetSessionPermissionMode(ctx context.Context, sessionID string, mode PermissionMode) error {
	if r == nil || r.Sessions == nil {
		return ErrSessionServiceRequired
	}
	if strings.TrimSpace(sessionID) == "" {
		return ErrSessionIDRequired
	}
	_, err := r.Sessions.SetPermissionMode(ctx, sessionID, mode)
	return err
}
