package search

import (
	"strconv"
	"strings"
)

const semanticChunkLines = 40

var declarationModifiers = map[string]bool{
	"abstract":  true,
	"async":     true,
	"default":   true,
	"export":    true,
	"extern":    true,
	"final":     true,
	"inline":    true,
	"internal":  true,
	"open":      true,
	"override":  true,
	"partial":   true,
	"private":   true,
	"protected": true,
	"public":    true,
	"pub":       true,
	"readonly":  true,
	"sealed":    true,
	"static":    true,
	"virtual":   true,
}

type chunkInput struct {
	Chunk cachedChunk
	Text  string
}

type chunkBoundary struct {
	Start int
	Line  int
}

func chunkFile(workspaceRoot, path, content string) ([]cachedChunk, []string) {
	inputs := buildChunkInputs(workspaceRoot, path, content)
	chunks := make([]cachedChunk, 0, len(inputs))
	texts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		chunks = append(chunks, input.Chunk)
		texts = append(texts, input.Text)
	}
	return chunks, texts
}

func buildChunkInputs(workspaceRoot, path, content string) []chunkInput {
	display := displayPath(workspaceRoot, path)
	if inputs := buildBoundaryChunkInputs(content, display); len(inputs) > 0 {
		return inputs
	}
	return buildWindowChunkInputs(content, display)
}

func buildBoundaryChunkInputs(content, display string) []chunkInput {
	lines := strings.Split(content, "\n")
	boundaries := collectChunkBoundaries(lines)
	if len(boundaries) == 0 {
		return nil
	}
	return buildSectionChunkInputs(lines, display, boundaries)
}

func collectChunkBoundaries(lines []string) []chunkBoundary {
	boundaries := make([]chunkBoundary, 0, 8)
	for idx, line := range lines {
		if !isChunkBoundary(line) {
			continue
		}
		boundary := chunkBoundary{
			Start: boundaryContextStart(lines, idx),
			Line:  idx + 1,
		}
		if len(boundaries) > 0 && boundary.Start <= boundaries[len(boundaries)-1].Start && boundary.Line <= boundaries[len(boundaries)-1].Line {
			continue
		}
		boundaries = append(boundaries, boundary)
	}
	return boundaries
}

func buildWindowChunkInputs(content, display string) []chunkInput {
	lines := strings.Split(content, "\n")
	boundaries := make([]chunkBoundary, 0, max(1, len(lines)/semanticChunkLines))
	for start := 1; start <= len(lines); start += semanticChunkLines {
		boundaries = append(boundaries, chunkBoundary{Start: start, Line: start})
	}
	return buildSectionChunkInputs(lines, display, boundaries)
}

func buildSectionChunkInputs(lines []string, display string, boundaries []chunkBoundary) []chunkInput {
	inputs := make([]chunkInput, 0, len(boundaries))
	for idx, boundary := range boundaries {
		end := min(boundary.Start+semanticChunkLines-1, len(lines))
		if idx+1 < len(boundaries) {
			end = min(end, boundaries[idx+1].Start-1)
		}
		block := joinChunkLines(lines, boundary.Start, end)
		if block == "" {
			continue
		}
		inputs = append(inputs, chunkInput{
			Chunk: cachedChunk{
				Path:      display,
				StartLine: boundary.Start,
				EndLine:   end,
				Line:      boundary.Line,
				Snippet:   summarizeLine(chunkSnippetLine(lines, boundary.Line)),
			},
			Text: chunkText(display, boundary.Line, "", "", "", block),
		})
	}
	return inputs
}

func chunkText(display string, line int, kind, name, detail, block string) string {
	var b strings.Builder
	b.WriteString(display)
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(line))
	if kind != "" {
		b.WriteByte(' ')
		b.WriteString(kind)
	}
	if name != "" {
		b.WriteByte(' ')
		b.WriteString(name)
	}
	if detail = strings.TrimSpace(detail); detail != "" {
		b.WriteByte('\n')
		b.WriteString(detail)
	}
	if block = strings.TrimSpace(block); block != "" {
		b.WriteByte('\n')
		b.WriteString(block)
	}
	return b.String()
}

