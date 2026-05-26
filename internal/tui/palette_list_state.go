package tui

type paletteListState struct {
	focusIndex int
	cursor     int
	offset     int
	maxVisible int
}

func newPaletteListState(maxVisible int) paletteListState {
	if maxVisible <= 0 {
		maxVisible = commandPaletteMaxVisible
	}
	return paletteListState{maxVisible: maxVisible}
}

func (s *paletteListState) visibleLimit() int {
	if s == nil || s.maxVisible <= 0 {
		return commandPaletteMaxVisible
	}
	return s.maxVisible
}

func (s *paletteListState) clampCursor(total int) {
	if s == nil {
		return
	}
	if total <= 0 {
		s.cursor = 0
		return
	}
	s.cursor = min(s.cursor, total-1)
	if s.cursor < 0 {
		s.cursor = 0
	}
}

func (s *paletteListState) resetWindow(total int) {
	if s == nil {
		return
	}
	s.clampCursor(total)
	s.offset = 0
}

func (s *paletteListState) moveCursor(delta, total int) bool {
	if s == nil || total <= 0 {
		return false
	}
	old := s.cursor
	s.cursor = min(max(s.cursor+delta, 0), total-1)
	s.ensureVisible(total)
	return s.cursor != old
}

func (s *paletteListState) ensureVisible(total int) {
	if s == nil {
		return
	}
	limit := s.visibleLimit()
	s.clampCursor(total)
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+limit {
		s.offset = s.cursor - limit + 1
	}
	if total <= limit {
		s.offset = 0
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

func (s *paletteListState) focusedButtonIndex(nonButtonCount, buttonCount int) int {
	if s == nil || buttonCount <= 0 {
		return -1
	}
	if s.focusIndex < nonButtonCount {
		return -1
	}
	buttonIdx := s.focusIndex - nonButtonCount
	if buttonIdx >= buttonCount {
		return -1
	}
	return buttonIdx
}

func (s *paletteListState) moveFocus(delta, total int) bool {
	if s == nil || total <= 0 {
		return false
	}
	old := s.focusIndex
	s.focusIndex += delta
	if s.focusIndex < 0 {
		s.focusIndex = total - 1
	} else if s.focusIndex >= total {
		s.focusIndex = 0
	}
	return s.focusIndex != old
}

func (s *paletteListState) visibleRange(total int) (int, int) {
	if s == nil || total <= 0 {
		return 0, 0
	}
	limit := s.visibleLimit()
	if total <= limit {
		return 0, total
	}
	start := min(max(s.offset, 0), total)
	end := min(start+limit, total)
	return start, end
}
