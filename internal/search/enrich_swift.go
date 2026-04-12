package search

import (
	"os"
	"regexp"
	"strings"
)

type swiftSymbolEnricher struct{}

const swiftSignatureLineWindow = 2

func (swiftSymbolEnricher) Name() string { return "swift-signatures" }

func (swiftSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "swift"
}

func (swiftSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectSwiftContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "swift" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = enclosingSwiftContainer(containers, out[i].Line)
		}
		if out[i].Signature == "" {
			if sig := extractSwiftSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type swiftContainer struct {
	name      string
	startLine int
	depth     int
}

var (
	swiftContainerDecl = regexp.MustCompile(`^\s*(?:(?:public|private|fileprivate|internal|open|final|actor|indirect|extension|enum|struct|class|protocol)\s+)*(class|struct|enum|protocol|actor|extension)\s+([A-Za-z_][\w]*)`)
	swiftFuncDecl      = regexp.MustCompile(`^\s*(?:(?:public|private|fileprivate|internal|open|final|override|mutating|nonmutating|convenience|required|static|class|async|throws|rethrows)\s+)*func\s+([A-Za-z_][\w]*)\s*(<[^>]+>)?\s*\(`)
	swiftInitDecl      = regexp.MustCompile(`^\s*(?:(?:public|private|fileprivate|internal|open|final|override|mutating|nonmutating|convenience|required)\s+)*init(?:\?)?\s*\(`)
)

func detectSwiftContainers(lines []string) []swiftContainer {
	var (
		containers []swiftContainer
		pending    string
		depth      int
	)
	for i, raw := range lines {
		line := stripStringsAndLineComments(raw)
		if match := swiftContainerDecl.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 3 {
			pending = match[2]
		}
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		newDepth := depth + opens - closes
		if pending != "" && opens > 0 {
			containers = append(containers, swiftContainer{name: pending, startLine: i + 1, depth: max(newDepth, depth+1)})
			pending = ""
		}
		depth = newDepth
	}
	return containers
}

func enclosingSwiftContainer(containers []swiftContainer, line int) string {
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

func extractSwiftSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), swiftSignatureLineWindow) {
		snippet := gatherSwiftSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "interface":
			if sig := extractSwiftTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractSwiftFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractSwiftTypeSignature(compact, name string) string {
	match := swiftContainerDecl.FindStringSubmatch(compact)
	if len(match) != 3 || match[2] != name {
		return ""
	}
	if idx := strings.Index(compact, "{"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func extractSwiftFunctionSignature(compact, name string) string {
	if name == "init" {
		if !swiftInitDecl.MatchString(compact) {
			return ""
		}
		if idx := strings.Index(compact, "{"); idx >= 0 {
			compact = strings.TrimSpace(compact[:idx])
		}
		return strings.TrimSpace(compact)
	}
	match := swiftFuncDecl.FindStringSubmatch(compact)
	if len(match) != 3 || match[1] != name {
		return ""
	}
	if idx := strings.Index(compact, "{"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func gatherSwiftSnippet(lines []string, startLine int) string {
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
		if (strings.Contains(line, "{") || strings.Contains(line, "->")) && openParens <= 0 {
			break
		}
	}
	return strings.Join(parts, " ")
}
