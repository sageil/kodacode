package search

import (
	"os"
	"regexp"
	"strings"
)

type jsTSSymbolEnricher struct{}

const jstsSignatureLineWindow = 2

func (jsTSSymbolEnricher) Name() string { return "js-ts-signatures" }

func (jsTSSymbolEnricher) Supports(language, path string) bool {
	switch DetectLanguage(path, language) {
	case "javascript", "typescript":
		return true
	default:
		return false
	}
}

func (jsTSSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectJSTSContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		language := DetectLanguage(path, out[i].Language)
		if language != "javascript" && language != "typescript" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = enclosingContainer(containers, out[i].Line)
		}
		if out[i].Signature == "" {
			if sig := extractJSTSSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type jsContainer struct {
	name      string
	startLine int
	depth     int
}

var (
	jstsContainerDecl     = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?(?:declare\s+)?(class|interface|namespace|module)\s+([A-Za-z_$][\w$]*)`)
	jstsFuncDecl          = regexp.MustCompile(`(?:export\s+)?(?:default\s+)?(?:async\s+)?function\*?\s+([A-Za-z_$][\w$]*)\s*(<[^>]+>)?\s*\(`)
	jstsConstArrow        = regexp.MustCompile(`(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?(?:<[^>]+>\s*)?\(`)
	jstsConstArrowBare    = regexp.MustCompile(`(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?[A-Za-z_$][\w$]*\s*=>`)
	jstsAssignedArrow     = regexp.MustCompile(`(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+|override\s+|declare\s+|abstract\s+)*([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?(?:<[^>]+>\s*)?\(`)
	jstsAssignedArrowBare = regexp.MustCompile(`(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+|override\s+|declare\s+|abstract\s+)*([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?[A-Za-z_$][\w$]*\s*=>`)
	jstsFuncExpr          = regexp.MustCompile(`(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?function\*?\s*\(`)
	jstsAssignedFuncExpr  = regexp.MustCompile(`(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+|override\s+|declare\s+|abstract\s+)*([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?function\*?\s*\(`)
	jstsConstructorDecl   = regexp.MustCompile(`(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+|override\s+|declare\s+|abstract\s+|async\s+)*constructor\s*\(`)
	jstsGetterSetterDecl  = regexp.MustCompile(`(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+|override\s+|declare\s+|abstract\s+)*(get|set)\s+([A-Za-z_$][\w$]*)\s*\(`)
	jstsMethodDecl        = regexp.MustCompile(`(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+|override\s+|declare\s+|abstract\s+|async\s+)*([A-Za-z_$][\w$]*)\s*(<[^>]+>)?\s*\(`)
	jstsTypeDeclStart     = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:abstract\s+)?(?:declare\s+)?(class|interface|type|enum|namespace|module)\s+([A-Za-z_$][\w$]*)`)
)

func detectJSTSContainers(lines []string) []jsContainer {
	var (
		containers []jsContainer
		stack      []jsContainer
		pending    string
		depth      int
	)

	for i, raw := range lines {
		for len(stack) > 0 && depth < stack[len(stack)-1].depth {
			stack = stack[:len(stack)-1]
		}

		line := stripStringsAndLineComments(raw)
		if match := jstsContainerDecl.FindStringSubmatch(line); len(match) == 3 {
			pending = match[2]
		}

		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		newDepth := depth + opens - closes

		if pending != "" && opens > 0 {
			container := jsContainer{name: pending, startLine: i + 1, depth: max(newDepth, depth+1)}
			containers = append(containers, container)
			stack = append(stack, container)
			pending = ""
		}
		depth = newDepth
	}

	return containers
}

func enclosingContainer(containers []jsContainer, line int) string {
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

func extractJSTSSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), jstsSignatureLineWindow) {
		snippet := gatherJSTSSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "interface":
			if sig := extractJSTSTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractJSTSFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractJSTSTypeSignature(compact string, name string) string {
	match := jstsTypeDeclStart.FindStringSubmatch(compact)
	if len(match) != 3 || match[2] != name {
		return ""
	}
	sig := compact
	if idx := strings.Index(sig, "{"); idx >= 0 {
		sig = strings.TrimSpace(sig[:idx])
	}
	if match[1] == "type" {
		if idx := strings.Index(sig, ";"); idx >= 0 {
			sig = strings.TrimSpace(sig[:idx])
		}
	} else if idx := strings.Index(sig, ";"); idx >= 0 {
		sig = strings.TrimSpace(sig[:idx])
	}
	return strings.TrimSpace(sig)
}

func extractJSTSFunctionSignature(compact string, name string) string {
	if match := jstsFuncDecl.FindStringSubmatch(compact); len(match) >= 2 && match[1] == name {
		return trimJSTSSignature(match[0], compact)
	}
	if match := jstsFuncExpr.FindStringSubmatch(compact); len(match) >= 2 && match[1] == name {
		return trimJSTSSignature(match[0], compact)
	}
	if match := jstsConstArrow.FindStringSubmatch(compact); len(match) >= 2 && match[1] == name {
		return trimJSTSSignature(match[0], compact)
	}
	if match := jstsConstArrowBare.FindStringSubmatch(compact); len(match) >= 2 && match[1] == name {
		return trimJSTSSignature(match[0], compact)
	}
	if match := jstsAssignedArrow.FindStringSubmatch(compact); len(match) >= 2 && match[1] == name {
		return trimJSTSSignature(match[0], compact)
	}
	if match := jstsAssignedArrowBare.FindStringSubmatch(compact); len(match) >= 2 && match[1] == name {
		return trimJSTSSignature(match[0], compact)
	}
	if match := jstsAssignedFuncExpr.FindStringSubmatch(compact); len(match) >= 2 && match[1] == name {
		return trimJSTSSignature(match[0], compact)
	}
	if name == "constructor" {
		if match := jstsConstructorDecl.FindString(compact); match != "" {
			return trimJSTSSignature(match, compact)
		}
	}
	if match := jstsGetterSetterDecl.FindStringSubmatch(compact); len(match) == 3 && match[2] == name {
		return trimJSTSSignature(match[0], compact)
	}
	if match := jstsMethodDecl.FindStringSubmatch(compact); len(match) >= 2 && match[1] == name && !isJSTSReservedMethodName(name) {
		return trimJSTSSignature(match[0], compact)
	}
	return ""
}

func gatherJSTSSnippet(lines []string, startLine int) string {
	var parts []string
	openParens := 0
	for i := startLine - 1; i < len(lines) && i < startLine+4; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, line)
		cleaned := stripStringsAndLineComments(line)
		openParens += strings.Count(cleaned, "(") - strings.Count(cleaned, ")")
		if (strings.Contains(cleaned, "{") || strings.Contains(cleaned, "=>") || strings.Contains(cleaned, ";")) && openParens <= 0 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func trimJSTSSignature(prefix, compact string) string {
	if prefix == "" {
		return ""
	}
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

func candidateLineNumbers(line, maxLine, window int) []int {
	lines := []int{line}
	for delta := 1; delta <= window; delta++ {
		if prev := line - delta; prev >= 1 {
			lines = append(lines, prev)
		}
		if next := line + delta; next <= maxLine {
			lines = append(lines, next)
		}
	}
	return lines
}

func isJSTSReservedMethodName(name string) bool {
	switch name {
	case "if", "for", "while", "switch", "catch", "return":
		return true
	default:
		return false
	}
}

func stripStringsAndLineComments(line string) string {
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
			// Remove the previously written '/' and stop.
			s := out.String()
			return s[:len(s)-1]
		}
		if r == '"' || r == '\'' || r == '`' {
			quote = r
			prev = r
			continue
		}
		out.WriteRune(r)
		prev = r
	}
	return out.String()
}

func compactWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
