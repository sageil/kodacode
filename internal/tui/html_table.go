package tui

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/charmbracelet/x/ansi"
)

var htmlTableRe = regexp.MustCompile(`(?is)<table[\s>].*?</table>`)

func stripHTMLTags(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}
	var sb strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(doc)
	return sb.String()
}

func renderHTMLTable(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}

	type cell struct {
		text     string
		width    int
		isHeader bool
	}

	var rows [][]cell

	// Recursively find <table> and extract rows.
	var walkTable func(*html.Node)
	var walkRow func(*html.Node) []cell

	textContent := func(n *html.Node) string {
		var sb strings.Builder
		var extract func(*html.Node)
		extract = func(n *html.Node) {
			if n.Type == html.TextNode {
				sb.WriteString(strings.TrimSpace(n.Data))
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				extract(c)
			}
		}
		extract(n)
		return sb.String()
	}

	walkRow = func(tr *html.Node) []cell {
		var cells []cell
		for td := tr.FirstChild; td != nil; td = td.NextSibling {
			if td.Type != html.ElementNode {
				continue
			}
			if td.Data == "td" || td.Data == "th" {
				t := textContent(td)
				cells = append(cells, cell{
					text:     t,
					width:    ansi.StringWidth(t),
					isHeader: td.Data == "th",
				})
			}
		}
		return cells
	}

	walkTable = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			if r := walkRow(n); len(r) > 0 {
				rows = append(rows, r)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkTable(c)
		}
	}
	walkTable(doc)

	if len(rows) == 0 {
		return htmlStr
	}

	// Compute column widths.
	numCols := 0
	for _, row := range rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	colWidths := make([]int, numCols)
	for _, row := range rows {
		for i, c := range row {
			if c.width > colWidths[i] {
				colWidths[i] = c.width
			}
		}
	}

	// Cap column widths to prevent overly wide tables.
	htmlMaxColWidth := tableAvailWidth
	if numCols > 0 {
		perCol := max(tableAvailWidth/numCols, 8)
		htmlMaxColWidth = perCol
	}
	for i, cw := range colWidths {
		if cw > htmlMaxColWidth {
			colWidths[i] = htmlMaxColWidth
		}
	}

	// Render with box-drawing characters.
	borderLine := func(left, mid, right, fill string) string {
		var sb strings.Builder
		sb.WriteString(ansiDim)
		sb.WriteString(left)
		for i, cw := range colWidths {
			sb.WriteString(strings.Repeat(fill, cw+2))
			if i < numCols-1 {
				sb.WriteString(mid)
			}
		}
		sb.WriteString(right)
		sb.WriteString(ansiReset)
		return sb.String()
	}

	renderRow := func(cells []cell, bold bool) string {
		var sb strings.Builder
		sb.WriteString(ansiDim + "│" + ansiReset)
		for i := range numCols {
			var c cell
			if i < len(cells) {
				c = cells[i]
			}
			text := c.text
			cellWidth := c.width
			// Truncate if cell content exceeds column width.
			if cellWidth > colWidths[i] {
				text = ansi.Truncate(text, colWidths[i], "…")
				cellWidth = ansi.StringWidth(text)
			}
			pad := max(colWidths[i]-cellWidth, 0)
			content := text + strings.Repeat(" ", pad)
			if bold || c.isHeader {
				sb.WriteString(" " + ansiBold + content + ansiReset + " ")
			} else {
				sb.WriteString(" " + content + " ")
			}
			sb.WriteString(ansiDim + "│" + ansiReset)
		}
		return sb.String()
	}

	topBorder := borderLine("┌", "┬", "┐", "─")
	headerSep := borderLine("├", "┼", "┤", "─")
	bottomBorder := borderLine("└", "┴", "┘", "─")

	var out strings.Builder
	out.WriteString("\n" + topBorder + "\n")

	// Find where headers end and data begins.
	headerEnd := 0
	for i, row := range rows {
		if len(row) > 0 && row[0].isHeader {
			headerEnd = i + 1
		} else {
			break
		}
	}

	for i, row := range rows {
		out.WriteString(renderRow(row, i < headerEnd) + "\n")
		if i == headerEnd-1 && headerEnd > 0 {
			out.WriteString(headerSep + "\n")
		}
	}
	out.WriteString(bottomBorder + "\n")
	return out.String()
}
