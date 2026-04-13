package tui

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// ansiRenderer is a goldmark NodeRenderer that converts markdown AST nodes
// into ANSI-escaped styled text with syntax-highlighted code blocks.
type ansiRenderer struct {
	themeName  string
	theme      *theme.Theme
	codeBgAnsi string
	preserveSoftBreaks bool
}

func (r *ansiRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindEmphasis, r.renderEmphasis)
	reg.Register(ast.KindCodeSpan, r.renderCodeSpan)
	reg.Register(ast.KindText, r.renderText)
	reg.Register(ast.KindString, r.renderString)
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindDocument, r.renderDocument)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindList, r.renderPassthrough)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindAutoLink, r.renderPassthrough)
	reg.Register(ast.KindImage, r.renderImage)
	// Table (GFM extension)
	reg.Register(east.KindTable, r.renderTable)
	reg.Register(east.KindTableHeader, r.renderPassthrough)
	reg.Register(east.KindTableRow, r.renderPassthrough)
	reg.Register(east.KindTableCell, r.renderPassthrough)
	// Strikethrough (GFM extension)
	reg.Register(east.KindStrikethrough, r.renderStrikethrough)
}

func (r *ansiRenderer) renderStrikethrough(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(ansiStrikethrough)
	} else {
		_, _ = w.WriteString(ansiReset)
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderDocument(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	_ = entering
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderParagraph(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString("\n")
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderPassthrough(w util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderBlockquote(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(ansiBqBar + ansiBqClr)
	} else {
		_, _ = w.WriteString(ansiReset)
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderLink(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(ansiLinkClr)
	} else {
		n := node.(*ast.Link)
		_, _ = w.WriteString(ansiReset)
		if len(n.Destination) > 0 {
			_, _ = w.WriteString(ansiDim + " (" + string(n.Destination) + ")" + ansiReset)
		}
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderImage(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*ast.Image)
		dest := string(n.Destination)
		// Extract just the filename from path or URL.
		name := filepath.Base(dest)
		if name == "." || name == "/" {
			name = dest
		}
		_, _ = w.WriteString(ansiDim + "🖼 [image: " + name + "]" + ansiReset)
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderListItem(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("  • ")
	} else {
		_, _ = w.WriteString("\n")
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderThematicBreak(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("────────────────────\n")
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderText(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Text)
	_, _ = w.Write(n.Segment.Value(source))
	if n.HardLineBreak() {
		_, _ = w.WriteString("\n")
	} else if n.SoftLineBreak() {
		if r.preserveSoftBreaks {
			_, _ = w.WriteString("\n")
		} else {
			_, _ = w.WriteString(" ")
		}
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderString(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.String)
	_, _ = w.Write(n.Value)
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderEmphasis(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Emphasis)
	if n.Level == 2 {
		// **bold**
		if entering {
			_, _ = w.WriteString(ansiBold)
		} else {
			_, _ = w.WriteString(ansiReset)
		}
	} else {
		// *italic*
		if entering {
			_, _ = w.WriteString(ansiItalic)
		} else {
			_, _ = w.WriteString(ansiReset)
		}
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderCodeSpan(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		bg := r.codeBgAnsi
		if bg == "" {
			bg = ansiCodeBg
		}
		_, _ = w.WriteString(bg + " ")
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if t, ok := child.(*ast.Text); ok {
				_, _ = w.Write(t.Segment.Value(source))
			} else if s, ok := child.(*ast.String); ok {
				_, _ = w.Write(s.Value)
			}
		}
		_, _ = w.WriteString(" " + ansiReset)
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderHeading(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(ansiHeadingClr)
	} else {
		_, _ = w.WriteString(ansiReset + "\n")
	}
	return ast.WalkContinue, nil
}

func (r *ansiRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	// Collect code content.
	var code strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		code.Write(line.Value(source))
	}

	// Extract language from the fence info (e.g. ```go).
	lang := ""
	if fcb, ok := node.(*ast.FencedCodeBlock); ok {
		if info := fcb.Info; info != nil {
			lang = string(info.Segment.Value(source))
			// Take first word only (e.g. "go" from "go title=example").
			if first, _, ok := strings.Cut(lang, " "); ok {
				lang = first
			}
		}
	}

	content := code.String()

	// Render language badge above code block when language is specified.
	_, _ = w.WriteString("\n")
	if lang != "" {
		badge := strings.ToUpper(lang[:1]) + lang[1:]
		badgeLine := "── " + badge + " " + strings.Repeat("─", 40)
		_, _ = w.WriteString(ansiDim + badgeLine + ansiReset + "\n")
	}

	// Diff blocks get custom coloring instead of syntax highlighting.
	if lang == "diff" {
		for line := range strings.SplitSeq(strings.TrimRight(content, "\n"), "\n") {
			switch {
			case strings.HasPrefix(line, "+"):
				_, _ = w.WriteString("  " + ansiGreen + line + ansiReset + "\n")
			case strings.HasPrefix(line, "-"):
				_, _ = w.WriteString("  " + ansiRed + line + ansiReset + "\n")
			case strings.HasPrefix(line, "@@"):
				_, _ = w.WriteString("  " + ansiCyan + line + ansiReset + "\n")
			default:
				_, _ = w.WriteString("  " + ansiDim + line + ansiReset + "\n")
			}
		}
		return ast.WalkSkipChildren, nil
	}

	highlighted := ""
	if lang != "" {
		highlighted = syntaxHighlight(content, lang, r.theme)
	}
	if highlighted != "" && highlighted != content {
		hlLines := strings.Split(strings.TrimRight(highlighted, "\n"), "\n")
		gutterWidth := len(fmt.Sprintf("%d", len(hlLines)))
		for i, line := range hlLines {
			if len(hlLines) > 5 {
				num := fmt.Sprintf("%*d", gutterWidth, i+1)
				_, _ = w.WriteString("  " + ansiDim + num + ansiReset + " " + line + "\n")
			} else {
				_, _ = w.WriteString("  " + line + "\n")
			}
		}
	} else {
		codeLines := strings.Split(strings.TrimRight(content, "\n"), "\n")
		gutterWidth := len(fmt.Sprintf("%d", len(codeLines)))
		for i, line := range codeLines {
			if len(codeLines) > 5 {
				num := fmt.Sprintf("%*d", gutterWidth, i+1)
				_, _ = w.WriteString(ansiDim + "  " + num + " " + line + ansiReset + "\n")
			} else {
				_, _ = w.WriteString(ansiDim + "  " + line + ansiReset + "\n")
			}
		}
	}
	return ast.WalkSkipChildren, nil
}

// renderHTMLBlock handles HTML blocks. If the block contains a <table>, it is
// rendered with box-drawing characters; otherwise the HTML tags are stripped
// and the text content is emitted.
func (r *ansiRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	// Collect the raw HTML from the block's lines.
	var raw strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		raw.Write(line.Value(source))
	}
	htmlStr := raw.String()

	if strings.Contains(htmlStr, "<table") || strings.Contains(htmlStr, "<TABLE") {
		rendered := renderHTMLTable(htmlStr)
		_, _ = w.WriteString(rendered)
	} else {
		// Strip tags, emit plain text.
		_, _ = w.WriteString(stripHTMLTags(htmlStr))
	}
	return ast.WalkSkipChildren, nil
}

func (r *ansiRenderer) renderRawHTML(_ util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

// renderTable renders a full markdown table with box-drawing characters.
// It walks the table AST, extracts cell content (with inline styling), computes
// column widths, and renders the table with proper alignment and borders.
func (r *ansiRenderer) renderTable(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	tbl, ok := node.(*east.Table)
	if !ok {
		return ast.WalkContinue, nil
	}

	// Collect all rows. Each row is a slice of cells; each cell has
	// rendered text (with ANSI escapes) and plain-text width.
	type cell struct {
		text  string
		width int
	}
	var headerRow []cell
	var dataRows [][]cell
	alignments := tbl.Alignments

	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []cell
		for c := row.FirstChild(); c != nil; c = c.NextSibling() {
			rendered := r.renderCellContent(source, c)
			cells = append(cells, cell{
				text:  rendered,
				width: ansi.StringWidth(rendered),
			})
		}
		if row.Kind() == east.KindTableHeader {
			headerRow = cells
		} else {
			dataRows = append(dataRows, cells)
		}
	}

	if headerRow == nil {
		return ast.WalkSkipChildren, nil
	}

	// Determine number of columns and compute max width per column.
	numCols := len(headerRow)
	colWidths := make([]int, numCols)
	for i, c := range headerRow {
		if c.width > colWidths[i] {
			colWidths[i] = c.width
		}
	}
	for _, row := range dataRows {
		for i, c := range row {
			if i < numCols && c.width > colWidths[i] {
				colWidths[i] = c.width
			}
		}
	}

	// Cap column widths so the table fits within a reasonable width.
	// Each column uses colWidth + 2 (padding) + 1 (border), plus 1 for the leading border.
	maxColWidth := tableAvailWidth
	if numCols > 0 {
		perCol := max(tableAvailWidth/numCols, 8)
		maxColWidth = perCol
	}
	for i, cw := range colWidths {
		if cw > maxColWidth {
			colWidths[i] = maxColWidth
		}
	}

	// Alignment helper: pad or truncate cell text to colWidth respecting alignment.
	alignCell := func(c cell, col int) string {
		cw := colWidths[col]
		// Truncate if cell content exceeds the capped column width.
		text := c.text
		cellWidth := c.width
		if cellWidth > cw {
			text = ansi.Truncate(text, cw, "…")
			cellWidth = ansi.StringWidth(text)
		}
		pad := cw - cellWidth
		if pad <= 0 {
			return text
		}
		align := east.AlignLeft
		if col < len(alignments) {
			align = alignments[col]
		}
		switch align {
		case east.AlignRight:
			return strings.Repeat(" ", pad) + text
		case east.AlignCenter:
			left := pad / 2
			right := pad - left
			return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
		default: // AlignLeft, AlignNone
			return text + strings.Repeat(" ", pad)
		}
	}

	// Border builders.
	borderLine := func(left, mid, right, fill string) string {
		var sb strings.Builder
		sb.WriteString(ansiDim)
		sb.WriteString(left)
		for i, cw := range colWidths {
			sb.WriteString(strings.Repeat(fill, cw+2)) // 1 padding each side
			if i < numCols-1 {
				sb.WriteString(mid)
			}
		}
		sb.WriteString(right)
		sb.WriteString(ansiReset)
		return sb.String()
	}

	topBorder := borderLine("┌", "┬", "┐", "─")
	headerSep := borderLine("├", "┼", "┤", "─")
	bottomBorder := borderLine("└", "┴", "┘", "─")

	renderRow := func(cells []cell, bold bool) string {
		var sb strings.Builder
		sb.WriteString(ansiDim + "│" + ansiReset)
		for i := range numCols {
			var c cell
			if i < len(cells) {
				c = cells[i]
			}
			content := alignCell(c, i)
			if bold {
				sb.WriteString(" " + ansiBold + content + ansiReset + " ")
			} else {
				sb.WriteString(" " + content + " ")
			}
			sb.WriteString(ansiDim + "│" + ansiReset)
		}
		return sb.String()
	}

	_, _ = w.WriteString("\n")
	_, _ = w.WriteString(topBorder + "\n")
	_, _ = w.WriteString(renderRow(headerRow, true) + "\n")
	_, _ = w.WriteString(headerSep + "\n")
	for _, row := range dataRows {
		_, _ = w.WriteString(renderRow(row, false) + "\n")
	}
	_, _ = w.WriteString(bottomBorder + "\n")

	return ast.WalkSkipChildren, nil
}

func (r *ansiRenderer) renderCellContent(source []byte, cellNode ast.Node) string {
	var sb strings.Builder
	for child := cellNode.FirstChild(); child != nil; child = child.NextSibling() {
		r.renderInlineNode(&sb, source, child)
	}
	return sb.String()
}

func (r *ansiRenderer) renderInlineNode(sb *strings.Builder, source []byte, node ast.Node) {
	switch n := node.(type) {
	case *ast.Text:
		sb.Write(n.Segment.Value(source))
	case *ast.String:
		sb.Write(n.Value)
	case *ast.Emphasis:
		if n.Level == 2 {
			sb.WriteString(ansiBold)
		} else {
			sb.WriteString(ansiItalic)
		}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			r.renderInlineNode(sb, source, child)
		}
		sb.WriteString(ansiReset)
	case *ast.CodeSpan:
		bg := r.codeBgAnsi
		if bg == "" {
			bg = ansiCodeBg
		}
		sb.WriteString(bg + " ")
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			if t, ok := child.(*ast.Text); ok {
				sb.Write(t.Segment.Value(source))
			} else if s, ok := child.(*ast.String); ok {
				sb.Write(s.Value)
			}
		}
		sb.WriteString(" " + ansiReset)
	case *ast.Link:
		sb.WriteString(ansiLinkClr)
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			r.renderInlineNode(sb, source, child)
		}
		sb.WriteString(ansiReset)
		if len(n.Destination) > 0 {
			sb.WriteString(ansiDim + " (" + string(n.Destination) + ")" + ansiReset)
		}
	default:
		// For any other inline node, recurse into children.
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			r.renderInlineNode(sb, source, child)
		}
	}
}

// renderMarkdown converts markdown src to ANSI-styled text using goldmark.
// themeName is used to select a matching chroma style for code blocks.
// On any error it returns the raw src unchanged.
func renderMarkdown(src string, th *theme.Theme, themeName, codeBgAnsi string) string {
	return renderMarkdownWithOptions(src, th, themeName, codeBgAnsi, false)
}

func renderMarkdownPreserveSoftBreaks(src string, th *theme.Theme, themeName, codeBgAnsi string) string {
	return renderMarkdownWithOptions(src, th, themeName, codeBgAnsi, true)
}

func renderMarkdownWithOptions(src string, th *theme.Theme, themeName, codeBgAnsi string, preserveSoftBreaks bool) string {
	// Pre-process: replace HTML tables with rendered box-drawing tables
	// before goldmark sees them, since goldmark splits inline HTML across
	// many nodes making table detection impossible.
	src = htmlTableRe.ReplaceAllStringFunc(src, renderHTMLTable)

	md := goldmark.New(
		// Register only the table parser (ParagraphTransformer + ASTTransformer),
		// Do not use the built-in HTML table renderer. ansiRenderer handles tables.
		// Also register the strikethrough inline parser for ~~text~~ syntax.
		goldmark.WithParserOptions(
			parser.WithParagraphTransformers(
				util.Prioritized(extension.NewTableParagraphTransformer(), 200),
			),
			parser.WithASTTransformers(
				util.Prioritized(extension.NewTableASTTransformer(), 200),
			),
			parser.WithInlineParsers(
				util.Prioritized(extension.NewStrikethroughParser(), 500),
			),
		),
		goldmark.WithRenderer(
			renderer.NewRenderer(
				renderer.WithNodeRenderers(
					util.Prioritized(&ansiRenderer{
						themeName:          themeName,
						theme:              th,
						codeBgAnsi:         codeBgAnsi,
						preserveSoftBreaks: preserveSoftBreaks,
					}, 1000),
				),
			),
		),
	)
	reader := text.NewReader([]byte(src))
	doc := md.Parser().Parse(reader)
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, []byte(src), doc); err != nil {
		return src
	}
	return buf.String()
}
