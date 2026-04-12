package search

import (
	"os"
	"regexp"
	"strings"
)

type luaSymbolEnricher struct{}

const luaSignatureLineWindow = 2

func (luaSymbolEnricher) Name() string { return "lua-signatures" }

func (luaSymbolEnricher) Supports(language, path string) bool {
	return DetectLanguage(path, language) == "lua"
}

func (luaSymbolEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	containers := detectLuaContainers(lines)

	out := cloneSymbols(symbols)
	for i := range out {
		if DetectLanguage(path, out[i].Language) != "lua" {
			continue
		}
		if out[i].Parent == "" && out[i].Kind == "function" {
			out[i].Parent = deriveLuaParent(lines, containers, out[i])
		}
		if out[i].Signature == "" {
			if sig := extractLuaSignature(lines, out[i]); sig != "" {
				out[i].Signature = sig
			}
		}
	}
	return out
}

type luaContainer struct {
	name string
	line int
}

var (
	luaTableDecl          = regexp.MustCompile(`^\s*(?:local\s+)?([A-Za-z_][\w]*)\s*=\s*\{`)
	luaMethodDecl         = regexp.MustCompile(`^\s*function\s+([A-Za-z_][\w]*)[:.]([A-Za-z_][\w]*)\s*\(`)
	luaFunctionDecl       = regexp.MustCompile(`^\s*(?:local\s+)?function\s+([A-Za-z_][\w]*)\s*\(`)
	luaAssignedMethodDecl = regexp.MustCompile(`^\s*([A-Za-z_][\w]*)[.:]([A-Za-z_][\w]*)\s*=\s*function\s*\(`)
	luaAssignedFuncDecl   = regexp.MustCompile(`^\s*([A-Za-z_][\w]*)\s*=\s*function\s*\(`)
)

func detectLuaContainers(lines []string) []luaContainer {
	var containers []luaContainer
	for i, raw := range lines {
		line := strings.TrimSpace(stripLuaComment(raw))
		if match := luaTableDecl.FindStringSubmatch(line); len(match) == 2 {
			containers = append(containers, luaContainer{name: match[1], line: i + 1})
		}
	}
	return containers
}

func deriveLuaParent(lines []string, containers []luaContainer, sym Symbol) string {
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), luaSignatureLineWindow) {
		line := strings.TrimSpace(stripLuaComment(lines[lineNo-1]))
		if match := luaMethodDecl.FindStringSubmatch(line); len(match) == 3 && match[2] == sym.Name {
			return match[1]
		}
		if match := luaAssignedMethodDecl.FindStringSubmatch(line); len(match) == 3 && match[2] == sym.Name {
			return match[1]
		}
	}

	var (
		name     string
		bestLine int
	)
	for _, c := range containers {
		if c.line >= sym.Line {
			continue
		}
		if c.line > bestLine {
			bestLine = c.line
			name = c.name
		}
	}
	return name
}

func extractLuaSignature(lines []string, sym Symbol) string {
	if sym.Line <= 0 || sym.Line > len(lines) {
		return ""
	}
	for _, lineNo := range candidateLineNumbers(sym.Line, len(lines), luaSignatureLineWindow) {
		snippet := gatherLuaSnippet(lines, lineNo)
		compact := compactWhitespace(snippet)
		if compact == "" {
			continue
		}
		switch sym.Kind {
		case "type", "package", "variable":
			if sig := extractLuaTypeSignature(compact, sym.Name); sig != "" {
				return sig
			}
		case "function":
			if sig := extractLuaFunctionSignature(compact, sym.Name); sig != "" {
				return sig
			}
		}
	}
	return ""
}

func extractLuaTypeSignature(compact, name string) string {
	match := luaTableDecl.FindStringSubmatch(compact)
	if len(match) != 2 || match[1] != name {
		return ""
	}
	return strings.TrimSpace(match[0])
}

func extractLuaFunctionSignature(compact, name string) string {
	if match := luaMethodDecl.FindStringSubmatch(compact); len(match) == 3 && match[2] == name {
		return trimLuaSignature(compact)
	}
	if match := luaAssignedMethodDecl.FindStringSubmatch(compact); len(match) == 3 && match[2] == name {
		return trimLuaSignature(compact)
	}
	if match := luaFunctionDecl.FindStringSubmatch(compact); len(match) == 2 && match[1] == name {
		return trimLuaSignature(compact)
	}
	if match := luaAssignedFuncDecl.FindStringSubmatch(compact); len(match) == 2 && match[1] == name {
		return trimLuaSignature(compact)
	}
	return ""
}

func gatherLuaSnippet(lines []string, startLine int) string {
	var parts []string
	openParens := 0
	for i := startLine - 1; i < len(lines) && i < startLine+4; i++ {
		line := strings.TrimSpace(stripLuaComment(lines[i]))
		if line == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		parts = append(parts, line)
		openParens += strings.Count(line, "(") - strings.Count(line, ")")
		if openParens <= 0 && (strings.Contains(line, "function") || strings.Contains(line, "{") || strings.Contains(line, "end") || strings.Contains(line, "=")) {
			break
		}
	}
	return strings.Join(parts, " ")
}

func stripLuaComment(line string) string {
	inSingle := false
	inDouble := false
	escaped := false
	for i := 0; i < len(line)-1; i++ {
		r := rune(line[i])
		if escaped {
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '-':
			if !inSingle && !inDouble && line[i:i+2] == "--" {
				return line[:i]
			}
		}
	}
	return line
}

func trimLuaSignature(compact string) string {
	if idx := strings.Index(compact, " end"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	if idx := strings.Index(compact, ";"); idx >= 0 {
		compact = strings.TrimSpace(compact[:idx])
	}
	return strings.TrimSpace(compact)
}
