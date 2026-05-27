package tui

import (
	"strconv"
	"strings"
	"sync"
)

var transcriptRenderCache = struct {
	mu    sync.Mutex
	cache *lruCache
}{
	cache: newLRUCache(256, 4<<20),
}

func cachedTranscriptRender(kind string, m Model, width int, render func() string, parts ...string) string {
	key := transcriptRenderCacheKey(kind, m, width, parts...)

	transcriptRenderCache.mu.Lock()
	cached, ok := transcriptRenderCache.cache.get(key)
	transcriptRenderCache.mu.Unlock()
	if ok {
		return cached
	}

	rendered := render()

	transcriptRenderCache.mu.Lock()
	transcriptRenderCache.cache.put(key, rendered)
	transcriptRenderCache.mu.Unlock()
	return rendered
}

func transcriptRenderCacheKey(kind string, m Model, width int, parts ...string) string {
	var b strings.Builder
	totalLen := len(kind) + len(parts)*4 + 128
	for _, part := range parts {
		totalLen += len(part)
	}
	b.Grow(totalLen)
	b.WriteString(kind)
	b.WriteString("\x00")
	b.WriteString(strconv.Itoa(max(width, 1)))
	b.WriteString("\x00")
	b.WriteString(modelRenderCacheKey(m))
	for _, part := range parts {
		b.WriteString("\x00")
		b.WriteString(part)
	}
	return b.String()
}
