package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestMarkdownSurfaceRenderCacheKeyDependsOnRenderInputs(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	altTheme := defaultTheme
	altTheme.SyntaxStyle = "rose-pine-moon"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})
	otherThemeModel := model
	otherThemeModel.theme = &altTheme

	base := markdownSurfaceRenderCacheKey("markdown_surface", model, "## Plan", 48, "#112233")
	if base == "" {
		t.Fatalf("markdownSurfaceRenderCacheKey() returned empty key")
	}
	if got := markdownSurfaceRenderCacheKey("markdown_surface", model, "## Plan", 48, "#112233"); got != base {
		t.Fatalf("markdownSurfaceRenderCacheKey() unstable for identical inputs")
	}
	if got := markdownSurfaceRenderCacheKey("literal_surface", model, "## Plan", 48, "#112233"); got == base {
		t.Fatalf("markdownSurfaceRenderCacheKey() did not vary with render kind")
	}
	if got := markdownSurfaceRenderCacheKey("markdown_surface", model, "## Plan", 36, "#112233"); got == base {
		t.Fatalf("markdownSurfaceRenderCacheKey() did not vary with width")
	}
	if got := markdownSurfaceRenderCacheKey("markdown_surface", model, "## Plan", 48, "#445566"); got == base {
		t.Fatalf("markdownSurfaceRenderCacheKey() did not vary with background")
	}
	if got := markdownSurfaceRenderCacheKey("markdown_surface", model, "## Different", 48, "#112233"); got == base {
		t.Fatalf("markdownSurfaceRenderCacheKey() did not vary with content")
	}
	if got := markdownSurfaceRenderCacheKey("markdown_surface", otherThemeModel, "## Plan", 48, "#112233"); got == base {
		t.Fatalf("markdownSurfaceRenderCacheKey() did not vary with theme")
	}
}

func TestMarkdownSurfaceRenderCacheKeepsBytesBounded(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	restore := swapMarkdownSurfaceRenderCacheForTest(newLRUCache(8, 32<<10))
	defer restore()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	for i := 1; i <= 24; i++ {
		body := "## Cache Layer\n\n" + strings.Repeat("- markdown output keeps growing with each render\n", i*24)
		rendered := renderMarkdownBlockOnSurface(model, body, 72, "")
		if len(rendered) == 0 {
			t.Fatal("renderMarkdownBlockOnSurface() returned no lines")
		}
	}

	if markdownSurfaceRenderCache.cache.bytes > markdownSurfaceRenderCache.cache.maxBytes {
		t.Fatalf("markdown surface cache bytes = %d, want <= %d", markdownSurfaceRenderCache.cache.bytes, markdownSurfaceRenderCache.cache.maxBytes)
	}
	if len(markdownSurfaceRenderCache.cache.entries) == 0 {
		t.Fatal("markdown surface cache = empty, want bounded retained entries")
	}
}

func swapMarkdownSurfaceRenderCacheForTest(cache *lruCache) func() {
	markdownSurfaceRenderCache.mu.Lock()
	prev := markdownSurfaceRenderCache.cache
	markdownSurfaceRenderCache.cache = cache
	markdownSurfaceRenderCache.mu.Unlock()

	return func() {
		markdownSurfaceRenderCache.mu.Lock()
		markdownSurfaceRenderCache.cache = prev
		markdownSurfaceRenderCache.mu.Unlock()
	}
}
