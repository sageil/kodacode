package search

import (
	"os"
	"regexp"
	"strings"
)

type rubySymbolEnricher struct{}

const rubySignatureLineWindow = 2

func (rubySymbolEnricher) Name() string { return "ruby-signatures" }

func (rubySymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "ruby"
}

func (rubySymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectRubyContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "ruby" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = enclosingRubyContainer(containers, out[i].Line)
		}
		if out[i].Signature == "" {
			if sig := extractRubySignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type rubyContainer struct {
	name      string
	startLine int
	endLine   int
}

var (
	rubyContainerDecl = regexp.MustCompile(`^\s*(class|module)\s+([A-Za-z_][\w:]*)(?:\s*<\s*([A-Za-z_][\w:]*))?`)
	rubyMethodDecl    = regexp.MustCompile(`^\s*(?:private\s+|protected\s+|public\s+)?def\s+(?:self\.)?([A-Za-z_][\w!?=]*)`)
)

func detectRubyContainers(lines []string) []rubyContainer {
	type frame struct {
		kind  string
		name  string
		start int
	}

	var (
		containers []rubyContainer
		stack      []frame
	)

	for i, raw := range lines {
		line := strings.TrimSpace(stripRubyComment(raw))
		if line == "" {
			continue
		}
		if match := rubyContainerDecl.FindStringSubmatch(line); len(match) >= 3 {
			stack = append(stack, frame{kind: match[1], name: match[2], start: i + 1})
			continue
		}
		if isRubyBlockOpener(line) {
			stack = append(stack, frame{kind: "block", start: i + 1})
			continue
		}
		if line == "end" && len(stack) > 0 {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if top.kind == "class" || top.kind == "module" {
				containers = append(containers, rubyContainer{
					name:      top.name,
					startLine: top.start,
					endLine:   i + 1,
				})
			}
		}
	}

	for i := range stack {
		if stack[i].kind == "class" || stack[i].kind == "module" {
			containers = append(containers, rubyContainer{
				name:      stack[i].name,
				startLine: stack[i].start,
				endLine:   len(lines),
			})
		}
	}

	return containers
}

func isRubyBlockOpener(line string) bool {
	if strings.HasPrefix(line, "def ") || strings.HasPrefix(line, "if ") || strings.HasPrefix(line, "unless ") ||
		strings.HasPrefix(line, "case ") || strings.HasPrefix(line, "begin") || strings.HasPrefix(line, "while ") ||
		strings.HasPrefix(line, "until ") || strings.HasPrefix(line, "for ") {
		return true
	}
	return strings.HasSuffix(line, " do")
}

func enclosingRubyContainer(containers []rubyContainer, line int) string {
	var (
		name      string
		bestStart int
	)
	for _, c := range containers {
		if line <= c.startLine || line >= c.endLine {
			continue
		}
		if c.startLine > bestStart {
			bestStart = c.startLine
			name = c.name
		}
	}
	return name
}

func extractRubySignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), rubySignatureLineWindow) {
		snippet := gatherRubySnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "package":
			if sig := extractRubyTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractRubyFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractRubyTypeSignature(compact, name string) string {
	match := rubyContainerDecl.FindStringSubmatch(compact)
	if len(match) < 3 || match[2] != name {
		return ""
	}
	return strings.TrimSpace(match[0])
}

func extractRubyFunctionSignature(compact, name string) string {
	match := rubyMethodDecl.FindStringSubmatch(compact)
	if len(match) != 2 || match[1] != name {
		return ""
	}
	if idx := strings.Index(compact, ";"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func gatherRubySnippet(lines []string, startLine int) string {
	var parts []string
	openParens := 0
	for i := startLine - 1; i < len(lines) && i < startLine+4; i++ {
		line := strings.TrimSpace(stripRubyComment(lines[i]))
		if line == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, line)
		openParens += strings.Count(line, "(") - strings.Count(line, ")")
		if openParens <= 0 && (strings.HasSuffix(line, "do") || strings.Contains(line, ";") || !strings.HasSuffix(line, ",")) {
			break
		}
	}
	return strings.Join(parts, " ")
}

func stripRubyComment(line string) string {
	inSingle := false
	inDouble := false
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}
