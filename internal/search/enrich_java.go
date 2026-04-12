package search

import (
	"os"
	"regexp"
	"strings"
)

type javaSymbolEnricher struct{}

const javaSignatureLineWindow = 2

func (javaSymbolEnricher) Name() string { return "java-signatures" }

func (javaSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "java"
}

func (javaSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectJavaContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "java" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = enclosingJavaContainer(containers, out[i].Line)
		}
		if out[i].Signature == "" {
			if sig := extractJavaSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type javaContainer struct {
	name      string
	startLine int
	depth     int
}

var (
	javaContainerDecl = regexp.MustCompile(`^\s*(?:(?:public|protected|private|abstract|final|static|sealed|non-sealed|strictfp)\s+)*(class|interface|enum|record)\s+([A-Za-z_][\w$]*)`)
	javaMethodDecl    = regexp.MustCompile(`(?:(?:public|protected|private|abstract|final|static|synchronized|native|default|strictfp)\s+)*(?:<[^>]+>\s+)?[A-Za-z_][\w$<>\[\],.?&\s]*\s+([A-Za-z_][\w$]*)\s*\(`)
	javaCtorDecl      = regexp.MustCompile(`(?:(?:public|protected|private)\s+)*([A-Za-z_][\w$]*)\s*\(`)
)

func detectJavaContainers(lines []string) []javaContainer {
	var (
		containers []javaContainer
		pending    string
		depth      int
	)

	for i, raw := range lines {
		line := stripJavaLine(raw)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@") {
			continue
		}
		if match := javaContainerDecl.FindStringSubmatch(trimmed); len(match) == 3 {
			pending = match[2]
		}

		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		newDepth := depth + opens - closes
		if pending != "" && opens > 0 {
			containers = append(containers, javaContainer{
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

func enclosingJavaContainer(containers []javaContainer, line int) string {
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

func extractJavaSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), javaSignatureLineWindow) {
		snippet := gatherJavaSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "interface":
			if sig := extractJavaTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractJavaFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractJavaTypeSignature(compact string, name string) string {
	match := javaContainerDecl.FindStringSubmatch(compact)
	if len(match) != 3 || match[2] != name {
		return ""
	}
	sig := compact
	if idx := strings.Index(sig, "{"); idx >= 0 {
		sig = strings.TrimSpace(sig[:idx])
	}
	return strings.TrimSpace(sig)
}

func extractJavaFunctionSignature(compact string, name string) string {
	if match := javaMethodDecl.FindStringSubmatch(compact); len(match) == 2 && match[1] == name {
		return trimJavaSignature(match[0], compact)
	}
	if match := javaCtorDecl.FindStringSubmatch(compact); len(match) == 2 && match[1] == name {
		return trimJavaSignature(match[0], compact)
	}
	return ""
}

func gatherJavaSnippet(lines []string, startLine int) string {
	var parts []string
	openParens := 0
	for i := startLine - 1; i < len(lines) && i < startLine+5; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "@") {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, line)
		cleaned := stripJavaLine(line)
		openParens += strings.Count(cleaned, "(") - strings.Count(cleaned, ")")
		if (strings.Contains(cleaned, "{") || strings.Contains(cleaned, ";")) && openParens <= 0 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func trimJavaSignature(prefix, compact string) string {
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

func stripJavaLine(line string) string {
	var out strings.Builder
	inDouble := false
	escaped := false
	prev := rune(0)
	for _, r := range line {
		if inDouble {
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
			if r == '"' {
				inDouble = false
			}
			prev = r
			continue
		}
		if r == '/' && prev == '/' {
			s := out.String()
			return s[:len(s)-1]
		}
		if r == '"' {
			inDouble = true
			prev = r
			continue
		}
		out.WriteRune(r)
		prev = r
	}
	return out.String()
}
