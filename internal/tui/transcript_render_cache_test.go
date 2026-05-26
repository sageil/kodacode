package tui

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTranscriptRenderCacheKeyDependsOnRenderInputs(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	altTheme := defaultTheme
	altTheme.Palette.Primary = "#ffffff"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})
	otherThemeModel := model
	otherThemeModel.theme = &altTheme

	base := transcriptRenderCacheKey("transcript_block", model, 72, "Plan", "body", "#7cc7ff", "0")
	if base == "" {
		t.Fatalf("transcriptRenderCacheKey() returned empty key")
	}
	if got := transcriptRenderCacheKey("transcript_block", model, 72, "Plan", "body", "#7cc7ff", "0"); got != base {
		t.Fatalf("transcriptRenderCacheKey() unstable for identical inputs")
	}
	if got := transcriptRenderCacheKey("transcript_block", model, 60, "Plan", "body", "#7cc7ff", "0"); got == base {
		t.Fatalf("transcriptRenderCacheKey() did not vary with width")
	}
	if got := transcriptRenderCacheKey("transcript_block", model, 72, "Plan", "body", "#7cc7ff", "1"); got == base {
		t.Fatalf("transcriptRenderCacheKey() did not vary with alignRight")
	}
	if got := transcriptRenderCacheKey("wide_tool_section", model, 72, "Plan", "body", "#7cc7ff", "0"); got == base {
		t.Fatalf("transcriptRenderCacheKey() did not vary with kind")
	}
	if got := transcriptRenderCacheKey("transcript_block", model, 72, "Plan", "other", "#7cc7ff", "0"); got == base {
		t.Fatalf("transcriptRenderCacheKey() did not vary with content")
	}
	if got := transcriptRenderCacheKey("transcript_block", otherThemeModel, 72, "Plan", "body", "#7cc7ff", "0"); got == base {
		t.Fatalf("transcriptRenderCacheKey() did not vary with theme")
	}
}
