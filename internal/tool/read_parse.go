package tool

import (
	"encoding/json"
	"errors"
	"strings"
)

func parseReadInput(args json.RawMessage) (_ readInput, err error) {
	defer func() {
		err = normalizeToolInputError(ReadToolName, err)
	}()
	var raw struct {
		Path      json.RawMessage `json:"path"`
		Paths     json.RawMessage `json:"paths"`
		Offset    json.RawMessage `json:"offset"`
		Limit     json.RawMessage `json:"limit"`
		LineStart json.RawMessage `json:"line_start"`
		LineEnd   json.RawMessage `json:"line_end"`
	}
	if err := DecodeArgsStrict(ReadToolName, args, &raw, "path", "paths", "offset", "limit", "line_start", "line_end"); err != nil {
		return readInput{}, err
	}

	offset, hasOffset, err := decodeOptionalIntArg(ReadToolName, raw.Offset, "offset")
	if err != nil {
		return readInput{}, err
	}
	limit, hasLimit, err := decodeOptionalIntArg(ReadToolName, raw.Limit, "limit")
	if err != nil {
		return readInput{}, err
	}
	lineStart, hasLineStart, err := decodeOptionalIntArg(ReadToolName, raw.LineStart, "line_start")
	if err != nil {
		return readInput{}, err
	}
	lineEnd, hasLineEnd, err := decodeOptionalIntArg(ReadToolName, raw.LineEnd, "line_end")
	if err != nil {
		return readInput{}, err
	}
	if !hasOffset && hasLineStart {
		offset = lineStart - 1
		hasOffset = true
	}
	if !hasLimit && hasLineEnd {
		startLine := 1
		if hasLineStart {
			startLine = lineStart
		} else if hasOffset {
			startLine = offset + 1
		}
		limit = lineEnd - startLine + 1
		hasLimit = true
	}

	paths, err := decodeReadPaths(raw.Path, raw.Paths)
	if err != nil {
		return readInput{}, err
	}
	if len(paths) == 0 {
		return readInput{}, ErrReadPathsRequired
	}

	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return readInput{}, ErrReadEmptyPath
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	input := readInput{Paths: normalized}
	if hasOffset {
		if offset < 0 {
			return readInput{}, ErrReadOffsetInvalid
		}
		input.Offset = offset
		input.HasOffset = true
	}
	if hasLimit {
		if limit < 1 {
			return readInput{}, ErrReadLimitInvalid
		}
		input.Limit = limit
		input.HasLimit = true
	}
	return input, nil
}

func decodeReadPaths(rawPath, rawPaths json.RawMessage) ([]string, error) {
	path, hasPath, err := decodeReadPath(rawPath)
	if err != nil {
		return nil, err
	}
	paths, hasPaths, err := decodeOptionalStringArrayArg(ReadToolName, rawPaths, "paths")
	if err != nil {
		return nil, err
	}
	if hasPath && hasPaths {
		return nil, ErrReadPathConflict
	}
	if hasPath {
		return []string{path}, nil
	}
	if hasPaths {
		return paths, nil
	}
	return nil, ErrReadPathsRequired
}

func decodeReadPath(raw json.RawMessage) (string, bool, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "", false, nil
	}
	var path string
	if err := json.Unmarshal(raw, &path); err != nil {
		return "", false, InvalidArguments(ReadToolName, errors.New("path must be a string"))
	}
	if strings.EqualFold(strings.TrimSpace(path), "null") {
		return "", false, nil
	}
	if strings.TrimSpace(path) == "" {
		return "", true, ErrReadEmptyPath
	}
	return path, true, nil
}

func readJSONType(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "missing value"
	}
	switch trimmed[0] {
	case '"':
		return "string"
	case '[':
		return "array"
	case '{':
		return "object"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}
