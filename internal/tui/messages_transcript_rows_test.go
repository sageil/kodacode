package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTranscriptMessageRowCachePartsVaryByRenderState(t *testing.T) {
	entry := events.TranscriptEntryState{
		Kind:     events.TranscriptEntryAssistant,
		Sequence: 42,
		Text:     "hello",
	}
	state := events.SessionState{SessionID: "session-1"}
	base := newAssistantTranscriptMessageRow(state, "turn-1", entry, 3, 80)
	if len(base.cacheParts()) == 0 {
		t.Fatal("message row cache parts empty")
	}

	withText := base
	withText.text = "different"
	if strings.Join(withText.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("message row cache parts did not vary by text")
	}

	withFocus := base
	withFocus.focused = true
	if strings.Join(withFocus.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("message row cache parts did not vary by focus state")
	}

	nextEntry := entry
	nextEntry.Sequence = 43
	withVersion := newAssistantTranscriptMessageRow(state, "turn-1", nextEntry, 3, 80)
	if strings.Join(withVersion.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("message row cache parts did not vary by entry sequence")
	}
}

func TestTranscriptMessageRowsRenderSections(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	state := events.SessionState{SessionID: "session-1"}
	turn := &events.TurnState{TurnID: "turn-1"}

	userEntry := events.TranscriptEntryState{
		Kind:     events.TranscriptEntryUser,
		Sequence: 1,
		Text:     "Review this module",
	}
	userRow := newUserTranscriptMessageRow("turn-1", userEntry, 0, 80)
	userSection, ok := userRow.section(model, turn)
	if !ok {
		t.Fatal("user row section not rendered")
	}
	userText := ansi.Strip(userSection.content)
	if !strings.Contains(userText, "Review this module") {
		t.Fatalf("user section missing text:\n%s", userText)
	}
	if len(userSection.selectionLines) == 0 {
		t.Fatal("user row did not own selection lines")
	}

	assistantEntry := events.TranscriptEntryState{
		Kind:     events.TranscriptEntryAssistant,
		Sequence: 2,
		Text:     "Done.",
	}
	assistantRow := newAssistantTranscriptMessageRow(state, "turn-1", assistantEntry, 1, 80)
	assistantSection, ok := assistantRow.section(model, turn)
	if !ok {
		t.Fatal("assistant row section not rendered")
	}
	assistantText := ansi.Strip(assistantSection.content)
	if !strings.Contains(assistantText, "Done.") {
		t.Fatalf("assistant section missing text:\n%s", assistantText)
	}
	if len(assistantSection.selectionLines) == 0 {
		t.Fatal("assistant row did not own selection lines")
	}
}

func TestDraftTranscriptMessageRowCachePartsVaryByRenderState(t *testing.T) {
	base := newDraftTranscriptMessageRow("turn-1", "draft prompt", 80)
	if len(base.cacheParts()) == 0 {
		t.Fatal("draft row cache parts empty")
	}

	withText := base
	withText.text = "different"
	if strings.Join(withText.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("draft row cache parts did not vary by text")
	}

	withFocus := base
	withFocus.focused = true
	if strings.Join(withFocus.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("draft row cache parts did not vary by focus state")
	}
}

func TestRenderDraftTurnSectionsUsesMessageRow(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	model.userText = "draft prompt"
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1"},
		},
	}

	sections := renderDraftTurnSections(model, state, 80)
	if len(sections) != 1 {
		t.Fatalf("draft sections = %d, want 1", len(sections))
	}
	rendered := ansi.Strip(sections[0].content)
	if !strings.Contains(rendered, "draft prompt") {
		t.Fatalf("draft section missing text:\n%s", rendered)
	}
	if len(sections[0].selectionLines) == 0 {
		t.Fatal("draft section missing row-owned selection lines")
	}

	state.Turns["turn-1"].UserText = "submitted prompt"
	if got := renderDraftTurnSections(model, state, 80); len(got) != 0 {
		t.Fatalf("draft sections with submitted turn = %d, want 0", len(got))
	}
}
