package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderMarkdownLinesOnSurfaceWithStreamKeyMatchesFullRenderAcrossStreamingUpdates(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	restoreRenderCache := swapMarkdownSurfaceRenderCacheForTest(newLRUCache(32, 256<<10))
	defer restoreRenderCache()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})
	model.renderCache.transcriptMarkdown = newStreamingMarkdownSurfaceCache(8)

	updates := []string{
		"# Plan\n\n- first item",
		"# Plan\n\n- first item\n- second item",
		"# Plan\n\n- first item\n- second item\n\n```go\nfmt.Println(\"hi\")",
		"# Plan\n\n- first item\n- second item\n\n```go\nfmt.Println(\"hi\")\n```",
		"# Plan\n\n- first item\n- second item\n\n```go\nfmt.Println(\"hi\")\n```\n\nFinal note.",
	}

	const streamKey = "assistant:session-1:turn-1:preview:block:0"
	for i, body := range updates {
		got := strings.Join(renderMarkdownLinesOnSurfaceWithStreamKey(model, body, 52, "", streamKey), "\n")
		want := strings.Join(renderMarkdownLinesOnSurface(model, body, 52, ""), "\n")
		if got != want {
			t.Fatalf("stream render step %d mismatch\nbody:\n%s\n\ngot:\n%s\n\nwant:\n%s", i, body, got, want)
		}
	}
}

func TestStreamingMarkdownSurfaceSkipsUnsafeOpenFencePrefix(t *testing.T) {
	var state streamingMarkdownSurface
	render := func(content string) string {
		return strings.Trim(content, "\n")
	}

	partial := "```go\nfmt.Println(\"hi\")"
	if got := state.Render(partial, "ctx", render); got != partial {
		t.Fatalf("partial render = %q, want %q", got, partial)
	}
	if state.stablePrefix != "" {
		t.Fatalf("stablePrefix = %q, want empty while fence is open", state.stablePrefix)
	}

	complete := partial + "\n```\n\nDone."
	if got := state.Render(complete, "ctx", render); got != complete {
		t.Fatalf("completed render = %q, want %q", got, complete)
	}
	if state.stablePrefix == "" {
		t.Fatal("stablePrefix = empty, want cached closed-fence prefix")
	}
	if !strings.HasPrefix(complete, state.stablePrefix) {
		t.Fatalf("stablePrefix = %q, want prefix of completed content", state.stablePrefix)
	}
}

func TestStreamingMarkdownSurfaceResetsOnContextChange(t *testing.T) {
	var state streamingMarkdownSurface
	render := func(content string) string {
		return strings.Trim(content, "\n")
	}

	first := "# First\n\nBody."
	second := "# Second\n\nDifferent body."

	_ = state.Render(first, "ctx-a", render)
	if state.stablePrefix == "" {
		t.Fatal("stablePrefix = empty after first render, want cached prefix")
	}
	if state.contextKey != "ctx-a" {
		t.Fatalf("contextKey = %q, want %q", state.contextKey, "ctx-a")
	}

	_ = state.Render(second, "ctx-b", render)
	if state.contextKey != "ctx-b" {
		t.Fatalf("contextKey = %q, want %q", state.contextKey, "ctx-b")
	}
	if state.stablePrefix == "" {
		t.Fatal("stablePrefix = empty after context reset, want recached prefix")
	}
	if !strings.HasPrefix(second, state.stablePrefix) {
		t.Fatalf("stablePrefix = %q, want prefix of second content", state.stablePrefix)
	}
	if strings.HasPrefix(first, state.stablePrefix) {
		t.Fatalf("stablePrefix = %q, want reset away from first content", state.stablePrefix)
	}
}

func TestStreamingMarkdownSurfaceCacheOwnersDoNotShareState(t *testing.T) {
	cacheA := newStreamingMarkdownSurfaceCache(4)
	cacheB := newStreamingMarkdownSurfaceCache(4)
	render := func(content string) string {
		return strings.Trim(content, "\n")
	}

	const streamKey = "shared"
	_ = cacheA.Render(streamKey, "ctx-a", "# First\n\nBody.", render)
	_ = cacheB.Render(streamKey, "ctx-b", "# Second\n\nBody.", render)

	stateA, ok := cacheA.cache.get(streamKey)
	if !ok {
		t.Fatal("cacheA missing stream state")
	}
	stateB, ok := cacheB.cache.get(streamKey)
	if !ok {
		t.Fatal("cacheB missing stream state")
	}
	if stateA.contextKey != "ctx-a" {
		t.Fatalf("cacheA contextKey = %q, want %q", stateA.contextKey, "ctx-a")
	}
	if stateB.contextKey != "ctx-b" {
		t.Fatalf("cacheB contextKey = %q, want %q", stateB.contextKey, "ctx-b")
	}
}

func TestStreamingMarkdownSurfaceCacheBoundsRetainedBytes(t *testing.T) {
	cache := newStreamingMarkdownSurfaceCache(4)
	cache.cache.maxBytes = 96

	cache.cache.put("first", streamingMarkdownSurface{
		contextKey:         "ctx-a",
		stablePrefix:       "alpha",
		stablePrefixRender: "alpha",
	})
	cache.cache.put("second", streamingMarkdownSurface{
		contextKey:         "ctx-b",
		stablePrefix:       strings.Repeat("b", 48),
		stablePrefixRender: strings.Repeat("B", 48),
	})

	if cache.cache.bytes > cache.cache.maxBytes {
		t.Fatalf("stream cache bytes = %d, want <= %d", cache.cache.bytes, cache.cache.maxBytes)
	}
	if _, ok := cache.cache.entries["second"]; ok {
		t.Fatal("oversized stream cache entry was retained")
	}
	if len(cache.cache.entries) != 1 {
		t.Fatalf("stream cache entry count = %d, want 1 retained small entry", len(cache.cache.entries))
	}
}
