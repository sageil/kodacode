package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

func TestBuildTimelineItemsIncludesParentAndChildBranchSessions(t *testing.T) {
	state := events.SessionState{
		SessionID:    "session-child",
		TurnOrder:    []string{"turn-1"},
		Turns:        map[string]*events.TurnState{"turn-1": {TurnID: "turn-1", UserText: "try this", Status: events.TurnStatusCompleted, CompletedAtSeq: 2}},
		Branch:       &events.SessionBranchState{ParentSessionID: "session-parent", ParentTurnID: "turn-parent", ParentSequence: 4},
		LastSequence: 2,
	}
	sessions := []app.SessionSummary{
		{ID: "session-parent", Title: "Original"},
		{
			ID:        "session-grandchild",
			Title:     "Follow-up branch",
			UpdatedAt: time.Unix(1710000000, 0).UTC(),
			Branch:    &events.SessionBranchState{ParentSessionID: "session-child", ParentTurnID: "turn-1", ParentSequence: 2},
		},
	}

	items := buildTimelineItems(state, sessions, timelineBuildOptions{})
	if len(items) != 3 {
		t.Fatalf("items = %#v, want parent, turn, and child branch", items)
	}
	if items[0].Kind != timelineItemParentSession || items[0].SessionID != "session-parent" {
		t.Fatalf("parent item = %#v", items[0])
	}
	if items[1].Kind != timelineItemTurn || items[1].TurnID != "turn-1" {
		t.Fatalf("turn item = %#v", items[1])
	}
	if items[2].Kind != timelineItemChildSession || items[2].SessionID != "session-grandchild" {
		t.Fatalf("child item = %#v", items[2])
	}
}

func TestBuildTimelineItemsFiltersSearchAndFoldsBranchSessions(t *testing.T) {
	state := events.SessionState{
		SessionID: "session-root",
		TurnOrder: []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", UserText: "implement cache", Status: events.TurnStatusCompleted, CompletedAtSeq: 2},
			"turn-2": {TurnID: "turn-2", UserText: "broken path", Status: events.TurnStatusFailed, CompletedAtSeq: 4},
		},
	}
	sessions := []app.SessionSummary{
		{
			ID:        "session-child",
			Title:     "Cache branch",
			UpdatedAt: time.Unix(1710000000, 0).UTC(),
			Branch:    &events.SessionBranchState{ParentSessionID: "session-root", ParentTurnID: "turn-1", ParentSequence: 2},
		},
		{
			ID:        "session-grandchild",
			Title:     "Nested branch",
			UpdatedAt: time.Unix(1710000100, 0).UTC(),
			Branch:    &events.SessionBranchState{ParentSessionID: "session-child", ParentTurnID: "turn-x", ParentSequence: 3},
		},
	}

	failed := buildTimelineItems(state, sessions, timelineBuildOptions{Filter: timelineFilterFailed})
	if len(failed) != 1 || failed[0].TurnID != "turn-2" {
		t.Fatalf("failed items = %#v", failed)
	}

	searched := buildTimelineItems(state, sessions, timelineBuildOptions{Search: "nested"})
	if len(searched) != 1 || searched[0].SessionID != "session-grandchild" {
		t.Fatalf("searched items = %#v", searched)
	}

	searchedFolded := buildTimelineItems(state, sessions, timelineBuildOptions{
		Search: "nested",
		Folded: map[string]bool{"session-child": true},
	})
	if len(searchedFolded) != 1 || searchedFolded[0].SessionID != "session-grandchild" {
		t.Fatalf("searched folded items = %#v", searchedFolded)
	}

	folded := buildTimelineItems(state, sessions, timelineBuildOptions{Folded: map[string]bool{"session-child": true}})
	for _, item := range folded {
		if item.SessionID == "session-grandchild" {
			t.Fatalf("folded items include grandchild: %#v", folded)
		}
	}
}

