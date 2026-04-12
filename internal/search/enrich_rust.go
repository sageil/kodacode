package search

import (
	"os"
	"regexp"
	"strings"
)

type rustSymbolEnricher struct{}

const rustSignatureLineWindow = 2

func (rustSymbolEnricher) Name() string { return "rust-signatures" }

func (rustSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "rust"
}

func (rustSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectRustContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "rust" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = enclosingRustContainer(containers, out[i].Line)
		}
		if out[i].Signature == "" {
			if sig := extractRustSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type rustContainer struct {
	name      string
	startLine int
	depth     int
}

var (
	rustTypeDecl = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:unsafe\s+)?(struct|enum|trait|type|union)\s+([A-Za-z_][\w]*)`)
	rustFnDecl   = regexp.MustCompile(`(?:pub(?:\([^)]*\))?\s+)?(?:default\s+)?(?:async\s+)?(?:const\s+)?(?:unsafe\s+)?fn\s+([A-Za-z_][\w]*)\s*(?:<[^>]+>)?\s*\(`)
)

func detectRustContainers(lines []string) []rustContainer {
	var (
		containers []rustContainer
		pending    string
		depth      int
	)

	for i, raw := range lines {
		line := stripBraceCountingLine(raw)
		if match := rustTypeDecl.FindStringSubmatch(line); len(match) == 3 {
			pending = match[2]
		} else if target := extractRustImplTarget(line); target != "" {
			pending = target
		}

		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		newDepth := depth + opens - closes
		if pending != "" && opens > 0 {
			containers = append(containers, rustContainer{
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

func extractRustImplTarget(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "impl") {
		return ""
	}
	if idx := strings.Index(line, "{"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	line = strings.TrimPrefix(line, "impl")
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "<") {
		if end := findMatchingAngle(line, 0); end > 0 {
			line = strings.TrimSpace(line[end+1:])
		}
	}
	if idx := strings.Index(line, " where "); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	if idx := strings.LastIndex(line, " for "); idx >= 0 {
		line = strings.TrimSpace(line[idx+5:])
	}
	target := strings.TrimSpace(line)
	if idx := strings.Index(target, "<"); idx >= 0 {
		target = target[:idx]
	}
	if idx := strings.Index(target, "::"); idx >= 0 {
		target = target[idx+2:]
	}
	fields := strings.Fields(target)
	if len(fields) == 0 {
		return ""
	}
	name := fields[len(fields)-1]
	if match := regexp.MustCompile(`([A-Za-z_][\w]*)$`).FindStringSubmatch(name); len(match) == 2 {
		return match[1]
	}
	return ""
}

func enclosingRustContainer(containers []rustContainer, line int) string {
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

func extractRustSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), rustSignatureLineWindow) {
		snippet := gatherRustSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "interface", "trait":
			if sig := extractRustTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractRustFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractRustTypeSignature(compact string, name string) string {
	match := rustTypeDecl.FindStringSubmatch(compact)
	if len(match) != 3 || match[2] != name {
		return ""
	}
	sig := compact
	if idx := strings.Index(sig, "{"); idx >= 0 {
		sig = strings.TrimSpace(sig[:idx])
	}
	if idx := strings.Index(sig, ";"); idx >= 0 {
		sig = strings.TrimSpace(sig[:idx])
	}
	return strings.TrimSpace(sig)
}

func extractRustFunctionSignature(compact string, name string) string {
	match := rustFnDecl.FindStringSubmatch(compact)
	if len(match) != 2 || match[1] != name {
		return ""
	}
	return trimRustSignature(match[0], compact)
}

func gatherRustSnippet(lines []string, startLine int) string {
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
		if strings.HasPrefix(line, "#[") {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, line)
		cleaned := stripBraceCountingLine(line)
		openParens += strings.Count(cleaned, "(") - strings.Count(cleaned, ")")
		if (strings.Contains(cleaned, "{") || strings.Contains(cleaned, ";")) && openParens <= 0 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func trimRustSignature(prefix, compact string) string {
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

func findMatchingAngle(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func stripBraceCountingLine(line string) string {
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
