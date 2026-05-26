package tui

import "github.com/charmbracelet/x/cellbuf"

type overlaySurface struct {
	base     *cellbuf.Buffer
	baseRows []string
	width    int
	height   int
	lines    map[int]cellbuf.Line
}

func newOverlaySurface(base *cellbuf.Buffer, baseRows []string) *overlaySurface {
	width := 1
	height := 1
	if base != nil {
		width = max(base.Width(), 1)
		height = max(base.Height(), 1)
	}
	return &overlaySurface{
		base:     base,
		baseRows: baseRows,
		width:    width,
		height:   height,
	}
}

func (s *overlaySurface) Width() int {
	if s == nil {
		return 0
	}
	return max(s.width, 1)
}

func (s *overlaySurface) Height() int {
	if s == nil {
		return 0
	}
	return max(s.height, 1)
}

func (s *overlaySurface) Cell(x, y int) *cellbuf.Cell {
	if s == nil || x < 0 || y < 0 || x >= s.Width() || y >= s.Height() {
		return nil
	}
	if line := s.lines[y]; line != nil && x < len(line) && line[x] != nil {
		return line[x]
	}
	if s.base == nil {
		return nil
	}
	return s.base.Cell(x, y)
}

func (s *overlaySurface) SetCell(x, y int, cell *cellbuf.Cell) bool {
	if s == nil || cell == nil || x < 0 || y < 0 || x >= s.Width() || y >= s.Height() {
		return false
	}
	if s.lines == nil {
		s.lines = make(map[int]cellbuf.Line)
	}
	line := s.lines[y]
	if line == nil {
		line = make(cellbuf.Line, s.Width())
		s.lines[y] = line
	}
	line[x] = cell.Clone()
	return true
}

func (s *overlaySurface) rowDirty(row int) bool {
	if s == nil || row < 0 || s.lines == nil {
		return false
	}
	return s.lines[row] != nil
}

func (s *overlaySurface) baseRow(row int) (string, bool) {
	if s == nil || row < 0 || row >= len(s.baseRows) {
		return "", false
	}
	return s.baseRows[row], true
}
