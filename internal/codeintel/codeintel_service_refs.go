package codeintel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/sageil/kodacode/internal/lsp"
	"github.com/sageil/kodacode/internal/tool"
)

type refsServer interface {
	References(ctx context.Context, filePath string, line, character int, includeDeclaration bool) ([]lsp.Location, error)
	DocumentHighlights(ctx context.Context, filePath string, line, character int) ([]lsp.DocumentHighlight, error)
}

func (w *workspaceCodeIntel) Refs(ctx context.Context, request tool.CodeIntelRefsRequest) (tool.CodeIntelRefsResult, error) {
	manager := w.service.managerForPath(w.roots, request.Path)
	if manager == nil {
		return tool.CodeIntelRefsResult{
			Notice: codeIntelUnavailableNotice(errors.New("no code-intelligence workspace root matched this path")),
		}, nil
	}
	server, err := manager.ServerForPath(ctx, request.Path)
	if err != nil {
		if errors.Is(err, lsp.ErrServerNotConfigured) {
			return tool.CodeIntelRefsResult{Notice: codeIntelUnavailableNotice(err)}, nil
		}
		return tool.CodeIntelRefsResult{Notice: codeIntelUnavailableNotice(err)}, nil
	}
	return refsFromServer(ctx, server, request)
}

func refsFromServer(ctx context.Context, server refsServer, request tool.CodeIntelRefsRequest) (tool.CodeIntelRefsResult, error) {
	var (
		locations []lsp.Location
		err       error
	)
	for _, candidate := range tool.BestEffortPositionCandidates(request.Path, request.Line, request.Character) {
		locations, err = server.References(ctx, request.Path, request.Line-1, candidate, request.IncludeDeclaration)
		if err != nil {
			if errors.Is(err, lsp.ErrReferencesUnsupported) {
				return tool.CodeIntelRefsResult{
					Notice: codeIntelUnsupportedNotice("this file type or language server does not support reference lookup"),
				}, nil
			}
			return tool.CodeIntelRefsResult{}, err
		}
		if len(locations) > 0 {
			break
		}
	}

	target := refsTargetNode(request.Path, request.Line, request.Character)
	if len(locations) == 0 {
		return tool.CodeIntelRefsResult{
			Supported: true,
			Found:     false,
			Target:    target,
		}, nil
	}

	sorted := dedupeAndSortReferenceLocations(locations)
	lineCache := newRefsLineCache()
	classificationSupported := true
	classificationIncomplete := false
	filtered := make([]tool.CodeIntelReference, 0, min(len(sorted), request.MaxResults))
	for _, location := range sorted {
		refPath := lsp.URIToPath(location.URI)
		line := location.Range.Start.Line + 1
		character := location.Range.Start.Character
		kind, supported, err := classifyReference(ctx, server, refPath, line, character)
		if err != nil {
			return tool.CodeIntelRefsResult{}, err
		}
		if !supported {
			classificationSupported = false
			classificationIncomplete = true
		}
		if !refsModeIncludes(request.Mode, kind, supported) {
			if kind == tool.CodeIntelReferenceKindReference {
				classificationIncomplete = true
			}
			continue
		}
		snippet := lineCache.snippet(refPath, line)
		filtered = append(filtered, tool.CodeIntelReference{
			Kind:      kind,
			Path:      refPath,
			Line:      line,
			Character: character,
			Snippet:   snippet,
		})
		if request.MaxResults > 0 && len(filtered) >= request.MaxResults {
			classificationIncomplete = classificationIncomplete || len(filtered) < len(sorted)
			return tool.CodeIntelRefsResult{
				Supported:                true,
				Found:                    true,
				Target:                   target,
				References:               filtered,
				Truncated:                len(filtered) < len(sorted),
				ClassificationSupported:  classificationSupported,
				ClassificationIncomplete: classificationIncomplete,
			}, nil
		}
	}

	return tool.CodeIntelRefsResult{
		Supported:                true,
		Found:                    len(filtered) > 0,
		Target:                   target,
		References:               filtered,
		ClassificationSupported:  classificationSupported,
		ClassificationIncomplete: classificationIncomplete,
	}, nil
}

func refsModeIncludes(mode tool.CodeIntelRefsMode, kind tool.CodeIntelReferenceKind, classificationSupported bool) bool {
	switch mode {
	case tool.CodeIntelRefsModeReaders:
		return classificationSupported && kind == tool.CodeIntelReferenceKindRead
	case tool.CodeIntelRefsModeWriters:
		return classificationSupported && kind == tool.CodeIntelReferenceKindWrite
	default:
		return true
	}
}

