package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/cellbuf"
)

func TestRenderedOverlayCacheReusesSingleEntry(t *testing.T) {
	cache := newRenderedOverlayCache()
	calls := 0

	firstRendered, firstCursor := cache.frameFor(11, func() (string, *tea.Cursor) {
		calls++
		return "first", tea.NewCursor(2, 3)
	})
	secondRendered, secondCursor := cache.frameFor(11, func() (string, *tea.Cursor) {
		calls++
		return "second", tea.NewCursor(4, 5)
	})

	if calls != 1 {
		t.Fatalf("render calls = %d, want 1 cache hit", calls)
	}
	if firstRendered != "first" || secondRendered != "first" {
		t.Fatalf("cache returned (%q, %q), want repeated first value", firstRendered, secondRendered)
	}
	if firstCursor == nil || secondCursor == nil || secondCursor.X != 2 || secondCursor.Y != 3 {
		t.Fatalf("cached cursor = %#v, want cloned first cursor", secondCursor)
	}
	if firstCursor == secondCursor {
		t.Fatal("cache returned shared cursor pointer; want clone")
	}
}

func TestComposerOverlayRenderCacheKeyDependsOnBasePopupAndCursor(t *testing.T) {
	baseA := cellbuf.NewBuffer(4, 2)
	baseB := cellbuf.NewBuffer(4, 2)
	cursorA := tea.NewCursor(2, 3)
	cursorB := tea.NewCursor(5, 3)

	base := composerOverlayRenderCacheKey(baseA, "popup", cursorA)
	if base == 0 {
		t.Fatal("composerOverlayRenderCacheKey() returned zero key")
	}
	if got := composerOverlayRenderCacheKey(baseA, "popup", cursorA); got != base {
		t.Fatalf("composerOverlayRenderCacheKey() unstable for identical inputs")
	}
	if got := composerOverlayRenderCacheKey(baseB, "popup", cursorA); got == base {
		t.Fatalf("composerOverlayRenderCacheKey() did not vary with base surface identity")
	}
	if got := composerOverlayRenderCacheKey(baseA, "other", cursorA); got == base {
		t.Fatalf("composerOverlayRenderCacheKey() did not vary with popup content")
	}
	if got := composerOverlayRenderCacheKey(baseA, "popup", cursorB); got == base {
		t.Fatalf("composerOverlayRenderCacheKey() did not vary with cursor position")
	}
}

func TestDialogOverlayRenderCacheKeyDependsOnBaseAreaAndDialogKey(t *testing.T) {
	baseA := cellbuf.NewBuffer(4, 2)
	baseB := cellbuf.NewBuffer(4, 2)
	areaA := dialogRenderArea{x: 1, y: 2, width: 30, height: 12}
	areaB := dialogRenderArea{x: 1, y: 3, width: 30, height: 12}

	base := dialogOverlayRenderCacheKey(baseA, areaA, dialogIDCost, 11)
	if base == 0 {
		t.Fatal("dialogOverlayRenderCacheKey() returned zero key")
	}
	if got := dialogOverlayRenderCacheKey(baseA, areaA, dialogIDCost, 11); got != base {
		t.Fatalf("dialogOverlayRenderCacheKey() unstable for identical inputs")
	}
	if got := dialogOverlayRenderCacheKey(baseB, areaA, dialogIDCost, 11); got == base {
		t.Fatalf("dialogOverlayRenderCacheKey() did not vary with base surface identity")
	}
	if got := dialogOverlayRenderCacheKey(baseA, areaB, dialogIDCost, 11); got == base {
		t.Fatalf("dialogOverlayRenderCacheKey() did not vary with render area")
	}
	if got := dialogOverlayRenderCacheKey(baseA, areaA, dialogIDTrace, 11); got == base {
		t.Fatalf("dialogOverlayRenderCacheKey() did not vary with dialog id")
	}
	if got := dialogOverlayRenderCacheKey(baseA, areaA, dialogIDCost, 22); got == base {
		t.Fatalf("dialogOverlayRenderCacheKey() did not vary with dialog overlay key")
	}
}
