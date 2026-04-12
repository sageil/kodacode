package tui

import "fmt"

// cachedMarkdown returns renderMarkdown(src, themeName), using a bounded LRU
// cache to avoid re-parsing completed assistant messages on every render cycle.
func (m *Messages) cachedMarkdown(src string) string {
	if w := m.vp.Width(); w > 0 {
		tableAvailWidth = w
	}
	if m.mdCache == nil {
		m.mdCache = newLRUCache(lruDefaultCap)
	}
	key := m.themeName + "\x00" + fmt.Sprintf("%d", tableAvailWidth) + "\x00" + src
	if v, ok := m.mdCache.get(key); ok {
		return v
	}
	v := renderMarkdown(src, m.theme, m.themeName, m.codeBgAnsi)
	m.mdCache.put(key, v)
	return v
}

func (m *Messages) cachedMarkdownPreserveSoftBreaks(src string) string {
	if w := m.vp.Width(); w > 0 {
		tableAvailWidth = w
	}
	if m.mdCache == nil {
		m.mdCache = newLRUCache(lruDefaultCap)
	}
	key := m.themeName + "\x00softbreaks\x00" + fmt.Sprintf("%d", tableAvailWidth) + "\x00" + src
	if v, ok := m.mdCache.get(key); ok {
		return v
	}
	v := renderMarkdownPreserveSoftBreaks(src, m.theme, m.themeName, m.codeBgAnsi)
	m.mdCache.put(key, v)
	return v
}

// cachedHighlight returns syntaxHighlight(content, lang, theme), using a bounded
// LRU cache to avoid re-tokenizing unchanged file content on every render cycle.
func (m *Messages) cachedHighlight(content, language string) string {
	if m.hlCache == nil {
		m.hlCache = newLRUCache(lruDefaultCap)
	}
	key := language + "\x00" + m.themeName + "\x00" + content
	if v, ok := m.hlCache.get(key); ok {
		return v
	}
	v := syntaxHighlight(content, language, m.theme)
	m.hlCache.put(key, v)
	return v
}