func joinChunkLines(lines []string, start, end int) string {
	if start < 1 || start > len(lines) || end < start {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[start-1:end], "\n"))
}

func firstContentLine(block string) string {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return block
}

func chunkSnippetLine(lines []string, line int) string {
	if line < 1 || line > len(lines) {
		return ""
	}
	if trimmed := strings.TrimSpace(lines[line-1]); trimmed != "" {
		return trimmed
	}
	return firstContentLine(strings.Join(lines, "\n"))
}

func isChunkBoundary(line string) bool {
	trimmed := strings.TrimSpace(line)
	switch {
	case trimmed == "":
		return false
	case isMarkdownHeading(trimmed), isBracketSection(trimmed):
		return true
	case isBoundaryContext(trimmed):
		return false
	}

	normalized := stripDeclarationModifiers(trimmed)
	return hasDeclarationPrefix(normalized) || looksLikeAssignedFunction(normalized) || looksLikeCallableBlock(normalized)
}

func isMarkdownHeading(line string) bool {
	return len(line) > 1 && strings.HasPrefix(line, "#") && line[1] == ' '
}

func isBracketSection(line string) bool {
	return strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && !strings.Contains(line[1:len(line)-1], "]")
}

func isBoundaryContext(line string) bool {
	switch {
	case strings.HasPrefix(line, "//"),
		strings.HasPrefix(line, "/*"),
		strings.HasPrefix(line, "*"),
		strings.HasPrefix(line, "*/"),
		strings.HasPrefix(line, "@"),
		strings.HasPrefix(line, "#["),
		strings.HasPrefix(line, "///"),
		strings.HasPrefix(line, "-- "):
		return true
	default:
		return false
	}
}

func boundaryContextStart(lines []string, idx int) int {
	start := idx + 1
	for current := idx - 1; current >= 0; current-- {
		trimmed := strings.TrimSpace(lines[current])
		if trimmed == "" || !isBoundaryContext(trimmed) {
			break
		}
		start = current + 1
	}
	return start
}

func stripDeclarationModifiers(line string) string {
	rest := strings.TrimSpace(line)
	for {
		token, remainder, ok := cutFirstToken(rest)
		if !ok || !declarationModifiers[token] {
			return rest
		}
		rest = strings.TrimSpace(remainder)
	}
}

func cutFirstToken(line string) (string, string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", "", false
	}
	token := fields[0]
	remainder := strings.TrimSpace(strings.TrimPrefix(line, token))
	return token, remainder, true
}

func hasDeclarationPrefix(line string) bool {
	for _, prefix := range []string{
		"func ",
		"function ",
		"type ",
		"class ",
		"interface ",
		"struct ",
		"enum ",
		"trait ",
		"impl ",
		"fn ",
		"def ",
		"protocol ",
		"namespace ",
		"module ",
		"record ",
		"object ",
		"const ",
		"var ",
		"let ",
		"val ",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func looksLikeAssignedFunction(line string) bool {
	if strings.Contains(line, "=>") && strings.Contains(line, "=") {
		return true
	}
	for _, marker := range []string{
		":= func(",
		"= func(",
		":= function(",
		"= function(",
		":= async function(",
		"= async function(",
	} {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func looksLikeCallableBlock(line string) bool {
	if !strings.HasSuffix(line, "{") {
		return false
	}
	if strings.Contains(line, "=>") {
		return true
	}
	if strings.Contains(line, "func(") || strings.Contains(line, "function(") {
		return false
	}

	open := strings.Index(line, "(")
	close := strings.LastIndex(line, ")")
	if open <= 0 || close < open {
		return false
	}

	last := lastHeadToken(strings.TrimSpace(line[:open]))
	switch last {
	case "", "if", "for", "while", "switch", "catch", "else", "do", "return", "case", "default", "guard", "with":
		return false
	default:
		return true
	}
}

func lastHeadToken(head string) string {
	fields := strings.Fields(head)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[len(fields)-1], "*&?:!,")
}
