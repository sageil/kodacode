package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestStreamingTranscriptRenderingKeepsCacheBytesBounded(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	restore := swapTranscriptRenderCachesForTest(
		newLRUCache(8, 32<<10),
		newLRUCache(8, 64<<10),
		newTranscriptRenderLRU(8, 96<<10),
	)
	defer restore()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		SessionID: "session-1",
		TurnID:    "turn-1",
	})

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:    "turn-1",
				Status:    events.TurnStatusRunning,
				ToolCalls: map[string]*events.ToolCallState{},
			},
		},
	}
	turn := state.Turns["turn-1"]

	for i := 1; i <= 24; i++ {
		turn.LastUpdatedAtSeq = int64(i)
		turn.StreamingText = strings.Repeat("streaming output keeps growing with each verbose delta\n", i*48)
		rendered := renderTranscriptMessages(model, state, 96)
		if strings.TrimSpace(rendered.content) == "" {
			t.Fatal("renderTranscriptMessages() returned empty content for streaming preview")
		}
	}

	if assistantContentCache.cache.bytes > assistantContentCache.cache.maxBytes {
		t.Fatalf("assistant content cache bytes = %d, want <= %d", assistantContentCache.cache.bytes, assistantContentCache.cache.maxBytes)
	}
	if transcriptRenderCache.cache.bytes > transcriptRenderCache.cache.maxBytes {
		t.Fatalf("transcript render cache bytes = %d, want <= %d", transcriptRenderCache.cache.bytes, transcriptRenderCache.cache.maxBytes)
	}
	if turnTranscriptChunkCache.cache.bytes > turnTranscriptChunkCache.cache.maxBytes {
		t.Fatalf("turn transcript chunk cache bytes = %d, want <= %d", turnTranscriptChunkCache.cache.bytes, turnTranscriptChunkCache.cache.maxBytes)
	}
	if len(assistantContentCache.cache.entries) == 0 {
		t.Fatal("assistant content cache = empty, want bounded retained entries")
	}
	if len(transcriptRenderCache.cache.entries) == 0 {
		t.Fatal("transcript render cache = empty, want bounded retained entries")
	}
	if len(turnTranscriptChunkCache.cache.entries) == 0 {
		t.Fatal("turn transcript chunk cache = empty, want bounded retained entries")
	}
}

func swapTranscriptRenderCachesForTest(assistant, transcript *lruCache, turn *transcriptRenderLRU) func() {
	assistantContentCache.mu.Lock()
	prevAssistant := assistantContentCache.cache
	assistantContentCache.cache = assistant
	assistantContentCache.mu.Unlock()

	transcriptRenderCache.mu.Lock()
	prevTranscript := transcriptRenderCache.cache
	transcriptRenderCache.cache = transcript
	transcriptRenderCache.mu.Unlock()

	turnTranscriptChunkCache.mu.Lock()
	prevTurn := turnTranscriptChunkCache.cache
	turnTranscriptChunkCache.cache = turn
	turnTranscriptChunkCache.mu.Unlock()

	return func() {
		assistantContentCache.mu.Lock()
		assistantContentCache.cache = prevAssistant
		assistantContentCache.mu.Unlock()

		transcriptRenderCache.mu.Lock()
		transcriptRenderCache.cache = prevTranscript
		transcriptRenderCache.mu.Unlock()

		turnTranscriptChunkCache.mu.Lock()
		turnTranscriptChunkCache.cache = prevTurn
		turnTranscriptChunkCache.mu.Unlock()
	}
}
