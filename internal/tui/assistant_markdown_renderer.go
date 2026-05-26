package tui

func renderAssistantMarkdownBlock(m Model, block string, width int, bg string) []string {
	return renderAssistantMarkdownBlockWithStreamKey(m, block, width, bg, "")
}

func renderMarkdownBlockOnSurface(m Model, block string, width int, bg string) []string {
	return renderMarkdownBlockOnSurfaceWithStreamKey(m, block, width, bg, "")
}

func renderAssistantMarkdownBlockWithStreamKey(m Model, block string, width int, bg string, streamKey string) []string {
	return renderMarkdownBlockOnSurfaceWithStreamKey(m, block, width, bg, streamKey)
}

func renderMarkdownBlockOnSurfaceWithStreamKey(m Model, block string, width int, bg string, streamKey string) []string {
	return renderMarkdownLinesOnSurfaceWithStreamKey(m, block, width, bg, streamKey)
}

func renderMarkdownBlockOnSurfaceWithStreamCache(m Model, block string, width int, bg string, streamCache *streamingMarkdownSurfaceCache, streamKey string) []string {
	return renderMarkdownLinesOnSurfaceWithStreamCache(m, block, width, bg, streamCache, streamKey)
}
