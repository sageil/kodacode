package tui

import (
	"strconv"
	"strings"
	"sync"

	"github.com/sageil/kodacode/internal/tui/theme"
)

var assistantContentCache = struct {
	mu    sync.Mutex
	cache *lruCache
}{
	cache: newLRUCache(lruDefaultCap, 2<<20),
}

func cachedAssistantContentLines(m Model, body string, width int, bg string, streamKey string) []string {
	key := assistantContentCacheKey(m, body, width, bg)

	assistantContentCache.mu.Lock()
	cached, ok := assistantContentCache.cache.get(key)
	assistantContentCache.mu.Unlock()
	if ok {
		return strings.Split(cached, "\n")
	}

	rendered := renderAssistantContentLinesUncachedWithStreamKey(m, body, width, bg, streamKey)
	serialized := strings.Join(rendered, "\n")

	assistantContentCache.mu.Lock()
	assistantContentCache.cache.put(key, serialized)
	assistantContentCache.mu.Unlock()
	return rendered
}

func assistantContentCacheKey(m Model, body string, width int, bg string) string {
	var b strings.Builder
	b.Grow(len(body) + 160)
	b.WriteString("assistant\x00")
	b.WriteString(strconv.Itoa(max(width, 1)))
	b.WriteString("\x00")
	b.WriteString(strings.TrimSpace(bg))
	b.WriteString("\x00")
	b.WriteString(modelRenderCacheKey(m))
	b.WriteString("\x00")
	b.WriteString(body)
	return b.String()
}

func modelThemeCacheKey(m Model) string {
	if m.theme == m.themeRenderTheme && m.themeRenderKey != "" {
		return m.themeRenderKey
	}
	return renderThemeCacheKey(m.theme)
}

func modelRenderCacheKey(m Model) string {
	shellTools := "0"
	if m.shellToolCallsVisible {
		shellTools = "1"
	}
	return modelThemeCacheKey(m) +
		"\x00icons:" + m.terminalIcons.CacheKey() +
		"\x00layout:" + strings.TrimSpace(m.layout) +
		"\x00shell_tools:" + shellTools
}

func renderThemeCacheKey(th *theme.Theme) string {
	if th == nil {
		return "nil"
	}
	var b strings.Builder
	b.Grow(256)
	b.WriteString(th.Name)
	b.WriteString("\x00")
	b.WriteString(th.SyntaxStyle)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Primary)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Secondary)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Surface)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Overlay)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Text)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Subtext)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Error)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Warning)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Success)
	b.WriteString("\x00")
	b.WriteString(th.Palette.Thinking)
	b.WriteString("\x00")
	b.WriteString(th.Tones.BG)
	b.WriteString("\x00")
	b.WriteString(th.Tones.BGAlt)
	b.WriteString("\x00")
	b.WriteString(th.Tones.Panel)
	b.WriteString("\x00")
	b.WriteString(th.Tones.PanelAlt)
	b.WriteString("\x00")
	b.WriteString(th.Tones.Line)
	b.WriteString("\x00")
	b.WriteString(th.Tones.LineStrong)
	b.WriteString("\x00")
	b.WriteString(th.Tones.Soft)
	return b.String()
}
