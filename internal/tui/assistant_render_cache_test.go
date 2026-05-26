package tui

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestAssistantContentCacheKeyDependsOnRenderInputs(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	altTheme := defaultTheme
	altTheme.Palette.Text = "#ffffff"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})
	otherThemeModel := model
	otherThemeModel.theme = &altTheme

	base := assistantContentCacheKey(model, "# Plan", 40, "")
	if base == "" {
		t.Fatalf("assistantContentCacheKey() returned empty key")
	}
	if got := assistantContentCacheKey(model, "# Plan", 40, ""); got != base {
		t.Fatalf("assistantContentCacheKey() unstable for identical inputs")
	}
	if got := assistantContentCacheKey(model, "# Plan", 24, ""); got == base {
		t.Fatalf("assistantContentCacheKey() did not vary with width")
	}
	if got := assistantContentCacheKey(model, "# Plan", 40, "#111111"); got == base {
		t.Fatalf("assistantContentCacheKey() did not vary with background")
	}
	if got := assistantContentCacheKey(model, "# Different", 40, ""); got == base {
		t.Fatalf("assistantContentCacheKey() did not vary with body")
	}
	if got := assistantContentCacheKey(otherThemeModel, "# Plan", 40, ""); got == base {
		t.Fatalf("assistantContentCacheKey() did not vary with theme")
	}
}
