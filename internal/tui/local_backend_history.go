package tui

import (
	"context"

	"github.com/sageil/kodacode/internal/app"
)

func (b *LocalBackend) ListPromptHistory(ctx context.Context, limit int) ([]app.PromptHistoryEntry, error) {
	return b.runtime.ListPromptHistory(ctx, limit)
}
