package tui

import (
	"strconv"
	"strings"
	"sync"
)

var markdownSurfaceRenderCache = struct {
	mu    sync.Mutex
	cache *lruCache
}{
	cache: newLRUCache(128, 4<<20),
}

func cachedMarkdownSurfaceLines(kind string, m Model, text string, width int, bg string, streamCache *streamingMarkdownSurfaceCache, streamKey string, render func(string) string) []string {
	contextKey := markdownSurfaceRenderContextKey(kind, m, width, bg)
	key := markdownSurfaceRenderCacheKeyFromContext(contextKey, text)

	markdownSurfaceRenderCache.mu.Lock()
	cached, ok := markdownSurfaceRenderCache.cache.get(key)
	markdownSurfaceRenderCache.mu.Unlock()
	if ok {
		return strings.Split(cached, "\n")
	}

	serialized := cachedMarkdownSurfaceRender(kind, text, contextKey, streamCache, streamKey, render)

	markdownSurfaceRenderCache.mu.Lock()
	markdownSurfaceRenderCache.cache.put(key, serialized)
	markdownSurfaceRenderCache.mu.Unlock()
	return strings.Split(serialized, "\n")
}

func markdownSurfaceRenderCacheKey(kind string, m Model, text string, width int, bg string) string {
	return markdownSurfaceRenderCacheKeyFromContext(markdownSurfaceRenderContextKey(kind, m, width, bg), text)
}

func markdownSurfaceRenderCacheKeyFromContext(contextKey string, text string) string {
	var b strings.Builder
	b.Grow(len(text) + 160)
	b.WriteString(contextKey)
	b.WriteString("\x00")
	b.WriteString(text)
	return b.String()
}

func markdownSurfaceRenderContextKey(kind string, m Model, width int, bg string) string {
	var b strings.Builder
	b.Grow(160)
	b.WriteString(strings.TrimSpace(kind))
	b.WriteString("\x00")
	b.WriteString(strconv.Itoa(max(width, 1)))
	b.WriteString("\x00")
	b.WriteString(strings.TrimSpace(bg))
	b.WriteString("\x00")
	b.WriteString(modelRenderCacheKey(m))
	return b.String()
}

func cachedMarkdownSurfaceRender(kind string, text string, contextKey string, streamCache *streamingMarkdownSurfaceCache, streamKey string, render func(string) string) string {
	if strings.TrimSpace(streamKey) == "" || strings.TrimSpace(kind) != "markdown_surface" {
		return render(text)
	}
	if streamCache == nil {
		return render(text)
	}
	return streamCache.Render(streamKey, contextKey, text, render)
}
