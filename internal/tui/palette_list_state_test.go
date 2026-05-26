package tui

import "testing"

func TestPaletteListStateCursorKeepsVisibleWindow(t *testing.T) {
	state := newPaletteListState(3)

	state.moveCursor(1, 5)
	state.moveCursor(1, 5)
	state.moveCursor(1, 5)

	if state.cursor != 3 {
		t.Fatalf("cursor = %d, want 3", state.cursor)
	}
	if state.offset != 1 {
		t.Fatalf("offset = %d, want 1", state.offset)
	}

	start, end := state.visibleRange(5)
	if start != 1 || end != 4 {
		t.Fatalf("visibleRange = (%d, %d), want (1, 4)", start, end)
	}
}

func TestPaletteListStateFocusWrapsAcrossInputsAndButtons(t *testing.T) {
	state := newPaletteListState(10)

	state.moveFocus(-1, 3)
	if state.focusIndex != 2 {
		t.Fatalf("focusIndex after reverse wrap = %d, want 2", state.focusIndex)
	}
	if button := state.focusedButtonIndex(1, 2); button != 1 {
		t.Fatalf("focusedButtonIndex = %d, want 1", button)
	}

	state.moveFocus(1, 3)
	if state.focusIndex != 0 {
		t.Fatalf("focusIndex after forward wrap = %d, want 0", state.focusIndex)
	}
	if button := state.focusedButtonIndex(1, 2); button != -1 {
		t.Fatalf("focusedButtonIndex at input = %d, want -1", button)
	}
}

func TestPaletteListStateResetWindowClampsCursor(t *testing.T) {
	state := newPaletteListState(4)
	state.cursor = 8
	state.offset = 6

	state.resetWindow(2)

	if state.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", state.cursor)
	}
	if state.offset != 0 {
		t.Fatalf("offset = %d, want 0", state.offset)
	}
}