func TestBuildTimelineItemsFoldsTurnBranchSessions(t *testing.T) {
	state := events.SessionState{
		SessionID: "session-root",
		TurnOrder: []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", UserText: "choose path", Status: events.TurnStatusCompleted, CompletedAtSeq: 2},
			"turn-2": {TurnID: "turn-2", UserText: "continue", Status: events.TurnStatusCompleted, CompletedAtSeq: 4},
		},
	}
	sessions := []app.SessionSummary{
		{
			ID:     "session-child",
			Title:  "Alternate path",
			Branch: &events.SessionBranchState{ParentSessionID: "session-root", ParentTurnID: "turn-1", ParentSequence: 2},
		},
	}

	folded := buildTimelineItems(state, sessions, timelineBuildOptions{Folded: map[string]bool{"turn:turn-1": true}})
	for _, item := range folded {
		if item.SessionID == "session-child" {
			t.Fatalf("folded turn branch items include child: %#v", folded)
		}
	}
	if len(folded) == 0 || !folded[0].Folded {
		t.Fatalf("first item = %#v, want folded branch-point turn", folded)
	}
	if label := timelineItemDisplayLabel(folded[0]); !strings.HasPrefix(label, "▸ 1. choose path") {
		t.Fatalf("folded turn label = %q", label)
	}

	searched := buildTimelineItems(state, sessions, timelineBuildOptions{
		Search: "alternate",
		Folded: map[string]bool{"turn:turn-1": true},
	})
	if len(searched) != 1 || searched[0].SessionID != "session-child" {
		t.Fatalf("searched folded branch items = %#v", searched)
	}
}

func TestTimelineBranchRowsUseTreeGlyphsAndPreview(t *testing.T) {
	state := events.SessionState{
		SessionID: "session-root",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", UserText: "choose path", Status: events.TurnStatusCompleted, CompletedAtSeq: 2},
		},
	}
	sessions := []app.SessionSummary{
		{
			ID:        "session-child-a",
			Title:     "Retry with cache",
			Status:    events.TurnStatusCompleted,
			UpdatedAt: time.Now().Add(-2 * time.Hour),
			Branch:    &events.SessionBranchState{ParentSessionID: "session-root", ParentTurnID: "turn-1", ParentSequence: 2},
		},
		{
			ID:        "session-child-b",
			Title:     "Minimal patch",
			Status:    events.TurnStatusFailed,
			UpdatedAt: time.Now().Add(-1 * time.Hour),
			Branch:    &events.SessionBranchState{ParentSessionID: "session-root", ParentTurnID: "turn-1", ParentSequence: 2},
		},
		{
			ID:        "session-grandchild",
			Title:     "Nested fix",
			Status:    events.TurnStatusRunning,
			UpdatedAt: time.Now().Add(-30 * time.Minute),
			Branch:    &events.SessionBranchState{ParentSessionID: "session-child-a", ParentTurnID: "turn-x", ParentSequence: 3},
		},
	}

	items := buildTimelineItems(state, sessions, timelineBuildOptions{})
	var firstChild, grandchild timelineItem
	for _, item := range items {
		switch item.SessionID {
		case "session-child-a":
			firstChild = item
		case "session-grandchild":
			grandchild = item
		}
	}
	if label := timelineItemDisplayLabel(firstChild); !strings.Contains(label, "├─ ▾ Retry with cache") {
		t.Fatalf("first child label = %q", label)
	}
	if label := timelineItemDisplayLabel(grandchild); !strings.Contains(label, "│  └─ Nested fix") {
		t.Fatalf("grandchild label = %q", label)
	}
	for _, needle := range []string{"branch from turn", "session", "1 child branch", "completed"} {
		if !strings.Contains(firstChild.Preview, needle) {
			t.Fatalf("first child preview missing %q: %q", needle, firstChild.Preview)
		}
	}
	if !strings.Contains(firstChild.Meta, "1 child") {
		t.Fatalf("first child meta = %q", firstChild.Meta)
	}
}

