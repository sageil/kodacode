package search

import (
	"os"
	"regexp"
	"strings"
)

type cSymbolEnricher struct{}

const cSignatureLineWindow = 2

func (cSymbolEnricher) Name() string { return "c-signatures" }

func (cSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "c"
}

func (cSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "c" {
			continue
		}
		if out[i].Signature == "" {
			if sig := extractCSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

var (
	cTypeDecl     = regexp.MustCompile(`^\s*(?:typedef\s+)?(struct|union|enum)\s+([A-Za-z_][\w]*)`)
	cTypedefDecl  = regexp.MustCompile(`^\s*typedef\s+(?:struct|union|enum)?\s*[A-Za-z_]*\s*\{?.*\}\s*([A-Za-z_][\w]*)\s*;?$`)
	cFunctionDecl = regexp.MustCompile(`(?:static\s+|inline\s+|extern\s+|const\s+|volatile\s+|unsigned\s+|signed\s+|long\s+|short\s+)*[A-Za-z_][\w\s\*]*\s+([A-Za-z_][\w]*)\s*\(`)
)

func extractCSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), cSignatureLineWindow) {
		snippet := gatherCSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type":
			if sig := extractCTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractCFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractCTypeSignature(compact, name string) string {
	if match := cTypeDecl.FindStringSubmatch(compact); len(match) == 3 && match[2] == name {
		if idx := strings.Index(compact, "{"); idx >= 0 {
			compact = strings.TrimSpace(compact[:idx])
		}
		if idx := strings.Index(compact, ";"); idx >= 0 {
			compact = strings.TrimSpace(compact[:idx])
		}
		return strings.TrimSpace(compact)
	}
	if match := cTypedefDecl.FindStringSubmatch(compact); len(match) == 2 && match[1] == name {
		return strings.TrimSpace(compact)
	}
	return ""
}

func extractCFunctionSignature(compact, name string) string {
	match := cFunctionDecl.FindStringSubmatch(compact)
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

func gatherCSnippet(lines []string, startLine int) string {
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
