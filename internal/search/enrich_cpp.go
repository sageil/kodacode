package search

import (
	"os"
	"regexp"
	"strings"
)

type cppSymbolEnricher struct{}

const cppSignatureLineWindow = 2

func (cppSymbolEnricher) Name() string { return "cpp-signatures" }

func (cppSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "cpp"
}

func (cppSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectCPPContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "cpp" {
			continue
		}
		if out[i].Kind == "function" {
			if out[i].Parent == "" {
				out[i].Parent = deriveCPPParent(lines, containers, out[i])
			}
		}
		if out[i].Signature == "" {
			if sig := extractCPPSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type cppContainer struct {
	name      string
	startLine int
	depth     int
}

var (
	cppContainerDecl = regexp.MustCompile(`^\s*(class|struct|namespace)\s+([A-Za-z_][\w]*)`)
	cppFunctionDecl  = regexp.MustCompile(`(?:template\s*<[^>]+>\s*)?(?:inline\s+|constexpr\s+|static\s+|virtual\s+|explicit\s+|friend\s+|extern\s+)*[A-Za-z_~][\w:<>\s*&,\[\]]*\s+((?:[A-Za-z_][\w]*::)?~?[A-Za-z_][\w]*)\s*\(`)
)

func detectCPPContainers(lines []string) []cppContainer {
	var (
		containers []cppContainer
		pending    string
		depth      int
	)
	for i, raw := range lines {
		line := stripStringsAndLineComments(raw)
		if match := cppContainerDecl.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 3 {
			pending = match[2]
		}
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		newDepth := depth + opens - closes
		if pending != "" && opens > 0 {
			containers = append(containers, cppContainer{name: pending, startLine: i + 1, depth: max(newDepth, depth+1)})
			pending = ""
		}
		depth = newDepth
	}
	return containers
}

func deriveCPPParent(lines []string, containers []cppContainer, sym Symbol) string {
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), cppSignatureLineWindow) {
		snippet := gatherCPPSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		if match := cppFunctionDecl.FindStringSubmatch(compact); len(match) == 2 {
			if parts := strings.Split(match[1], "::"); len(parts) > 1 {
				return parts[len(parts)-2]
			}
		}
	}
	var (
		name      string
		bestStart int
	)
	for _, c := range containers {
		if c.startLine >= sym.Line {
			continue
		}
		if c.startLine > bestStart {
			bestStart = c.startLine
			name = c.name
		}
	}
	return name
}

func extractCPPSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), cppSignatureLineWindow) {
		snippet := gatherCPPSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "package":
			if sig := extractCPPTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractCPPFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractCPPTypeSignature(compact, name string) string {
	match := cppContainerDecl.FindStringSubmatch(compact)
	if len(match) != 3 || match[2] != name {
		return ""
	}
	if idx := strings.Index(compact, "{"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func extractCPPFunctionSignature(compact, name string) string {
	match := cppFunctionDecl.FindStringSubmatch(compact)
	if len(match) != 2 {
		return ""
	}
	target := match[1]
	if parts := strings.Split(target, "::"); len(parts) > 0 {
		target = parts[len(parts)-1]
	}
	target = strings.TrimPrefix(target, "~")
	compare := strings.TrimPrefix(name, "~")
	if target != compare {
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

func gatherCPPSnippet(lines []string, startLine int) string {
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
		if (strings.Contains(line, "{") || strings.Contains(line, ";")) && openParens <= 0 {
			break
		}
	}
	return strings.Join(parts, " ")
}
