package search

import (
	"os"
	"regexp"
	"strings"
)

type zigSymbolEnricher struct{}

const zigSignatureLineWindow = 2

func (zigSymbolEnricher) Name() string { return "zig-signatures" }

func (zigSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "zig"
}

func (zigSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectZigContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "zig" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = enclosingZigContainer(containers, out[i].Line)
		}
		if out[i].Signature == "" {
			if sig := extractZigSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type zigContainer struct {
	name      string
	startLine int
	depth     int
}

var (
	zigTypeDecl = regexp.MustCompile(`^\s*(?:pub\s+)?const\s+([A-Za-z_][\w]*)\s*=\s*(?:packed\s+|extern\s+)?(struct|union|enum|opaque)\b`)
	zigFnDecl   = regexp.MustCompile(`(?:pub\s+)?(?:inline\s+)?(?:extern\s+)?fn\s+([A-Za-z_][\w]*)\s*\(`)
)

func detectZigContainers(lines []string) []zigContainer {
	var (
		containers []zigContainer
		pending    string
		depth      int
	)

	for i, raw := range lines {
		line := stripZigLine(raw)
		if match := zigTypeDecl.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 3 {
			pending = match[1]
		}

		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		newDepth := depth + opens - closes
		if pending != "" && opens > 0 {
			containers = append(containers, zigContainer{
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

func enclosingZigContainer(containers []zigContainer, line int) string {
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

func extractZigSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), zigSignatureLineWindow) {
		snippet := gatherZigSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type":
			if sig := extractZigTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractZigFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractZigTypeSignature(compact, name string) string {
	match := zigTypeDecl.FindStringSubmatch(compact)
	if len(match) != 3 || match[1] != name {
		return ""
	}
	if idx := strings.Index(compact, "{"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}

func extractZigFunctionSignature(compact, name string) string {
	match := zigFnDecl.FindStringSubmatch(compact)
	if len(match) != 2 || match[1] != name {
		return ""
	}
	return trimZigSignature(match[0], compact)
}

func gatherZigSnippet(lines []string, startLine int) string {
	var parts []string
	openParens := 0
	for i := startLine - 1; i < len(lines) && i < startLine+5; i++ {
		line := strings.TrimSpace(stripZigLine(lines[i]))
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

func trimZigSignature(prefix, compact string) string {
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
	return strings.TrimSpace(sig)
}

func stripZigLine(line string) string {
	return stripStringsAndLineComments(line)
}
