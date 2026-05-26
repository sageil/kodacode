package tui

import tea "charm.land/bubbletea/v2"

type renderedOverlayCache struct {
	key      uint64
	valid    bool
	rendered string
	cursor   *tea.Cursor
}

func newRenderedOverlayCache() *renderedOverlayCache {
	return &renderedOverlayCache{}
}

func (c *renderedOverlayCache) frameFor(key uint64, render func() (string, *tea.Cursor)) (string, *tea.Cursor) {
	if c != nil && c.valid && c.key == key {
		return c.rendered, cloneTeaCursor(c.cursor)
	}
	if render == nil {
		return "", nil
	}
	rendered, cursor := render()
	if c != nil {
		c.key = key
		c.valid = true
		c.rendered = rendered
		c.cursor = cloneTeaCursor(cursor)
	}
	return rendered, cloneTeaCursor(cursor)
}

func cloneTeaCursor(cursor *tea.Cursor) *tea.Cursor {
	if cursor == nil {
		return nil
	}
	cloned := *cursor
	return &cloned
}
