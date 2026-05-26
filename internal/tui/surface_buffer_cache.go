package tui

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/cellbuf"
)

type surfaceBufferCache struct {
	name    string
	key     string
	content string
	width   int
	height  int
	buffer  *cellbuf.Buffer
	rows    []string
}

func newSurfaceBufferCache(name ...string) *surfaceBufferCache {
	cacheName := ""
	if len(name) > 0 {
		cacheName = name[0]
	}
	return &surfaceBufferCache{name: cacheName}
}

func (c *surfaceBufferCache) surfaceFor(content string, width, height int) (*cellbuf.Buffer, []string) {
	width = max(width, 1)
	height = max(height, 1)
	if c != nil && c.buffer != nil && c.width == width && c.height == height && c.content == content {
		traceRenderCacheLookupStringKey(c.name, c.key, true, len(content), 0)
		return c.buffer, c.rows
	}
	key := cachedSurfaceBufferKey(content, width, height)
	if c != nil && c.key == key && c.buffer != nil {
		traceRenderCacheLookupStringKey(c.name, key, true, len(content), 0)
		c.content = content
		c.width = width
		c.height = height
		return c.buffer, c.rows
	}

	var startedAt time.Time
	if renderTraceEnabled() {
		startedAt = time.Now()
	}
	buf, rows := newRenderedSurface(content, width, height)
	if c != nil {
		var renderDuration time.Duration
		if !startedAt.IsZero() {
			renderDuration = time.Since(startedAt)
		}
		traceRenderCacheLookupStringKey(c.name, key, false, len(content), renderDuration)
	}
	if c == nil {
		return buf, rows
	}
	c.key = key
	c.content = content
	c.width = width
	c.height = height
	c.buffer = buf
	c.rows = rows
	return c.buffer, c.rows
}

func (c *surfaceBufferCache) cloneFor(content string, width, height int) *cellbuf.Buffer {
	base, _ := c.surfaceFor(content, width, height)
	if base == nil {
		return nil
	}
	return cloneCellBuffer(base)
}

var renderedSurfaceBufferCache = struct {
	mu    sync.Mutex
	cache *surfaceBufferCache
}{
	cache: newSurfaceBufferCache("global_surface_buffer"),
}

func cachedSurfaceBase(content string, width, height int) (*cellbuf.Buffer, []string) {
	renderedSurfaceBufferCache.mu.Lock()
	defer renderedSurfaceBufferCache.mu.Unlock()
	return renderedSurfaceBufferCache.cache.surfaceFor(content, width, height)
}

func cachedSurfaceBaseBuffer(content string, width, height int) *cellbuf.Buffer {
	base, _ := cachedSurfaceBase(content, width, height)
	return base
}

// cachedSurfaceBuffer memoizes the decoded form of the last rendered surface so
// repeated callers can reuse the parsed cell buffer instead of reparsing ANSI
// every time. It is bounded to a single entry and returns a clone because
// callers mutate the surface after retrieval.
func cachedSurfaceBuffer(content string, width, height int) *cellbuf.Buffer {
	renderedSurfaceBufferCache.mu.Lock()
	defer renderedSurfaceBufferCache.mu.Unlock()
	return renderedSurfaceBufferCache.cache.cloneFor(content, width, height)
}

func cachedSurfaceBufferKey(content string, width, height int) string {
	var b strings.Builder
	b.Grow(len(content) + 32)
	b.WriteString(strconv.Itoa(width))
	b.WriteString("\x00")
	b.WriteString(strconv.Itoa(height))
	b.WriteString("\x00")
	b.WriteString(content)
	return b.String()
}

func newRenderedSurfaceBuffer(content string, width, height int) *cellbuf.Buffer {
	buf := cellbuf.NewBuffer(max(width, 1), max(height, 1))
	cellbuf.SetContent(buf, placeBlock(max(width, 1), max(height, 1), "", content))
	return buf
}

func newRenderedSurface(content string, width, height int) (*cellbuf.Buffer, []string) {
	buf := newRenderedSurfaceBuffer(content, width, height)
	return buf, renderCellBufferRows(buf)
}

func cloneCellBuffer(src *cellbuf.Buffer) *cellbuf.Buffer {
	if src == nil {
		return nil
	}

	lines := make([]cellbuf.Line, len(src.Lines))
	for y, line := range src.Lines {
		if len(line) == 0 {
			continue
		}
		clonedLine := make(cellbuf.Line, len(line))
		for x, cell := range line {
			if cell != nil {
				clonedLine[x] = cell.Clone()
			}
		}
		lines[y] = clonedLine
	}

	return &cellbuf.Buffer{Lines: lines}
}
