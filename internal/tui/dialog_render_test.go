package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

func renderTestDialogContent(dialog dialogModel) string {
	if dialog == nil {
		return ""
	}
	width, height := testDialogFrame(dialog)
	width = max(width, 1)
	height = max(height, 1)

	const sentinelBG = "#010203"
	base := cellbuf.NewBuffer(width, height)
	baseContent := placeBlock(width, height, sentinelBG, "")
	cellbuf.SetContent(base, baseContent)

	buf := cellbuf.NewBuffer(width, height)
	cellbuf.SetContent(buf, baseContent)
	area := dialogRenderArea{width: width, height: height}
	dialog.SetFrame(width, height)
	_ = dialog.Draw(buf, area)

	return cropTestDialogBuffer(buf, base)
}

func renderTestDialogContentPlain(dialog dialogModel) string {
	if dialog == nil {
		return ""
	}
	width, height := testDialogFrame(dialog)
	width = max(width, 1)
	height = max(height, 1)

	rendered, _ := renderDialogOnSurface("", dialog, dialogRenderArea{width: width, height: height}, width, height)
	return strings.Trim(ansi.Strip(rendered), "\n")
}

func testDialogFrame(dialog dialogModel) (int, int) {
	switch d := dialog.(type) {
	case *commandPaletteDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 240, 80
	case *connectDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 160, 60
	case *themeDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 120, 40
	case *skillsDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 160, 60
	case *reasoningVariantDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 120, 40
	case *traceDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 160, 60
	case *costDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 160, 60
	case *trustDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 160, 60
	case *toolDetailDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 240, 80
	case *shellToolsDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 120, 40
	case *taskDetailDialog:
		if d.frameWidth > 0 && d.frameHeight > 0 {
			return d.frameWidth, d.frameHeight
		}
		return 240, 80
	default:
		return 80, 24
	}
}

func cropTestDialogBuffer(buf *cellbuf.Buffer, base *cellbuf.Buffer) string {
	if buf == nil || buf.Width() <= 0 || buf.Height() <= 0 {
		return ""
	}

	minX := buf.Width()
	minY := buf.Height()
	maxX := -1
	maxY := -1
	for y := 0; y < buf.Height(); y++ {
		for x := 0; x < buf.Width(); x++ {
			cell := buf.Cell(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			if base != nil {
				baseCell := base.Cell(x, y)
				if baseCell != nil && cell.Equal(baseCell) {
					continue
				}
			}
			minX = min(minX, x)
			minY = min(minY, y)
			maxX = max(maxX, x)
			maxY = max(maxY, y)
		}
	}

	if maxX < minX || maxY < minY {
		return ""
	}

	cropped := cellbuf.NewBuffer(maxX-minX+1, maxY-minY+1)
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			cell := buf.Cell(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			cloned := *cell
			_ = cropped.SetCell(x-minX, y-minY, &cloned)
		}
	}

	return strings.TrimRight(renderCellBuffer(cropped), "\n")
}
