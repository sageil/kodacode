package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/workspace"
)

type workspaceSessionLocator interface {
	LatestWorkspaceSessionID(ctx context.Context, workspaceRoot string) (string, bool, error)
}

func latestWorkspaceSessionID(ctx context.Context, store events.ReplayStore, workspaceRoot string) (string, bool, error) {
	scope, err := workspace.New(workspaceRoot)
	if err != nil {
		return "", false, err
	}
	locator, ok := store.(workspaceSessionLocator)
	if !ok {
		return "", false, nil
	}
	return locator.LatestWorkspaceSessionID(ctx, scope.Root())
}