func TestTimelineBranchRowsShowStoredSummaryPreview(t *testing.T) {
	state := events.SessionState{
		SessionID: "session-root",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", UserText: "choose path", Status: events.TurnStatusCompleted, CompletedAtSeq: 2},
		},
	}
	sessions := []app.SessionSummary{
		{
			ID:        "session-child",
			Title:     "Retry with cache",
			Status:    events.TurnStatusCompleted,
			UpdatedAt: time.Now(),
			Branch:    &events.SessionBranchState{ParentSessionID: "session-root", ParentTurnID: "turn-1", ParentSequence: 2},
			BranchSummary: &app.BranchSummary{
				Summary: "Changed cache invalidation and left one retry follow-up.",
			},
		},
	}

	items := buildTimelineItems(state, sessions, timelineBuildOptions{Search: "retry follow-up"})
	if len(items) != 1 {
		t.Fatalf("items = %#v, want matching summary row", items)
	}
	if got := items[0].Preview; got != "Changed cache invalidation and left one retry follow-up." {
		t.Fatalf("preview = %q", got)
	}
	if !strings.Contains(items[0].Meta, "summary") {
		t.Fatalf("meta = %q, want summary marker", items[0].Meta)
	}
}

func TestTimelineDialogSummarizesBranchSession(t *testing.T) {
	state := events.SessionState{
		SessionID: "session-root",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", UserText: "try branch", Status: events.TurnStatusCompleted, CompletedAtSeq: 2},
		},
	}
	sessions := []app.SessionSummary{
		{
			ID:     "session-child",
			Title:  "Old branch",
			Branch: &events.SessionBranchState{ParentSessionID: "session-root", ParentTurnID: "turn-1", ParentSequence: 2},
		},
	}
	dialog := newTimelineDialog(state, sessions, nil)
	dialog.cursor = 1

	_, closeCmd := dialog.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	if closeCmd == nil {
		t.Fatal("closeCmd = nil")
	}
	closed, ok := closeCmd().(dialogClosedMsg)
	if !ok {
		t.Fatalf("closeCmd() = %#v, want dialogClosedMsg", closeCmd())
	}
	result, ok := closed.result.(timelineDialogResult)
	if !ok {
		t.Fatalf("closed.result = %#v, want timelineDialogResult", closed.result)
	}
	if result.SummarySessionID != "session-child" {
		t.Fatalf("summary result = %#v", result)
	}
}

func TestTimelineDialogLabelsBranchSession(t *testing.T) {
	state := events.SessionState{
		SessionID: "session-root",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", UserText: "try branch", Status: events.TurnStatusCompleted, CompletedAtSeq: 2},
		},
	}
	sessions := []app.SessionSummary{
		{
			ID:     "session-child",
			Title:  "Old branch",
			Branch: &events.SessionBranchState{ParentSessionID: "session-root", ParentTurnID: "turn-1", ParentSequence: 2},
		},
	}
	dialog := newTimelineDialog(state, sessions, nil)
	dialog.cursor = 1

	updated, _ := dialog.Update(tea.KeyPressMsg{Text: "e", Code: 'e'})
	next, ok := updated.(*timelineDialog)
	if !ok {
		t.Fatalf("updated dialog = %T, want *timelineDialog", updated)
	}
	next.labelInput.SetValue("Focused branch")
	_, closeCmd := next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if closeCmd == nil {
		t.Fatal("closeCmd = nil")
	}
	closed, ok := closeCmd().(dialogClosedMsg)
	if !ok {
		t.Fatalf("closeCmd() = %#v, want dialogClosedMsg", closeCmd())
	}
	result, ok := closed.result.(timelineDialogResult)
	if !ok {
		t.Fatalf("closed.result = %#v, want timelineDialogResult", closed.result)
	}
	if result.LabelSessionID != "session-child" || result.Label != "Focused branch" {
		t.Fatalf("label result = %#v", result)
	}
}
