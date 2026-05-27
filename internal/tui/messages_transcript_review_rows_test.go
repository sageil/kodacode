package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTranscriptReviewRowCachePartsVaryByRenderState(t *testing.T) {
	entry := events.TranscriptEntryState{
		Kind:     events.TranscriptEntryReview,
		Sequence: 9,
	}
	turn := reviewRowTestTurn()
	base := newReviewTranscriptRow("turn-1", turn, entry, 3, 80)
	if len(base.cacheParts()) == 0 {
		t.Fatal("review row cache parts empty")
	}

	withFocus := base
	withFocus.focused = true
	if strings.Join(withFocus.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("review row cache parts did not vary by focus state")
	}

	nextEntry := entry
	nextEntry.Sequence = 10
	withVersion := newReviewTranscriptRow("turn-1", turn, nextEntry, 3, 80)
	if strings.Join(withVersion.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("review row cache parts did not vary by entry sequence")
	}

	changedTurn := reviewRowTestTurn()
	changedTurn.Review.Findings[0].Title = "Different issue"
	withReview := newReviewTranscriptRow("turn-1", changedTurn, entry, 3, 80)
	if strings.Join(withReview.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("review row cache parts did not vary by review content")
	}
}

func TestTranscriptReviewRowRendersSectionAndOwnsState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	entry := events.TranscriptEntryState{
		Kind:     events.TranscriptEntryReview,
		Sequence: 1,
	}
	turn := reviewRowTestTurn()
	row := newReviewTranscriptRow("turn-1", turn, entry, 0, 80)
	turn.Review.Findings[0].Title = "mutated after row construction"

	section, ok := row.section(model)
	if !ok {
		t.Fatal("review row section not rendered")
	}
	rendered := ansi.Strip(section.content)
	if !strings.Contains(rendered, "Security Review") {
		t.Fatalf("review section missing title:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Silent fallback drops review state") {
		t.Fatalf("review row did not render snapshotted finding:\n%s", rendered)
	}
	if strings.Contains(rendered, "mutated after row construction") {
		t.Fatalf("review row read mutated source turn:\n%s", rendered)
	}
	if len(section.selectionLines) == 0 {
		t.Fatal("review row did not own selection lines")
	}
}

func reviewRowTestTurn() *events.TurnState {
	return &events.TurnState{
		TurnID: "turn-1",
		Config: &events.TurnConfigState{
			Model: "openai/gpt-5",
		},
		Review: &events.ReviewState{
			Title: "Security Review",
			Findings: []events.ReviewFindingState{{
				Severity:    "P1",
				Path:        "internal/app/runtime_review.go",
				Line:        57,
				Title:       "Silent fallback drops review state",
				Explanation: "Structured review state is required downstream.",
			}},
			OverallCorrectness: "incorrect",
			OverallSummary:     "The review path needs structured output to be reliable.",
		},
	}
}
