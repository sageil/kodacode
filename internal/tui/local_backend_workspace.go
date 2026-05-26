package tui

import (
	"context"

	"github.com/sageil/kodacode/internal/app"
)

func (b *LocalBackend) ListWorkspacePaths(_ context.Context, workspaceRoot string) ([]app.WorkspacePath, error) {
	return app.ListWorkspacePaths(workspaceRoot)
}
