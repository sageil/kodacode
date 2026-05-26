package tui

import "time"

type renderedTextCache struct {
	name  string
	key   uint64
	valid bool
	value string
}

func newRenderedTextCache(name ...string) *renderedTextCache {
	cacheName := ""
	if len(name) > 0 {
		cacheName = name[0]
	}
	return &renderedTextCache{name: cacheName}
}

func (c *renderedTextCache) renderedFor(key uint64, render func() string) string {
	if c != nil && c.valid && c.key == key {
		traceRenderCacheLookup(c.name, key, true, len(c.value), 0)
		return c.value
	}
	if render == nil {
		return ""
	}
	var startedAt time.Time
	if renderTraceEnabled() {
		startedAt = time.Now()
	}
	value := render()
	var renderDuration time.Duration
	if !startedAt.IsZero() {
		renderDuration = time.Since(startedAt)
	}
	traceRenderCacheLookup(cacheTraceName(c), key, false, len(value), renderDuration)
	if c != nil {
		c.key = key
		c.valid = true
		c.value = value
	}
	return value
}

func cacheTraceName(c *renderedTextCache) string {
	if c == nil {
		return ""
	}
	return c.name
}
