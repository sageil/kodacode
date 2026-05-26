package tui

import "sync"

type transcriptRenderLRU struct {
	cap      int
	maxBytes int
	bytes    int
	entries  map[string]*transcriptRenderLRUEntry
	head     *transcriptRenderLRUEntry
	tail     *transcriptRenderLRUEntry
}

type transcriptRenderLRUEntry struct {
	key        string
	value      transcriptRender
	size       int
	prev, next *transcriptRenderLRUEntry
}

var turnTranscriptChunkCache = struct {
	mu    sync.Mutex
	cache *transcriptRenderLRU
}{
	cache: newTranscriptRenderLRU(64, 8<<20),
}
