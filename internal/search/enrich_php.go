package search

import (
	"os"
	"regexp"
	"strings"
)

type phpSymbolEnricher struct{}

const phpSignatureLineWindow = 2

func (phpSymbolEnricher) Name() string { return "php-signatures" }

func (phpSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "php"
}

func (phpSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectPHPContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "php" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = enclosingPHPContainer(containers, out[i].Line)
		}
		if out[i].Signature == "" {
			if sig := extractPHPSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type phpContainer struct {
	name      string
	startLine int
	depth     int
}

var (
	phpContainerDecl = regexp.MustCompile(`^\s*(?:abstract\s+|final\s+)?(class|interface|trait|enum)\s+([A-Za-z_][\w]*)`)
	phpNamedFunction = regexp.MustCompile(`^\s*(?:(?:public|protected|private|static|final|abstract|readonly)\s+)*(?:function)\s*&?\s*([A-Za-z_][\w]*)\s*\(`)
)

func detectPHPContainers(lines []string) []phpContainer {
	var (
		containers []phpContainer
		pending    string
		depth      int
	)
	for i, raw := range lines {
		line := stripPHPLine(raw)
		if match := phpContainerDecl.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 3 {
			pending = match[2]
		}
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		newDepth := depth + opens - closes
		if pending != "" && opens > 0 {
			containers = append(containers, phpContainer{name: pending, startLine: i + 1, depth: max(newDepth, depth+1)})
			pending = ""
		}
		depth = newDepth
	}
	return containers
}

func enclosingPHPContainer(containers []phpContainer, line int) string {
	var (
		name      string
		bestStart int
	)
	for _, c := range containers {
		if c.startLine >= line {
			continue
		}
		if c.startLine > bestStart {
			bestStart = c.startLine
			name = c.name
		}
	}
	return name
}

func extractPHPSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), phpSignatureLineWindow) {
		snippet := gatherPHPSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "interface":
			if sig := extractPHPTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractPHPFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractPHPTypeSignature(compact, name string) string {
	match := phpContainerDecl.FindStringSubmatch(compact)
	if len(match) != 3 || match[2] != name {
		return ""
	}
	if idx := strings.Index(compact, "{"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func extractPHPFunctionSignature(compact, name string) string {
	match := phpNamedFunction.FindStringSubmatch(compact)
	if len(match) != 2 || match[1] != name {
		return ""
	}
	if idx := strings.Index(compact, "{"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	if idx := strings.Index(compact, ";"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func gatherPHPSnippet(lines []string, startLine int) string {
	var parts []string
	openParens := 0
	for i := startLine - 1; i < len(lines) && i < startLine+5; i++ {
		line := strings.TrimSpace(stripPHPLine(lines[i]))
		if line == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, line)
		openParens += strings.Count(line, "(") - strings.Count(line, ")")
		if (strings.Contains(line, "{") || strings.Contains(line, ";")) && openParens <= 0 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func stripPHPLine(line string) string {
	var out strings.Builder
	var quote rune
	escaped := false
	prev := rune(0)
	for _, r := range line {
		if quote != 0 {
			if escaped {
				escaped = false
				prev = r
				continue
			}
			if r == '\\' {
				escaped = true
				prev = r
				continue
			}
			if r == quote {
				quote = 0
			}
			prev = r
			continue
		}
		if r == '/' && prev == '/' {
			s := out.String()
			return s[:len(s)-1]
		}
		if r == '#' {
			return out.String()
		}
		if r == '"' || r == '\'' {
			quote = r
			prev = r
			continue
		}
		out.WriteRune(r)
		prev = r
	}
	return out.String()
}
