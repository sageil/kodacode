package tui

import (
	"strings"

	"github.com/charmbracelet/x/cellbuf"
)

func drawSurfaceBlockWithMode(surface dialogSurface, block string, x, y int, transparentBlanks bool) {
	if surface == nil {
		return
	}
	blockWidth := blockWidth(block)
	blockHeight := max(strings.Count(block, "\n")+1, 0)
	if blockWidth <= 0 || blockHeight <= 0 {
		return
	}

	buf := cellbuf.NewBuffer(max(blockWidth, 1), max(blockHeight, 1))
	cellbuf.SetContent(buf, block)
	drawBlockBufferWithMode(surface, buf, x, y, transparentBlanks)
}
