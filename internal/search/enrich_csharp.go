package search

import (
	"os"
	"regexp"
	"strings"
)

type csharpSymbolEnricher struct{}

const csharpSignatureLineWindow = 2

func (csharpSymbolEnricher) Name() string { return "csharp-signatures" }

func (csharpSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "csharp"
}

func (csharpSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectCSharpContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "csharp" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = enclosingCSharpContainer(containers, out[i].Line)
		}
		if out[i].Signature == "" {
			if sig := extractCSharpSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type csharpContainer struct {
	name      string
	startLine int
	depth     int
}

var (
	csharpContainerDecl = regexp.MustCompile(`^\s*(?:(?:public|protected|private|internal|abstract|sealed|static|partial|file)\s+)*(class|interface|struct|record|enum|namespace)\s+([A-Za-z_][\w.]*)`)
	csharpMethodDecl    = regexp.MustCompile(`(?:(?:public|protected|private|internal|abstract|sealed|static|virtual|override|async|extern|unsafe|partial|new)\s+)*(?:<[^>]+>\s+)?[A-Za-z_][\w<>\[\],.?&\s]*\s+([A-Za-z_][\w]*)(?:<[^>]+>)?\s*\(`)
	csharpCtorDecl      = regexp.MustCompile(`(?:(?:public|protected|private|internal)\s+)*([A-Za-z_][\w]*)\s*\(`)
)

func detectCSharpContainers(lines []string) []csharpContainer {
	var (
		containers []csharpContainer
		pending    string
		depth      int
	)

	for i, raw := range lines {
		line := stripStringsAndLineComments(raw)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			continue
		}
		if match := csharpContainerDecl.FindStringSubmatch(trimmed); len(match) == 3 {
			pending = lastCSharpNamePart(match[2])
		}

		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		newDepth := depth + opens - closes
		if pending != "" && opens > 0 {
			containers = append(containers, csharpContainer{
				name:      pending,
				startLine: i + 1,
				depth:     max(newDepth, depth+1),
			})
			pending = ""
		}
		depth = newDepth
	}

	return containers
}

func lastCSharpNamePart(name string) string {
	parts := strings.Split(name, ".")
	return parts[len(parts)-1]
}

func enclosingCSharpContainer(containers []csharpContainer, line int) string {
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

func extractCSharpSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), csharpSignatureLineWindow) {
		snippet := gatherCSharpSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "interface", "package":
			if sig := extractCSharpTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractCSharpFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractCSharpTypeSignature(compact, name string) string {
	match := csharpContainerDecl.FindStringSubmatch(compact)
	if len(match) != 3 || lastCSharpNamePart(match[2]) != name {
		return ""
	}
	if idx := strings.Index(compact, "{"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func extractCSharpFunctionSignature(compact, name string) string {
	if match := csharpMethodDecl.FindStringSubmatch(compact); len(match) == 2 && match[1] == name {
		return trimCSharpSignature(match[0], compact)
	}
	if match := csharpCtorDecl.FindStringSubmatch(compact); len(match) == 2 && match[1] == name {
		return trimCSharpSignature(match[0], compact)
	}
	return ""
}

func gatherCSharpSnippet(lines []string, startLine int) string {
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
		if strings.HasPrefix(line, "[") {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, line)
		openParens += strings.Count(line, "(") - strings.Count(line, ")")
		if (strings.Contains(line, "{") || strings.Contains(line, ";") || strings.Contains(line, "=>")) && openParens <= 0 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func trimCSharpSignature(prefix, compact string) string {
	start := strings.Index(compact, prefix)
	if start < 0 {
		return ""
	}
	sig := compact[start:]
	if idx := strings.Index(sig, "{"); idx >= 0 {
		sig = strings.TrimSpace(sig[:idx])
	}
	if idx := strings.Index(sig, ";"); idx >= 0 {
		sig = strings.TrimSpace(sig[:idx])
	}
	if idx := strings.Index(sig, "=>"); idx >= 0 {
		sig = strings.TrimSpace(sig[:idx])
	}
	return strings.TrimSpace(sig)
}
