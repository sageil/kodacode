package search

import (
	"os"
	"regexp"
	"strings"
)

type kotlinSymbolEnricher struct{}

const kotlinSignatureLineWindow = 2

func (kotlinSymbolEnricher) Name() string { return "kotlin-signatures" }

func (kotlinSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "kotlin"
}

func (kotlinSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectKotlinContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "kotlin" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = enclosingKotlinContainer(containers, out[i].Line)
		}
		if out[i].Signature == "" {
			if sig := extractKotlinSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type kotlinContainer struct {
	name      string
	startLine int
	depth     int
}

var (
	kotlinContainerDecl = regexp.MustCompile(`^\s*(?:(?:public|private|protected|internal|data|sealed|enum|annotation|value|inline|open|abstract|final)\s+)*(class|interface|object)\s+([A-Za-z_][\w]*)`)
	kotlinEnumClassDecl = regexp.MustCompile(`^\s*(?:(?:public|private|protected|internal|sealed)\s+)*enum\s+class\s+([A-Za-z_][\w]*)`)
	kotlinFunDecl       = regexp.MustCompile(`^\s*(?:(?:public|private|protected|internal|open|abstract|override|final|inline|tailrec|operator|infix|external|suspend)\s+)*fun\s*(?:<[^>]+>\s*)?(?:[A-Za-z_][\w]*\.)?([A-Za-z_][\w]*)\s*\(`)
)

func detectKotlinContainers(lines []string) []kotlinContainer {
	var (
		containers []kotlinContainer
		pending    string
		depth      int
	)
	for i, raw := range lines {
		line := stripStringsAndLineComments(raw)
		trimmed := strings.TrimSpace(line)
		if match := kotlinEnumClassDecl.FindStringSubmatch(trimmed); len(match) == 2 {
			pending = match[1]
		} else if match := kotlinContainerDecl.FindStringSubmatch(trimmed); len(match) == 3 {
			pending = match[2]
		}
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		newDepth := depth + opens - closes
		if pending != "" && opens > 0 {
			containers = append(containers, kotlinContainer{name: pending, startLine: i + 1, depth: max(newDepth, depth+1)})
			pending = ""
		}
		depth = newDepth
	}
	return containers
}

func enclosingKotlinContainer(containers []kotlinContainer, line int) string {
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

func extractKotlinSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), kotlinSignatureLineWindow) {
		snippet := gatherKotlinSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "interface":
			if sig := extractKotlinTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractKotlinFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractKotlinTypeSignature(compact, name string) string {
	if match := kotlinEnumClassDecl.FindStringSubmatch(compact); len(match) == 2 && match[1] == name {
		if idx := strings.Index(compact, "{"); idx >= 0 {
			compact = strings.TrimSpace(compact[:idx])
		}
		return strings.TrimSpace(compact)
	}
	match := kotlinContainerDecl.FindStringSubmatch(compact)
	if len(match) != 3 || match[2] != name {
		return ""
	}
	if idx := strings.Index(compact, "{"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func extractKotlinFunctionSignature(compact, name string) string {
	match := kotlinFunDecl.FindStringSubmatch(compact)
	if len(match) != 2 || match[1] != name {
		return ""
	}
	if idx := strings.Index(compact, "{"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	if idx := strings.Index(compact, "="); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func gatherKotlinSnippet(lines []string, startLine int) string {
	var parts []string
	openParens := 0
	for i := startLine - 1; i < len(lines) && i < startLine+5; i++ {
		line := strings.TrimSpace(stripStringsAndLineComments(lines[i]))
		if line == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, line)
		openParens += strings.Count(line, "(") - strings.Count(line, ")")
		if (strings.Contains(line, "{") || strings.Contains(line, "=")) && openParens <= 0 {
			break
		}
	}
	return strings.Join(parts, " ")
}
