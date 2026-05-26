package tui

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type codeBlockSection struct {
	header string
	path   string
	lines  []string
}

func syntaxHighlightLexer(language, sample string) chroma.Lexer {
	if name := strings.TrimSpace(language); name != "" {
		if lexer := lexers.Get(name); lexer != nil {
			return chroma.Coalesce(lexer)
		}
	}
	if lexer := lexers.Analyse(sample); lexer != nil {
		return chroma.Coalesce(lexer)
	}
	return chroma.Coalesce(lexers.Fallback)
}

func splitMultiFileCodeSections(lines []string) ([]codeBlockSection, bool) {
	sections := make([]codeBlockSection, 0, 4)
	current := codeBlockSection{}
	headerCount := 0

	flush := func() {
		if current.header == "" && len(current.lines) == 0 {
			return
		}
		sections = append(sections, current)
		current = codeBlockSection{}
	}

	for _, line := range lines {
		if path, ok := parseMultiFileCodeHeader(line); ok {
			flush()
			current.header = line
			current.path = path
			headerCount++
			continue
		}
		current.lines = append(current.lines, line)
	}
	flush()

	if headerCount < 2 {
		return nil, false
	}
	return sections, true
}

func parseMultiFileCodeHeader(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "=== ") || !strings.HasSuffix(trimmed, " ===") {
		return "", false
	}
	path := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "=== "), " ==="))
	if path == "" {
		return "", false
	}
	return path, true
}

func syntaxHighlightStyle(th *theme.Theme) *chroma.Style {
	if th != nil {
		if name := strings.TrimSpace(th.SyntaxStyle); name != "" {
			if style, ok := chromastyles.Registry[name]; ok && style != nil {
				return style
			}
		}
	}
	return buildThemeSyntaxStyle(th)
}

func buildThemeSyntaxStyle(th *theme.Theme) *chroma.Style {
	builder := chroma.NewStyleBuilder("kodacode-theme")
	builder.Add(chroma.Background, colorFor(th, "text", "#ecf0ff"))
	builder.Add(chroma.Text, colorFor(th, "soft", softTextColor))
	builder.Add(chroma.Comment, "italic "+colorFor(th, "subtext", "#9da8ca"))
	builder.Add(chroma.CommentPreproc, colorFor(th, "warning", "#ffd28f"))
	builder.Add(chroma.Keyword, "bold "+colorFor(th, "primary", "#7cc7ff"))
	builder.Add(chroma.KeywordConstant, "bold "+colorFor(th, "warning", "#ffd28f"))
	builder.Add(chroma.KeywordType, colorFor(th, "warning", "#ffd28f"))
	builder.Add(chroma.Operator, colorFor(th, "primary", "#7cc7ff"))
	builder.Add(chroma.Punctuation, colorFor(th, "text", "#ecf0ff"))
	builder.Add(chroma.Name, colorFor(th, "text", "#ecf0ff"))
	builder.Add(chroma.NameBuiltin, colorFor(th, "secondary", "#7dcfff"))
	builder.Add(chroma.NameFunction, colorFor(th, "secondary", "#7dcfff"))
	builder.Add(chroma.NameClass, colorFor(th, "warning", "#ffd28f"))
	builder.Add(chroma.NameNamespace, colorFor(th, "secondary", "#7dcfff"))
	builder.Add(chroma.NameTag, colorFor(th, "primary", "#7cc7ff"))
	builder.Add(chroma.NameAttribute, colorFor(th, "secondary", "#7dcfff"))
	builder.Add(chroma.LiteralString, colorFor(th, "success", "#90e5b4"))
	builder.Add(chroma.LiteralStringEscape, colorFor(th, "warning", "#ffd28f"))
	builder.Add(chroma.LiteralNumber, colorFor(th, "secondary", "#7dcfff"))
	builder.Add(chroma.LiteralDate, colorFor(th, "secondary", "#7dcfff"))
	builder.Add(chroma.GenericDeleted, colorFor(th, "error", "#ff9aa6"))
	builder.Add(chroma.GenericInserted, colorFor(th, "success", "#90e5b4"))
	builder.Add(chroma.GenericError, "bold "+colorFor(th, "error", "#ff9aa6"))
	builder.Add(chroma.GenericHeading, "bold "+colorFor(th, "primary", "#7cc7ff"))
	builder.Add(chroma.GenericSubheading, "bold "+colorFor(th, "secondary", "#7dcfff"))
	builder.Add(chroma.GenericStrong, "bold "+colorFor(th, "text", "#ecf0ff"))
	builder.Add(chroma.GenericEmph, "italic "+colorFor(th, "text", "#ecf0ff"))
	style, err := builder.Build()
	if err != nil {
		return chromastyles.Fallback
	}
	return style
}

func syntaxHighlightCodeLine(line string, lexer chroma.Lexer, style *chroma.Style, fg, bg string) string {
	if strings.TrimSpace(line) == "" {
		return ""
	}
	if lexer == nil {
		lexer = chroma.Coalesce(lexers.Fallback)
	}
	if style == nil {
		style = buildThemeSyntaxStyle(nil)
	}
	iterator, err := lexer.Tokenise(nil, line)
	if err != nil {
		return applyDefaultCodeANSIStyle(line, fg, bg)
	}

	var buf bytes.Buffer
	if err := formatters.TTY16m.Format(&buf, style, iterator); err != nil {
		return applyDefaultCodeANSIStyle(line, fg, bg)
	}
	return applyDefaultCodeANSIStyle(buf.String(), fg, bg)
}

func applyDefaultCodeANSIStyle(rendered, fg, bg string) string {
	base := ansiStylePrefix(fg, bg)
	if base == "" {
		return rendered
	}
	rendered = strings.ReplaceAll(rendered, "\x1b[0m", "\x1b[0m"+base)
	return base + rendered + "\x1b[0m"
}

func ansiStylePrefix(fg, bg string) string {
	var b strings.Builder
	if strings.TrimSpace(fg) != "" {
		r, g, bl := parseHex(fg)
		_, _ = fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm", r, g, bl)
	}
	if strings.TrimSpace(bg) != "" {
		r, g, bl := parseHex(bg)
		_, _ = fmt.Fprintf(&b, "\x1b[48;2;%d;%d;%dm", r, g, bl)
	}
	return b.String()
}