func classifyReference(ctx context.Context, server refsServer, filePath string, line, character int) (tool.CodeIntelReferenceKind, bool, error) {
	highlights, err := server.DocumentHighlights(ctx, filePath, line-1, character)
	if err != nil {
		if errors.Is(err, lsp.ErrDocumentHighlightUnsupported) {
			return tool.CodeIntelReferenceKindReference, false, nil
		}
		return "", false, err
	}
	position := lsp.Position{Line: line - 1, Character: character}
	for _, highlight := range highlights {
		if !rangeContainsPositionInclusive(highlight.Range, position) {
			continue
		}
		switch highlight.Kind {
		case 2:
			return tool.CodeIntelReferenceKindRead, true, nil
		case 3:
			return tool.CodeIntelReferenceKindWrite, true, nil
		default:
			return tool.CodeIntelReferenceKindReference, true, nil
		}
	}
	return tool.CodeIntelReferenceKindReference, true, nil
}

func dedupeAndSortReferenceLocations(locations []lsp.Location) []lsp.Location {
	seen := make(map[string]bool, len(locations))
	out := make([]lsp.Location, 0, len(locations))
	for _, location := range locations {
		key := location.URI + "\x00" + refsPositionKey(location.Range.Start)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, location)
	}
	sort.Slice(out, func(i, j int) bool {
		pathI := lsp.URIToPath(out[i].URI)
		pathJ := lsp.URIToPath(out[j].URI)
		if pathI != pathJ {
			return pathI < pathJ
		}
		if out[i].Range.Start.Line != out[j].Range.Start.Line {
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}
		return out[i].Range.Start.Character < out[j].Range.Start.Character
	})
	return out
}

func refsPositionKey(pos lsp.Position) string {
	return fmt.Sprintf("%d:%d", pos.Line, pos.Character)
}

func refsTargetNode(path string, line, character int) tool.CodeIntelTraceNode {
	name := refsSymbolName(path, line, character)
	return tool.CodeIntelTraceNode{
		Name:      name,
		Kind:      "Symbol",
		Path:      path,
		Line:      line,
		Character: character,
	}
}

func refsSymbolName(path string, line, character int) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return nearestSymbolTokenInContent(string(content), line, character)
}

func nearestSymbolTokenInContent(content string, line, character int) string {
	text, ok := refsLineTextAt(content, line)
	if !ok {
		return ""
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	if character < 0 {
		character = 0
	}
	if character >= len(runes) {
		character = len(runes) - 1
	}
	if character < 0 {
		return ""
	}
	index := character
	if !isRefsSymbolRune(runes[index]) {
		best := -1
		bestDistance := len(runes) + 1
		for idx, r := range runes {
			if !isRefsSymbolRune(r) {
				continue
			}
			distance := absInt(idx - character)
			if distance < bestDistance {
				best = idx
				bestDistance = distance
			}
		}
		if best < 0 {
			return ""
		}
		index = best
	}
	start := index
	for start > 0 && isRefsSymbolRune(runes[start-1]) {
		start--
	}
	end := index + 1
	for end < len(runes) && isRefsSymbolRune(runes[end]) {
		end++
	}
	return strings.TrimSpace(string(runes[start:end]))
}

func refsLineTextAt(content string, line int) (string, bool) {
	lines := strings.Split(content, "\n")
	if line < 1 || line > len(lines) {
		return "", false
	}
	return lines[line-1], true
}

func isRefsSymbolRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

type refsLineCache struct {
	lines map[string][]string
}

func newRefsLineCache() *refsLineCache {
	return &refsLineCache{lines: map[string][]string{}}
}

func (c *refsLineCache) snippet(path string, line int) string {
	if c == nil || line < 1 {
		return ""
	}
	lines, ok := c.lines[path]
	if !ok {
		content, err := os.ReadFile(path)
		if err != nil {
			c.lines[path] = nil
			return ""
		}
		lines = strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
		c.lines[path] = lines
	}
	if line > len(lines) {
		return ""
	}
	return summarizeReferenceLine(lines[line-1])
}

func summarizeReferenceLine(line string) string {
	line = strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
	if len(line) <= 160 {
		return line
	}
	return line[:160] + "..."
}
