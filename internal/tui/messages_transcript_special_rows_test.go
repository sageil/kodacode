package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTranscriptSpecialRowCachePartsVaryByRenderState(t *testing.T) {
	entry := events.TranscriptEntryState{
		Kind:     events.TranscriptEntryReasoning,
		Sequence: 7,
		Text:     "checking context",
	}
	base := newReasoningTranscriptRow("turn-1", entry, 2, 80, false)
	if len(base.cacheParts()) == 0 {
		t.Fatal("special row cache parts empty")
	}

	withText := base
	withText.text = "different"
	if strings.Join(withText.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("special row cache parts did not vary by text")
	}

	withDimmed := base
	withDimmed.dimmed = true
	if strings.Join(withDimmed.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("special row cache parts did not vary by dimmed state")
	}

	withFocus := base
	withFocus.focused = true
	if strings.Join(withFocus.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("special row cache parts did not vary by focus state")
	}
}

func TestTranscriptSpecialRowsRenderSections(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		SessionID: "session-1",
		TurnID:    "turn-1",
	})

	reasoningEntry := events.TranscriptEntryState{
		Kind:     events.TranscriptEntryReasoning,
		Sequence: 1,
		Text:     "I am checking the current state.",
	}
	reasoningRow := newReasoningTranscriptRow("turn-1", reasoningEntry, 0, 80, false)
	reasoningSection, ok := reasoningRow.section(model)
	if !ok {
		t.Fatal("reasoning row section not rendered")
	}
	if reasoningText := ansi.Strip(reasoningSection.content); !strings.Contains(reasoningText, "I am checking the current state.") {
		t.Fatalf("reasoning section missing text:\n%s", reasoningText)
	}

	compactionEntry := events.TranscriptEntryState{
		Kind:     events.TranscriptEntryCompaction,
		Sequence: 2,
		Text:     "## Critical Context\n- Prior work was compacted.",
	}
	compactionRow := newHistoryCompactionTranscriptRow("turn-1", compactionEntry, 1, 80)
	compactionSection, ok := compactionRow.section(model)
	if !ok {
		t.Fatal("compaction row section not rendered")
	}
	if compactionText := ansi.Strip(compactionSection.content); !strings.Contains(compactionText, "Prior work was compacted.") {
		t.Fatalf("compaction section missing summary:\n%s", compactionText)
	}
}

func TestFallbackHistoryCompactionRowUsesEffectiveContinuation(t *testing.T) {
	turn := &events.TurnState{
		TurnID: "turn-1",
		Continuation: &events.HistoryContinuationState{
			RenderedSummary: "## Critical Context\n- Existing context survives.",
		},
	}
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": turn,
		},
	}

	row, ok := newFallbackHistoryCompactionTranscriptRow(state, "turn-1", turn, 80)
	if !ok {
		t.Fatal("fallback compaction row not created")
	}
	if row.kind != transcriptSpecialRowCompaction {
		t.Fatalf("fallback row kind = %q, want compaction", row.kind)
	}
	if !strings.Contains(row.text, "Existing context survives.") {
		t.Fatalf("fallback row text = %q, want continuation summary", row.text)
	}
}
